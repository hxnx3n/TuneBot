package listeners

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/features/ping"
)

func RoutePingComponent(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i.Type != discordgo.InteractionMessageComponent {
		return false
	}

	customID := i.MessageComponentData().CustomID
	if !strings.HasPrefix(customID, "ping_") {
		return false
	}

	HandlePingComponent(s, i)
	return true
}

func HandlePingComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	if i.MessageComponentData().CustomID == "ping_refresh" {
		ping.RespondPing(s, i, discordgo.InteractionResponseUpdateMessage)
	}
}
