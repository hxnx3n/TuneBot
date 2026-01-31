package listeners

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

func RouteDashboardComponent(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	switch i.Type {
	case discordgo.InteractionModalSubmit:
		if i.ModalSubmitData().CustomID == dashboardSearchModalID {
			handleDashboardSearch(s, i)
			return true
		}
		return false
	case discordgo.InteractionMessageComponent:
	default:
		return false
	}

	customID := i.MessageComponentData().CustomID
	if !strings.HasPrefix(customID, "dashboard_") {
		return false
	}

	HandleDashboardComponent(s, i)
	return true
}
