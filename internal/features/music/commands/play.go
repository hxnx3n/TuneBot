package commands

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/features/modals"
	musicsearch "github.com/hxnx/tunebot/internal/features/music/search"
	shared "github.com/hxnx/tunebot/internal/features/shared"
	"github.com/hxnx/tunebot/internal/music"
)

const (
	playSearchModalID         = "play_search_modal"
	playSearchInputID         = "play_search_input"
	playSearchProviderInputID = "play_search_provider_input"
)

func Play(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if i.GuildID == "" {
		shared.RespondEphemeral(s, i, "이 명령어는 서버에서만 사용할 수 있습니다.")
		return
	}

	userID := shared.GetInteractionUserID(i)
	if userID == "" {
		shared.RespondEphemeral(s, i, "사용자 정보를 확인할 수 없습니다.")
		return
	}

	modal := &discordgo.InteractionResponseData{
		CustomID: playSearchModalID,
		Title:    "노래 검색",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    playSearchInputID,
						Label:       "노래 제목 또는 URL",
						Style:       discordgo.TextInputShort,
						Placeholder: "유튜브/스포티파이/URL 입력",
						Required:    true,
					},
				},
			},
			discordgo.Label{
				Label:       "플랫폼 (자동/유튜브/스포티파이/사운드클라우드)",
				Description: "자동 선택 또는 직접 선택하세요",
				Component: discordgo.SelectMenu{
					MenuType:    discordgo.StringSelectMenu,
					CustomID:    playSearchProviderInputID,
					Placeholder: "플랫폼 선택",
					Options: []discordgo.SelectMenuOption{
						{
							Label:   "자동",
							Value:   "auto",
							Default: true,
						},
						{
							Label: "유튜브",
							Value: "youtube",
						},
						{
							Label: "스포티파이",
							Value: "spotify",
						},
						{
							Label: "사운드클라우드",
							Value: "soundcloud",
						},
					},
				},
			},
		},
	}

	response, err := modals.DefaultAwaiter.ShowAndAwaitModal(s, i, modal, 60*time.Second)
	if err != nil {
		log.Printf("play modal failed: %v", err)
		return
	}

	if err := deferEphemeral(response.Interaction, s); err != nil {
		log.Printf("play modal defer failed: %v", err)
		return
	}

	query := strings.TrimSpace(getModalInputValue(response.Data, playSearchInputID))
	if query == "" {
		sendFollowupEphemeral(s, response.Interaction, "입력값이 비어 있습니다.")
		return
	}

	provider := strings.TrimSpace(getModalSelectValue(response.Data, playSearchProviderInputID))
	sourceHint := parseProviderHint(provider)
	if provider != "" && strings.ToLower(provider) != "auto" && sourceHint == music.TrackSourceUnknown {
		sendFollowupEphemeral(s, response.Interaction, "지원하지 않는 플랫폼입니다. 자동/유튜브/스포티파이/사운드클라우드 중에서 선택해 주세요.")
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
		switch {
		case errors.Is(err, music.ErrSpotifyClientNil):
			sendFollowupEphemeral(s, response.Interaction, "Spotify 검색을 사용하려면 SPOTIFY_CLIENT_ID/SECRET 설정이 필요합니다.")
		default:
			log.Printf("play search failed: %v", err)
			sendFollowupEphemeral(s, response.Interaction, "검색에 실패했습니다.")
		}
		return
	}
	if len(results) == 0 {
		sendFollowupEphemeral(s, response.Interaction, "검색 결과가 없습니다.")
		return
	}

	musicsearch.SaveSession(musicsearch.Session{
		GuildID: response.Interaction.GuildID,
		UserID:  shared.GetInteractionUserID(response.Interaction),
		Query:   query,
		Results: results,
	})

	sendFollowupSearchResults(s, response.Interaction, query, results)
}

func deferEphemeral(i *discordgo.InteractionCreate, s *discordgo.Session) error {
	if s == nil || i == nil {
		return nil
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{},
	})
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
		log.Printf("play followup failed: %v", err)
	}
}

func sendFollowupSearchResults(s *discordgo.Session, i *discordgo.InteractionCreate, query string, results []music.Track) {
	if s == nil || i == nil {
		return
	}

	components := musicsearch.BuildSearchComponents(musicsearch.SearchCustomIDPrefix, query, results)

	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Components: components,
		Flags:      discordgo.MessageFlagsIsComponentsV2,
	})
	if err != nil {
		log.Printf("play followup results failed: %v", err)
	}
}
