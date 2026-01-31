package listeners

import "github.com/bwmarrin/discordgo"

func HandleDashboardComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	switch i.MessageComponentData().CustomID {
	case "dashboard_join":
		handleDashboardJoin(s, i)
	case "dashboard_search":
		handleDashboardSearch(s, i)
	case "dashboard_pause":
		handleDashboardPause(s, i)
	case "dashboard_queue":
		handleDashboardQueue(s, i)
	case "dashboard_loop":
		handleDashboardLoop(s, i)
	default:
		return
	}
}
