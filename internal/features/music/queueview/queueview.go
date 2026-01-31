package queueview

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/music"
)

const (
	CustomIDPrefix = "music_queue_page"
	DefaultPerPage = 10
	MaxPerPage     = 25
)

type PageInfo struct {
	Page       int
	PerPage    int
	TotalItems int
	TotalPages int
	StartIndex int
	EndIndex   int
}

func BuildQueueComponents(items []music.QueueItem, page int, perPage int) ([]discordgo.MessageComponent, PageInfo) {
	total := len(items)
	if perPage <= 0 {
		perPage = DefaultPerPage
	}
	perPage = clamp(perPage, 1, MaxPerPage)
	totalPages := max(1, int(math.Ceil(float64(total)/float64(perPage))))
	page = clamp(page, 1, totalPages)

	start := (page - 1) * perPage
	end := min(start+perPage, total)

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		index := i + 1
		title := strings.TrimSpace(items[i].Track.Title)
		if title == "" {
			title = "알 수 없는 제목"
		}
		if items[i].Track.URL != "" {
			lines = append(lines, fmt.Sprintf("%d. [%s](%s)", index, title, items[i].Track.URL))
		} else {
			lines = append(lines, fmt.Sprintf("%d. %s", index, title))
		}
	}

	listContent := "대기열이 비어 있습니다."
	if len(lines) > 0 {
		listContent = strings.Join(lines, "\n")
	}

	info := PageInfo{
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
		StartIndex: start,
		EndIndex:   end,
	}

	divider := true
	spacing := discordgo.SeparatorSpacingSizeSmall
	accent := 0xC9A0FF

	prevDisabled := page <= 1
	nextDisabled := page >= totalPages

	components := []discordgo.MessageComponent{
		discordgo.Container{
			AccentColor: &accent,
			Components: []discordgo.MessageComponent{
				discordgo.TextDisplay{Content: "📋 **대기열**"},
				discordgo.TextDisplay{Content: fmt.Sprintf("페이지 **%d/%d** · 표시 **%d곡**", page, totalPages, end-start)},
				discordgo.Separator{Divider: &divider, Spacing: &spacing},
				discordgo.TextDisplay{Content: listContent},
				discordgo.Separator{Divider: &divider, Spacing: &spacing},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Style:    discordgo.SecondaryButton,
							Label:    "이전",
							CustomID: MakeQueuePageCustomID(page-1, perPage),
							Disabled: prevDisabled,
						},
						discordgo.Button{
							Style:    discordgo.SecondaryButton,
							Label:    "다음",
							CustomID: MakeQueuePageCustomID(page+1, perPage),
							Disabled: nextDisabled,
						},
					},
				},
			},
		},
	}

	return components, info
}

func MakeQueuePageCustomID(page int, perPage int) string {
	if page < 1 {
		page = 1
	}
	perPage = clamp(perPage, 1, MaxPerPage)
	return fmt.Sprintf("%s:%d:%d", CustomIDPrefix, page, perPage)
}

func ParseQueuePageCustomID(customID string) (page int, perPage int, ok bool) {
	if !strings.HasPrefix(customID, CustomIDPrefix+":") {
		return 0, 0, false
	}

	parts := strings.Split(customID, ":")
	if len(parts) != 3 {
		return 0, 0, false
	}

	pageVal, err := strconv.Atoi(parts[1])
	if err != nil || pageVal < 1 {
		return 0, 0, false
	}

	perPageVal, err := strconv.Atoi(parts[2])
	if err != nil || perPageVal < 1 {
		return 0, 0, false
	}

	return pageVal, clamp(perPageVal, 1, MaxPerPage), true
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
