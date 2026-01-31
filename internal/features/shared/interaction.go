package shared

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

var accentColor = 0xC9A0FF

func RespondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if s == nil || i == nil {
		return
	}

	divider := true
	spacing := discordgo.SeparatorSpacingSizeSmall

	components := []discordgo.MessageComponent{
		discordgo.Container{
			AccentColor: &accentColor,
			Components: []discordgo.MessageComponent{
				discordgo.TextDisplay{Content: "알림"},
				discordgo.Separator{Divider: &divider, Spacing: &spacing},
				discordgo.TextDisplay{Content: content},
			},
		},
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Components: components,
			Flags:      discordgo.MessageFlagsIsComponentsV2 | discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("failed to respond: %v", err)
	}
}

func GetOptionString(options []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, opt := range options {
		if opt.Name == name {
			return opt.StringValue()
		}
	}
	return ""
}

func GetOptionInt(options []*discordgo.ApplicationCommandInteractionDataOption, name string) int {
	for _, opt := range options {
		if opt.Name == name {
			return int(opt.IntValue())
		}
	}
	return 0
}

func GetOptionInt64(options []*discordgo.ApplicationCommandInteractionDataOption, name string) int64 {
	for _, opt := range options {
		if opt.Name == name {
			return opt.IntValue()
		}
	}
	return 0
}

func GetInteractionUserID(i *discordgo.InteractionCreate) string {
	if i == nil {
		return ""
	}
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}
