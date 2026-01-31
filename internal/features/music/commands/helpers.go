package commands

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/music"
)

func detectSourceHint(input string) music.TrackSource {
	lower := strings.ToLower(input)
	switch {
	case strings.Contains(lower, "spotify.com") || strings.HasPrefix(lower, "spotify:track:"):
		return music.TrackSourceSpotify
	case strings.Contains(lower, "soundcloud.com"):
		return music.TrackSourceSoundCloud
	case strings.Contains(lower, "youtube.com") || strings.Contains(lower, "youtu.be"):
		return music.TrackSourceYouTube
	default:
		return music.TrackSourceUnknown
	}
}

func getModalInputValue(data discordgo.ModalSubmitInteractionData, customID string) string {
	for _, component := range data.Components {
		var row discordgo.ActionsRow
		switch r := component.(type) {
		case discordgo.ActionsRow:
			row = r
		case *discordgo.ActionsRow:
			row = *r
		default:
			continue
		}
		for _, inner := range row.Components {
			switch input := inner.(type) {
			case discordgo.TextInput:
				if input.CustomID == customID {
					return input.Value
				}
			case *discordgo.TextInput:
				if input.CustomID == customID {
					return input.Value
				}
			}
		}
	}
	return ""
}

func getModalSelectValue(data discordgo.ModalSubmitInteractionData, customID string) string {
	for _, component := range data.Components {
		var row discordgo.ActionsRow
		switch r := component.(type) {
		case discordgo.ActionsRow:
			row = r
		case *discordgo.ActionsRow:
			row = *r
		default:
			continue
		}
		for _, inner := range row.Components {
			switch c := inner.(type) {
			case discordgo.SelectMenu:
				if c.CustomID == customID && len(c.Values) > 0 {
					return c.Values[0]
				}
			case *discordgo.SelectMenu:
				if c.CustomID == customID && len(c.Values) > 0 {
					return c.Values[0]
				}
			case discordgo.Label:
				if menu, ok := c.Component.(discordgo.SelectMenu); ok {
					if menu.CustomID == customID && len(menu.Values) > 0 {
						return menu.Values[0]
					}
				}
				if menu, ok := c.Component.(*discordgo.SelectMenu); ok {
					if menu.CustomID == customID && len(menu.Values) > 0 {
						return menu.Values[0]
					}
				}
			case *discordgo.Label:
				if menu, ok := c.Component.(discordgo.SelectMenu); ok {
					if menu.CustomID == customID && len(menu.Values) > 0 {
						return menu.Values[0]
					}
				}
				if menu, ok := c.Component.(*discordgo.SelectMenu); ok {
					if menu.CustomID == customID && len(menu.Values) > 0 {
						return menu.Values[0]
					}
				}
			}
		}
	}
	return ""
}

func parseProviderHint(provider string) music.TrackSource {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "auto", "default":
		return music.TrackSourceUnknown
	case "youtube", "yt":
		return music.TrackSourceYouTube
	case "spotify", "sp":
		return music.TrackSourceSpotify
	case "soundcloud", "sc":
		return music.TrackSourceSoundCloud
	default:
		return music.TrackSourceUnknown
	}
}
