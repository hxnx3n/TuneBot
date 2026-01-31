package listeners

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	dashboard "github.com/hxnx/tunebot/internal/features/dashboard"
	musicsearch "github.com/hxnx/tunebot/internal/features/music/search"
	"github.com/hxnx/tunebot/internal/music"
)

func handleDashboardSearch(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionModalSubmit:
		handleDashboardSearchModalSubmit(s, i)
		return
	case discordgo.InteractionMessageComponent:
	default:
		return
	}

	userID := getInteractionUserID(i)
	if userID == "" {
		dashboard.RespondEphemeral(s, i, "사용자 정보를 확인할 수 없습니다.")
		return
	}

	modal := &discordgo.InteractionResponseData{
		CustomID: dashboardSearchModalID,
		Title:    "노래 검색",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    dashboardSearchInputID,
						Label:       "노래 제목 또는 URL",
						Style:       discordgo.TextInputShort,
						Placeholder: "유튜브/스포티파이/URL 입력",
						Required:    true,
					},
				},
			},
			discordgo.Label{
				Label:       "플랫폼 (auto / yt / sp / sc)",
				Description: "자동 선택 또는 직접 선택하세요",
				Component: discordgo.SelectMenu{
					MenuType:    discordgo.StringSelectMenu,
					CustomID:    dashboardSearchProviderInputID,
					Placeholder: "플랫폼 선택",
					Options: []discordgo.SelectMenuOption{
						{
							Label:   "자동",
							Value:   "auto",
							Default: true,
						},
						{
							Label: "YouTube",
							Value: "youtube",
						},
						{
							Label: "Spotify",
							Value: "spotify",
						},
						{
							Label: "SoundCloud",
							Value: "soundcloud",
						},
					},
				},
			},
		},
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: modal,
	}); err != nil {
		log.Printf("dashboard search: modal open failed: %v", err)
	}
}

func sendFollowupEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if s == nil || i == nil {
		return
	}

	const maxContentLength = 2000
	if len(content) > maxContentLength {
		if maxContentLength > 1 {
			content = content[:maxContentLength-1] + "…"
		} else {
			content = content[:maxContentLength]
		}
	}

	divider := true
	spacing := discordgo.SeparatorSpacingSizeSmall

	components := []discordgo.MessageComponent{
		discordgo.Container{
			AccentColor: &musicsearch.AccentColor,
			Components: []discordgo.MessageComponent{
				discordgo.TextDisplay{Content: "알림"},
				discordgo.Separator{Divider: &divider, Spacing: &spacing},
				discordgo.TextDisplay{Content: content},
			},
		},
	}

	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Components: components,
		Flags:      discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
	})
	if err != nil {
		log.Printf("dashboard search: followup failed: %v", err)
	}
}

func sendFollowupSearchResults(s *discordgo.Session, i *discordgo.InteractionCreate, query string, results []music.Track) {
	if s == nil || i == nil {
		return
	}

	components := musicsearch.BuildSearchComponents(musicsearch.SearchCustomIDPrefix, query, results)

	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Components: components,
		Flags:      discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
	})
	if err != nil {
		log.Printf("dashboard search: followup results failed: %v", err)
	}
}

func handleDashboardSearchModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionModalSubmit {
		return
	}

	userID := getInteractionUserID(i)
	if userID == "" {
		dashboard.RespondEphemeral(s, i, "사용자 정보를 확인할 수 없습니다.")
		return
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		log.Printf("dashboard search: defer failed: %v", err)
		return
	}

	data := i.ModalSubmitData()
	query := strings.TrimSpace(getModalInputValue(data, dashboardSearchInputID))
	if query == "" {
		log.Printf("dashboard search: empty input, modal components: %s", formatModalComponents(data))
		sendFollowupEphemeral(s, i, "입력값이 비어 있습니다.")
		return
	}

	provider := strings.TrimSpace(getModalSelectValue(data, dashboardSearchProviderInputID))
	sourceHint := parseProviderHint(provider)
	if provider != "" && strings.ToLower(provider) != "auto" && sourceHint == music.TrackSourceUnknown {
		sendFollowupEphemeral(s, i, "지원하지 않는 플랫폼입니다. 자동 / youtube / spotify / soundcloud 중에서 선택해 주세요.")
		return
	}
	if sourceHint == music.TrackSourceUnknown {
		sourceHint = detectSourceHint(query)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	var spotifyClient *music.SpotifyClient
	spotifyID := strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_ID"))
	spotifySecret := strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_SECRET"))
	if spotifyID != "" && spotifySecret != "" {
		spotifyClient = music.NewSpotifyClient(spotifyID, spotifySecret)
	}

	results, err := music.SearchTracks(ctx, query, sourceHint, musicsearch.MaxResults, music.NewYTDLPResolver(), spotifyClient)
	if err != nil {
		log.Printf("dashboard search: search failed: %v", err)
		sendFollowupEphemeral(s, i, "검색에 실패했습니다.")
		return
	}
	if len(results) == 0 {
		sendFollowupEphemeral(s, i, "검색 결과가 없습니다.")
		return
	}

	musicsearch.SaveSession(musicsearch.Session{
		GuildID: i.GuildID,
		UserID:  userID,
		Query:   query,
		Results: results,
	})

	sendFollowupSearchResults(s, i, query, results)
}
