package commands

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/database"
	dashboard "github.com/hxnx/tunebot/internal/features/dashboard"
)

func SetupDashboard(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.GuildID == "" {
		dashboard.RespondEphemeral(s, i, "이 명령어는 서버에서만 사용하실 수 있습니다.")
		return
	}

	if !hasManageChannelsPermission(i) {
		dashboard.RespondEphemeral(s, i, "채널을 설정할 권한이 없습니다.")
		return
	}

	categoryID, channelName := parseSetupOptions(i)
	if channelName == "" {
		channelName = dashboard.DefaultDashboardChannelName
	}

	channelID := ""
	if categoryID == "" && channelName == dashboard.DefaultDashboardChannelName {
		if entry, ok := dashboard.GetDashboardEntry(i.GuildID); ok && entry.ChannelID != "" {
			if _, err := s.Channel(entry.ChannelID); err == nil {
				channelID = entry.ChannelID
			}
		}
	}

	if channelID == "" {
		channels, err := s.GuildChannels(i.GuildID)
		if err == nil {
			for _, ch := range channels {
				if ch.Type == discordgo.ChannelTypeGuildText && ch.Name == channelName {
					if categoryID == "" || ch.ParentID == categoryID {
						channelID = ch.ID
						break
					}
				}
			}
		}
	}

	if channelID == "" {
		channel, err := s.GuildChannelCreateComplex(i.GuildID, discordgo.GuildChannelCreateData{
			Name:     channelName,
			Type:     discordgo.ChannelTypeGuildText,
			ParentID: categoryID,
		})
		if err != nil {
			log.Printf("failed to create dashboard channel: %v", err)
			dashboard.RespondEphemeral(s, i, "대시보드 채널 생성에 실패했습니다.")
			return
		}
		channelID = channel.ID
	}

	if err := dashboard.DeletePreviousDashboard(s, i.GuildID); err != nil {
		log.Printf("failed to delete previous dashboard message: %v", err)
	}

	dashboardMessage, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Components: dashboard.BuildDashboardComponents(i.GuildID),
		Flags:      discordgo.MessageFlagsIsComponentsV2,
	})

	if err != nil {
		log.Printf("failed to send dashboard message: %v", err)
		dashboard.RespondEphemeral(s, i, "대시보드 메시지 생성에 실패했습니다.")
		return
	}

	dashboard.SetDashboardEntry(i.GuildID, dashboard.DashboardEntry{
		ChannelID: channelID,
		MessageID: dashboardMessage.ID,
	})

	repo := database.NewGuildRepository()
	if err := repo.UpsertDashboardEntry(i.GuildID, channelID, dashboardMessage.ID); err != nil {
		log.Printf("failed to save dashboard entry: %v", err)
	}

	if err := dashboard.UpdateDashboardByGuild(s, i.GuildID); err != nil {
		log.Printf("failed to start dashboard updater: %v", err)
	}

	dashboard.RespondEphemeral(s, i, fmt.Sprintf("대시보드 채널을 설정했습니다.\n<#%s>", channelID))
}

func parseSetupOptions(i *discordgo.InteractionCreate) (string, string) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return "", ""
	}

	data := i.ApplicationCommandData()
	var categoryID string
	var channelName string

	for _, opt := range data.Options {
		switch opt.Name {
		case "category":
			categoryID = opt.StringValue()
		case "channelName", "channel_name":
			channelName = opt.StringValue()
		}
	}

	return categoryID, channelName
}

func hasManageChannelsPermission(i *discordgo.InteractionCreate) bool {
	if i.Member == nil {
		return false
	}

	perms := i.Member.Permissions
	return perms&discordgo.PermissionManageChannels != 0
}
