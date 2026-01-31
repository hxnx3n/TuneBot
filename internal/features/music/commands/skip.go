package commands

import (
	"github.com/bwmarrin/discordgo"
	shared "github.com/hxnx/tunebot/internal/features/shared"
	"github.com/hxnx/tunebot/internal/music"
)

func Skip(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if i.GuildID == "" {
		shared.RespondEphemeral(s, i, "이 명령어는 서버에서만 사용할 수 있습니다.")
		return
	}

	player := music.DefaultPlayerManager.Get(i.GuildID)
	if err := player.Skip(); err != nil {
		shared.RespondEphemeral(s, i, "스킵할 곡이 없습니다.")
		return
	}

	shared.RespondEphemeral(s, i, "다음 곡으로 넘어갑니다.")
}
