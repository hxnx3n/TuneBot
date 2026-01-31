package listeners

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	dashboard "github.com/hxnx/tunebot/internal/features/dashboard"
	"github.com/hxnx/tunebot/internal/music"
)

func handleDashboardQueue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.GuildID == "" {
		dashboard.RespondEphemeral(s, i, "이 버튼은 서버에서만 사용할 수 있습니다.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store := music.NewQueueStoreFromDefault()
	if store == nil {
		dashboard.RespondEphemeral(s, i, "큐 저장소가 초기화되지 않았습니다.")
		return
	}

	items, err := store.List(ctx, i.GuildID, dashboardQueueListLimit)
	if err != nil {
		dashboard.RespondEphemeral(s, i, "큐 정보를 가져오지 못했습니다.")
		return
	}

	if len(items) == 0 {
		dashboard.RespondEphemeral(s, i, "현재 대기열이 비어 있습니다.")
		return
	}

	var b strings.Builder
	b.WriteString("📋 **현재 대기열**\n")
	for idx, item := range items {
		title := truncateForDisplay(item.Track.Title, 60)
		b.WriteString(fmt.Sprintf("%d. %s\n", idx+1, title))
	}

	dashboard.RespondEphemeral(s, i, b.String())
}
