package listeners

import (
	"log"

	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/database"
	dashboard "github.com/hxnx/tunebot/internal/features/dashboard"
	"github.com/hxnx/tunebot/internal/music"
)

func HandleVoiceStateUpdate(s *discordgo.Session, vs *discordgo.VoiceStateUpdate) {
	if s == nil || vs == nil || vs.GuildID == "" {
		return
	}

	botID := ""
	if s.State != nil && s.State.User != nil {
		botID = s.State.User.ID
	}
	if botID == "" {
		return
	}

	guild := getGuildWithVoiceStates(s, vs.GuildID)
	if guild == nil {
		return
	}

	botChannelID := ""
	for _, state := range guild.VoiceStates {
		if state.UserID == botID && state.ChannelID != "" {
			botChannelID = state.ChannelID
			break
		}
	}
	if botChannelID == "" {
		return
	}

	hasOtherUser := false
	for _, state := range guild.VoiceStates {
		if state.ChannelID != botChannelID || state.UserID == botID {
			continue
		}
		hasOtherUser = true
		break
	}

	if hasOtherUser {
		return
	}

	player := music.DefaultPlayerManager.Get(vs.GuildID)
	if err := player.Stop(true); err != nil {
		return
	}
	if err := dashboard.UpdateDashboardByGuild(s, vs.GuildID); err != nil {
		log.Printf("failed to update dashboard after auto-stop: %v", err)
	}

	repo := database.NewGuildRepository()
	channelID, _, ok, err := repo.GetDashboardEntry(vs.GuildID)
	if err != nil || !ok || channelID == "" {
		return
	}

	embed := &discordgo.MessageEmbed{
		Description: "🔇 음성 채널에 유저가 없어 재생을 종료했습니다.",
		Color:       0x3C6AA1,
	}
	if _, err := s.ChannelMessageSendEmbed(channelID, embed); err != nil {
		log.Printf("failed to send voice-empty notice: %v", err)
	}
}

func getGuildWithVoiceStates(s *discordgo.Session, guildID string) *discordgo.Guild {
	if s.State != nil {
		if g, err := s.State.Guild(guildID); err == nil {
			return g
		}
	}
	g, err := s.Guild(guildID)
	if err != nil {
		return nil
	}
	return g
}
