package listeners

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	dashboard "github.com/hxnx/tunebot/internal/features/dashboard"
	search "github.com/hxnx/tunebot/internal/features/music/search"
	"github.com/hxnx/tunebot/internal/music"
)

const dashboardAutoDeleteDelay = 30 * time.Second

func HandleMusicMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if s == nil || m == nil || m.Author == nil {
		return
	}
	if m.Author.Bot {
		return
	}
	if m.GuildID == "" {
		return
	}

	entry, ok := dashboard.GetDashboardEntry(m.GuildID)
	if ok && entry.ChannelID != "" {
		if entry.ChannelID != m.ChannelID {
			return
		}
	} else {
		ch, err := s.Channel(m.ChannelID)
		if err != nil || ch == nil || ch.Name != dashboard.DefaultDashboardChannelName {
			return
		}
	}

	content := strings.TrimSpace(m.Content)
	if content == "" {
		return
	}
	if strings.HasPrefix(content, "/") {
		scheduleDelete(s, m.ChannelID, m.ID, dashboardAutoDeleteDelay)
		return
	}

	userID := m.Author.ID
	if userID == "" {
		return
	}

	spotifyID := strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_ID"))
	spotifySecret := strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_SECRET"))
	var spotifyClient *music.SpotifyClient
	if spotifyID != "" && spotifySecret != "" {
		spotifyClient = music.NewSpotifyClient(spotifyID, spotifySecret)
	}

	sourceHint := resolveSourceHint(content)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	divider := true
	spacing := discordgo.SeparatorSpacingSizeSmall
	loadingComponents := []discordgo.MessageComponent{
		discordgo.Container{
			AccentColor: &search.AccentColor,
			Components: []discordgo.MessageComponent{
				discordgo.TextDisplay{Content: "검색 중"},
				discordgo.Separator{Divider: &divider, Spacing: &spacing},
				discordgo.TextDisplay{Content: "요청하신 곡을 찾는 중이에요. 잠시만 기다려 주세요."},
			},
		},
	}
	loadingMsg, _ := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Components: loadingComponents,
		Flags:      discordgo.MessageFlagsIsComponentsV2,
		Reference:  &discordgo.MessageReference{MessageID: m.ID, ChannelID: m.ChannelID, GuildID: m.GuildID},
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{},
			RepliedUser: false,
		},
	})
	scheduleDelete(s, m.ChannelID, m.ID, dashboardAutoDeleteDelay)

	results, err := music.SearchTracks(ctx, content, sourceHint, search.MaxResults, music.NewYTDLPResolver(), spotifyClient)
	if err != nil {
		errorText := "검색에 실패했습니다."
		if errors.Is(err, music.ErrSpotifyClientNil) {
			errorText = "Spotify 기능은 비활성화되어 있습니다."
		} else {
			fmt.Printf("music message: search failed: %v\n", err)
		}

		if loadingMsg != nil {
			errorComponents := []discordgo.MessageComponent{
				discordgo.Container{
					AccentColor: &search.AccentColor,
					Components: []discordgo.MessageComponent{
						discordgo.TextDisplay{Content: "검색 실패"},
						discordgo.Separator{Divider: &divider, Spacing: &spacing},
						discordgo.TextDisplay{Content: errorText},
					},
				},
			}
			_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID:         loadingMsg.ID,
				Channel:    m.ChannelID,
				Components: &errorComponents,
				Flags:      discordgo.MessageFlagsIsComponentsV2,
			})
			scheduleDelete(s, m.ChannelID, loadingMsg.ID, dashboardAutoDeleteDelay)
			return
		}

		errorComponents := []discordgo.MessageComponent{
			discordgo.Container{
				AccentColor: &search.AccentColor,
				Components: []discordgo.MessageComponent{
					discordgo.TextDisplay{Content: "검색 실패"},
					discordgo.Separator{Divider: &divider, Spacing: &spacing},
					discordgo.TextDisplay{Content: errorText},
				},
			},
		}
		errorMsg, _ := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Components: errorComponents,
			Flags:      discordgo.MessageFlagsIsComponentsV2,
			Reference:  &discordgo.MessageReference{MessageID: m.ID, ChannelID: m.ChannelID, GuildID: m.GuildID},
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Parse:       []discordgo.AllowedMentionType{},
				RepliedUser: false,
			},
		})
		if errorMsg != nil {
			scheduleDelete(s, m.ChannelID, errorMsg.ID, dashboardAutoDeleteDelay)
			scheduleDelete(s, m.ChannelID, m.ID, dashboardAutoDeleteDelay)
		}
		return
	}

	search.SaveSession(search.Session{
		GuildID: m.GuildID,
		UserID:  userID,
		Query:   content,
		Results: results,
	})

	components := search.BuildSearchComponents(search.SearchCustomIDPrefix, content, results)

	if loadingMsg != nil {
		_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID:         loadingMsg.ID,
			Channel:    m.ChannelID,
			Components: &components,
			Flags:      discordgo.MessageFlagsIsComponentsV2,
		})
		scheduleDelete(s, m.ChannelID, loadingMsg.ID, dashboardAutoDeleteDelay)
		return
	}

	resultMsg, _ := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Components: components,
		Flags:      discordgo.MessageFlagsIsComponentsV2,
		Reference:  &discordgo.MessageReference{MessageID: m.ID, ChannelID: m.ChannelID, GuildID: m.GuildID},
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{},
			RepliedUser: false,
		},
	})
	if resultMsg != nil {
		scheduleDelete(s, m.ChannelID, resultMsg.ID, dashboardAutoDeleteDelay)
		scheduleDelete(s, m.ChannelID, m.ID, dashboardAutoDeleteDelay)
	}
}

func resolveSourceHint(input string) music.TrackSource {
	hint := detectSourceHint(input)
	if hint == music.TrackSourceUnknown {
		return music.TrackSourceYouTube
	}
	return hint
}

func detectSourceHint(input string) music.TrackSource {
	lower := strings.ToLower(input)
	switch {
	case strings.Contains(lower, "spotify.com") || strings.HasPrefix(lower, "spotify:track:"):
		return music.TrackSourceSpotify
	case strings.Contains(lower, "soundcloud.com"):
		return music.TrackSourceSoundCloud
	case strings.Contains(lower, "youtube.com") || strings.Contains(lower, "youtu.be"):
		return music.TrackSourceYouTube
	default:
		return music.TrackSourceUnknown
	}
}

func scheduleDelete(s *discordgo.Session, channelID, messageID string, delay time.Duration) {
	if s == nil || channelID == "" || messageID == "" {
		return
	}
	time.AfterFunc(delay, func() {
		_ = s.ChannelMessageDelete(channelID, messageID)
	})
}
