package listeners

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	dashboard "github.com/hxnx/tunebot/internal/features/dashboard"
	queueview "github.com/hxnx/tunebot/internal/features/music/queueview"
	search "github.com/hxnx/tunebot/internal/features/music/search"
	"github.com/hxnx/tunebot/internal/music"
)

func HandleMusicComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if s == nil || i == nil || i.Type != discordgo.InteractionMessageComponent {
		return
	}

	data := i.MessageComponentData()
	if strings.HasPrefix(data.CustomID, queueview.CustomIDPrefix) {
		handleQueuePagination(s, i, data.CustomID)
		return
	}
	if !strings.HasPrefix(data.CustomID, search.SearchCustomIDPrefix) {
		return
	}

	userID := getInteractionUserID(i)
	if userID == "" || i.GuildID == "" {
		respondEphemeral(s, i, "사용자 정보를 확인할 수 없습니다.")
		return
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		log.Printf("music search: defer failed: %v", err)
		return
	}

	session, ok := search.GetSession(i.GuildID, userID)
	if !ok || len(session.Results) == 0 {
		sendFollowupEphemeral(s, i, "검색 세션이 만료되었거나 결과가 없습니다.")
		return
	}

	if len(data.Values) == 0 {
		sendFollowupEphemeral(s, i, "선택된 항목이 없습니다.")
		return
	}

	index, err := strconv.Atoi(data.Values[0])
	if err != nil || index < 0 || index >= len(session.Results) {
		sendFollowupEphemeral(s, i, "유효하지 않은 선택입니다.")
		return
	}

	track := session.Results[index]

	player := music.DefaultPlayerManager.Get(i.GuildID)
	spotifyID := strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_ID"))
	spotifySecret := strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_SECRET"))
	if spotifyID != "" && spotifySecret != "" {
		playerManager := music.DefaultPlayerManager.WithSpotify(music.NewSpotifyClient(spotifyID, spotifySecret))
		player = playerManager.Get(i.GuildID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	item, err := player.EnqueueAndPlay(ctx, s, userID, track.URL, track.Source, 0)
	if err != nil {
		switch {
		case errors.Is(err, music.ErrNoVoiceChannel):
			sendFollowupEphemeral(s, i, "먼저 음성 채널에 입장해 주세요.")
		case errors.Is(err, music.ErrSpotifyClientNil):
			sendFollowupEphemeral(s, i, "현재 Spotify 트랙은 재생할 수 없습니다.")
		default:
			log.Printf("music search: enqueue failed: %v", err)
			sendFollowupEphemeral(s, i, "재생 요청에 실패했습니다.")
		}
		return
	}

	search.DeleteSession(i.GuildID, userID)
	sendFollowupQueueAdded(s, i, item)
	_ = dashboard.UpdateDashboardByGuild(s, i.GuildID)
}

func handleQueuePagination(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	if s == nil || i == nil {
		return
	}

	page, perPage, ok := queueview.ParseQueuePageCustomID(customID)
	if !ok {
		respondEphemeral(s, i, "유효하지 않은 페이지 요청입니다.")
		return
	}
	if i.GuildID == "" {
		respondEphemeral(s, i, "이 명령어는 서버에서만 사용할 수 있습니다.")
		return
	}

	store := music.NewQueueStoreFromDefault()
	if store == nil {
		respondEphemeral(s, i, "대기열을 조회할 수 없습니다.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	items, err := store.List(ctx, i.GuildID, 0)
	if err != nil {
		log.Printf("queue page error: %v", err)
		respondEphemeral(s, i, "대기열을 불러오지 못했습니다.")
		return
	}
	if len(items) == 0 {
		respondEphemeral(s, i, "대기열이 비어 있습니다.")
		return
	}

	components, _ := queueview.BuildQueueComponents(items, page, perPage)

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Components: components,
			Flags:      discordgo.MessageFlagsIsComponentsV2,
		},
	}); err != nil {
		log.Printf("queue page respond failed: %v", err)
	}
}

func getInteractionUserID(i *discordgo.InteractionCreate) string {
	if i == nil {
		return ""
	}
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func deferEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{},
	})
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if s == nil || i == nil {
		return
	}

	divider := true
	spacing := discordgo.SeparatorSpacingSizeSmall

	components := []discordgo.MessageComponent{
		discordgo.Container{
			AccentColor: &search.AccentColor,
			Components: []discordgo.MessageComponent{
				discordgo.TextDisplay{Content: "알림"},
				discordgo.Separator{Divider: &divider, Spacing: &spacing},
				discordgo.TextDisplay{Content: content},
			},
		},
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Components: components,
			Flags:      discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
		},
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
			AccentColor: &search.AccentColor,
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
		log.Printf("music search: followup failed: %v", err)
	}
}

func sendFollowupQueueAdded(s *discordgo.Session, i *discordgo.InteractionCreate, item music.QueueItem) {
	if s == nil || i == nil {
		return
	}

	description := fmt.Sprintf("**%s**", item.Track.Title)
	if item.Track.URL != "" {
		description = fmt.Sprintf("[**%s**](%s)", item.Track.Title, item.Track.URL)
	}

	lines := []string{
		fmt.Sprintf("🎵 %s", description),
		fmt.Sprintf("⏱️ 재생 시간: %s", formatDurationShort(item.Track.Duration)),
	}

	store := music.NewQueueStoreFromDefault()
	if store != nil && i.GuildID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if size, err := store.QueueSize(ctx, i.GuildID); err == nil && size > 0 {
			queueText := fmt.Sprintf("%d곡", size)

			lines = append(lines,
				fmt.Sprintf("📍 순서: #%d", size),
				fmt.Sprintf("📋 대기열: %s", queueText),
			)

			dashboard.UpdateDashboardQueueCountCache(i.GuildID, size)
		}
	}

	divider := true
	spacing := discordgo.SeparatorSpacingSizeSmall

	queueInfo := strings.Join(lines, "\n")
	queueComponents := []discordgo.MessageComponent{
		discordgo.TextDisplay{Content: "대기열에 추가됨"},
		discordgo.Separator{Divider: &divider, Spacing: &spacing},
	}
	if item.Track.Thumbnail != "" {
		queueComponents = append(queueComponents, discordgo.Section{
			Components: []discordgo.MessageComponent{
				discordgo.TextDisplay{Content: queueInfo},
			},
			Accessory: discordgo.Thumbnail{
				Media: discordgo.UnfurledMediaItem{URL: item.Track.Thumbnail},
			},
		})
	} else {
		queueComponents = append(queueComponents, discordgo.TextDisplay{Content: queueInfo})
	}

	components := []discordgo.MessageComponent{
		discordgo.Container{
			AccentColor: &search.AccentColor,
			Components:  queueComponents,
		},
	}

	if i.Message == nil || i.ChannelID == "" {
		log.Printf("music search: queue update failed: missing message context")
		return
	}

	if _, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:         i.Message.ID,
		Channel:    i.ChannelID,
		Components: &components,
		Flags:      discordgo.MessageFlagsIsComponentsV2,
	}); err != nil {
		log.Printf("music search: queue update failed: %v", err)
	}
}

func formatDurationShort(d time.Duration) string {
	if d <= 0 {
		return "실시간"
	}

	totalSeconds := int(d.Seconds())
	min := totalSeconds / 60
	sec := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d", min, sec)
}
