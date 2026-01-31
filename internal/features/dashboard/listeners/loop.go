package listeners

import (
	"context"
	"time"

	"github.com/bwmarrin/discordgo"
	dashboard "github.com/hxnx/tunebot/internal/features/dashboard"
	"github.com/hxnx/tunebot/internal/music"
)

func handleDashboardLoop(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.GuildID == "" {
		dashboard.RespondEphemeral(s, i, "이 버튼은 서버에서만 사용할 수 있습니다.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store := music.NewQueueStoreFromDefault()
	if store == nil {
		dashboard.RespondEphemeral(s, i, "설정을 저장할 수 없습니다.")
		return
	}

	settings, err := store.GetSettings(ctx, i.GuildID)
	if err != nil {
		dashboard.RespondEphemeral(s, i, "현재 설정을 불러오지 못했습니다.")
		return
	}

	switch settings.RepeatMode {
	case music.RepeatModeNone:
		settings.RepeatMode = music.RepeatModeTrack
	case music.RepeatModeTrack:
		settings.RepeatMode = music.RepeatModeQueue
	default:
		settings.RepeatMode = music.RepeatModeNone
	}

	if err := store.SetSettings(ctx, i.GuildID, settings); err != nil {
		dashboard.RespondEphemeral(s, i, "설정 저장에 실패했습니다.")
		return
	}

	dashboard.UpdateDashboardSettingsCache(i.GuildID, settings)
	dashboard.RespondUpdateDashboardMessage(s, i)
}
