package commands

import (
	"github.com/bwmarrin/discordgo"
	shared "github.com/hxnx/tunebot/internal/features/shared"
	"github.com/hxnx/tunebot/internal/music"
)

func Stop(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if i.GuildID == "" {
		shared.RespondEphemeral(s, i, "이 명령어는 서버에서만 사용할 수 있습니다.")
		return
	}

	player := music.DefaultPlayerManager.Get(i.GuildID)
	if err := player.Stop(true); err != nil {
		shared.RespondEphemeral(s, i, "정지할 재생이 없습니다.")
		return
	}

	shared.RespondEphemeral(s, i, "재생을 정지하고 대기열을 비웠습니다.")
}
