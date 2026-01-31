package search

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/music"
)

const (
	MaxResults           = 4
	MaxSelectOptions     = 25
	SearchSessionTTL     = 2 * time.Minute
	SearchCustomIDPrefix = "music_search_select"
)

var AccentColor = 0xC9A0FF

type Session struct {
	GuildID   string
	UserID    string
	Query     string
	Results   []music.Track
	CreatedAt time.Time
}

var store = struct {
	mu   sync.RWMutex
	data map[string]Session
}{
	data: make(map[string]Session),
}

func sessionKey(guildID, userID string) string {
	return guildID + ":" + userID
}

func SaveSession(s Session) {
	if s.GuildID == "" || s.UserID == "" {
		return
	}
	s.CreatedAt = time.Now().UTC()
	store.mu.Lock()
	store.data[sessionKey(s.GuildID, s.UserID)] = s
	store.mu.Unlock()
}

func GetSession(guildID, userID string) (Session, bool) {
	store.mu.RLock()
	session, ok := store.data[sessionKey(guildID, userID)]
	store.mu.RUnlock()
	if !ok {
		return Session{}, false
	}
	if time.Since(session.CreatedAt) > SearchSessionTTL {
		DeleteSession(guildID, userID)
		return Session{}, false
	}
	return session, true
}

func DeleteSession(guildID, userID string) {
	store.mu.Lock()
	delete(store.data, sessionKey(guildID, userID))
	store.mu.Unlock()
}

func BuildSearchComponents(customID string, query string, results []music.Track) []discordgo.MessageComponent {
	if customID == "" {
		customID = SearchCustomIDPrefix
	}

	divider := true
	spacing := discordgo.SeparatorSpacingSizeSmall

	if strings.TrimSpace(query) == "" {
		query = "알 수 없음"
	}
	summary := buildResultSummary(results)

	options := make([]discordgo.SelectMenuOption, 0, min(len(results), MaxSelectOptions))
	for i, track := range results {
		if i >= MaxSelectOptions {
			break
		}
		label := truncate(track.Title, 80)
		desc := formatResultDescription(track)
		options = append(options, discordgo.SelectMenuOption{
			Label:       label,
			Description: truncate(desc, 100),
			Value:       fmt.Sprintf("%d", i),
		})
	}

	menu := discordgo.SelectMenu{
		MenuType:    discordgo.StringSelectMenu,
		CustomID:    customID,
		Placeholder: "재생할 곡을 선택하세요",
		Options:     options,
	}

	return []discordgo.MessageComponent{
		discordgo.Container{
			AccentColor: &AccentColor,
			Components: []discordgo.MessageComponent{
				discordgo.TextDisplay{Content: "🔎 **검색 결과**"},
				discordgo.TextDisplay{Content: fmt.Sprintf("검색어: **%s**", escapeMarkdown(query))},
				discordgo.TextDisplay{Content: summary},
				discordgo.Separator{Divider: &divider, Spacing: &spacing},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						menu,
					},
				},
			},
		},
	}
}

func BuildSearchEmbed(query string, results []music.Track) *discordgo.MessageEmbed {
	lines := make([]string, 0, len(results))
	for i, track := range results {
		if i >= MaxResults {
			break
		}
		line := fmt.Sprintf(
			"%d. **%s** %s",
			i+1,
			escapeMarkdown(truncate(track.Title, 80)),
			formatDuration(track.Duration),
		)
		lines = append(lines, line)
	}

	desc := "검색 결과가 없습니다."
	if len(lines) > 0 {
		desc = strings.Join(lines, "\n")
	}

	return &discordgo.MessageEmbed{
		Title:       "검색 결과",
		Description: desc,
		Color:       AccentColor,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "검색어",
				Value: escapeMarkdown(query),
			},
		},
	}
}

func buildResultSummary(results []music.Track) string {
	lines := make([]string, 0, len(results))
	for i, track := range results {
		if i >= MaxResults {
			break
		}
		line := fmt.Sprintf(
			"%d. **%s** %s",
			i+1,
			escapeMarkdown(truncate(track.Title, 80)),
			formatDuration(track.Duration),
		)
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "검색 결과가 없습니다."
	}
	return strings.Join(lines, "\n")
}

func formatResultDescription(track music.Track) string {
	source := string(track.Source)
	if source == "" {
		source = "알 수 없음"
	}
	return fmt.Sprintf("%s • %s", source, formatDuration(track.Duration))
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "[실시간]"
	}
	totalSeconds := int(d.Seconds())
	min := totalSeconds / 60
	sec := totalSeconds % 60
	return fmt.Sprintf("[%02d:%02d]", min, sec)
}

func escapeMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"*", "\\*",
		"_", "\\_",
		"`", "\\`",
		"~", "\\~",
	)
	return replacer.Replace(text)
}

func truncate(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 1 {
		return text[:max]
	}
	return text[:max-1] + "…"
}
