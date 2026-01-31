package listeners

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/music"
)

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

func formatModalComponents(data discordgo.ModalSubmitInteractionData) string {
	var b strings.Builder
	for rowIndex, component := range data.Components {
		var row discordgo.ActionsRow
		switch r := component.(type) {
		case discordgo.ActionsRow:
			row = r
		case *discordgo.ActionsRow:
			row = *r
		default:
			fmt.Fprintf(&b, "row[%d]=%T ", rowIndex, component)
			continue
		}
		fmt.Fprintf(&b, "row[%d]:", rowIndex)
		for compIndex, inner := range row.Components {
			switch c := inner.(type) {
			case discordgo.TextInput:
				fmt.Fprintf(&b, " text[%d]{id=%s value=%q}", compIndex, c.CustomID, c.Value)
			case *discordgo.TextInput:
				fmt.Fprintf(&b, " text[%d]{id=%s value=%q}", compIndex, c.CustomID, c.Value)
			case discordgo.SelectMenu:
				fmt.Fprintf(&b, " select[%d]{id=%s values=%v}", compIndex, c.CustomID, c.Values)
			case *discordgo.SelectMenu:
				fmt.Fprintf(&b, " select[%d]{id=%s values=%v}", compIndex, c.CustomID, c.Values)
			case discordgo.Label:
				fmt.Fprintf(&b, " label[%d]{text=%q desc=%q comp=%T}", compIndex, c.Label, c.Description, c.Component)
			case *discordgo.Label:
				fmt.Fprintf(&b, " label[%d]{text=%q desc=%q comp=%T}", compIndex, c.Label, c.Description, c.Component)
			default:
				fmt.Fprintf(&b, " comp[%d]=%T", compIndex, inner)
			}
		}
		fmt.Fprint(&b, " ")
	}
	return strings.TrimSpace(b.String())
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func truncateForDisplay(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 1 {
		return text[:max]
	}
	return text[:max-1] + "…"
}

func getInteractionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

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

var errNoVoiceChannel = errors.New("user is not in a voice channel")

func findUserVoiceChannel(s *discordgo.Session, guildID string, userID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("discord session is nil")
	}

	var guild *discordgo.Guild
	if s.State != nil {
		if g, err := s.State.Guild(guildID); err == nil {
			guild = g
		}
	}
	if guild == nil {
		g, err := s.Guild(guildID)
		if err != nil {
			return "", err
		}
		guild = g
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID && vs.ChannelID != "" {
			return vs.ChannelID, nil
		}
	}

	return "", errNoVoiceChannel
}
