package commands

import (
	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/features/botinfo"
)

func Info(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	botinfo.RespondBotInfo(s, i, discordgo.InteractionResponseChannelMessageWithSource)
}
