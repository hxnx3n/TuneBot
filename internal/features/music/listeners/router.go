package listeners

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

func RouteMusicComponent(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i.Type != discordgo.InteractionMessageComponent {
		return false
	}

	customID := i.MessageComponentData().CustomID
	if !strings.HasPrefix(customID, "music_") {
		return false
	}

	HandleMusicComponent(s, i)
	return true
}
