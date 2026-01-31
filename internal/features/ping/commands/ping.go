package commands

import (
	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/features/ping"
)

func Ping(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	ping.RespondPing(s, i, discordgo.InteractionResponseChannelMessageWithSource)
}
