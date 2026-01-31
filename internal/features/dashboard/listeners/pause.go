package listeners

import (
	"errors"
	"log"

	"github.com/bwmarrin/discordgo"
	dashboard "github.com/hxnx/tunebot/internal/features/dashboard"
	"github.com/hxnx/tunebot/internal/music"
)

func handleDashboardPause(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.GuildID == "" {
		dashboard.RespondEphemeral(s, i, "이 버튼은 서버에서만 사용할 수 있습니다.")
		return
	}

	userID := getInteractionUserID(i)
	if userID == "" {
		dashboard.RespondEphemeral(s, i, "사용자 정보를 확인할 수 없습니다.")
		return
	}

	_, err := findUserVoiceChannel(s, i.GuildID, userID)
	if err != nil {
		if errors.Is(err, errNoVoiceChannel) {
			dashboard.RespondEphemeral(s, i, "먼저 음성 채널에 접속해 주세요.")
			return
		}
		log.Printf("dashboard pause: failed to find voice channel: %v", err)
		dashboard.RespondEphemeral(s, i, "음성 채널 정보를 확인할 수 없습니다.")
		return
	}

	player := music.DefaultPlayerManager.Get(i.GuildID)
	state := player.State()
	if !state.IsPlaying || state.Track == nil {
		dashboard.RespondEphemeral(s, i, "재생 중인 곡이 없습니다.")
		return
	}

	if err := player.TogglePause(); err != nil {
		log.Printf("dashboard pause: toggle failed: %v", err)
		dashboard.RespondEphemeral(s, i, "일시정지/재개에 실패했습니다.")
		return
	}

	dashboard.RespondUpdateDashboardMessage(s, i)
}
