package listeners

import (
	"errors"
	"log"

	"github.com/bwmarrin/discordgo"
	dashboard "github.com/hxnx/tunebot/internal/features/dashboard"
	"github.com/hxnx/tunebot/internal/music"
)

func handleDashboardJoin(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.GuildID == "" {
		dashboard.RespondEphemeral(s, i, "이 버튼은 서버에서만 사용할 수 있습니다.")
		return
	}

	userID := getInteractionUserID(i)
	if userID == "" {
		dashboard.RespondEphemeral(s, i, "사용자 정보를 확인할 수 없습니다.")
		return
	}

	player := music.DefaultPlayerManager.Get(i.GuildID)
	if player.HasVoiceConnection() {
		if err := player.Stop(false); err != nil && !errors.Is(err, music.ErrPlaybackStopped) {
			log.Printf("dashboard join: failed to leave voice channel: %v", err)
			dashboard.RespondEphemeral(s, i, "음성 채널 퇴장에 실패했습니다.")
			return
		}
		if err := dashboard.UpdateDashboardByGuild(s, i.GuildID); err != nil {
			log.Printf("dashboard join: failed to update dashboard after leave: %v", err)
		}
		dashboard.RespondEphemeral(s, i, "음성 채널에서 퇴장했습니다.")
		return
	}

	channelID, err := findUserVoiceChannel(s, i.GuildID, userID)
	if err != nil {
		if errors.Is(err, errNoVoiceChannel) {
			dashboard.RespondEphemeral(s, i, "먼저 음성 채널에 접속해 주세요.")
			return
		}
		log.Printf("dashboard join: failed to find voice channel: %v", err)
		dashboard.RespondEphemeral(s, i, "음성 채널 정보를 확인할 수 없습니다.")
		return
	}

	if err := player.JoinVoice(s, channelID); err != nil {
		log.Printf("dashboard join: failed to join voice channel: %v", err)
		dashboard.RespondEphemeral(s, i, "음성 채널 참가에 실패했습니다.")
		return
	}

	if err := dashboard.UpdateDashboardByGuild(s, i.GuildID); err != nil {
		log.Printf("dashboard join: failed to update dashboard after join: %v", err)
	}
	dashboard.RespondEphemeral(s, i, "음성 채널에 참가했습니다.")
}
