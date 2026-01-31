package commands

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const syncOwnerID = "1447268601496338588"

func HandleSyncMessage(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	if s == nil || m == nil || m.Author == nil {
		return false
	}
	if m.Author.Bot {
		return false
	}
	if m.GuildID == "" {
		return false
	}

	if strings.TrimSpace(m.Content) != "!sync" {
		return false
	}

	if m.Author.ID != syncOwnerID {
		_, _ = s.ChannelMessageSend(m.ChannelID, "이 명령어는 봇 소유자만 사용할 수 있습니다.")
		return true
	}

	appID := ""
	if s.State != nil && s.State.User != nil {
		appID = s.State.User.ID
	}
	if appID == "" {
		_, _ = s.ChannelMessageSend(m.ChannelID, "슬래시 커맨드 동기화 실패: 애플리케이션 ID를 확인할 수 없습니다.")
		return true
	}

	if _, err := RegisterCommands(s, appID, m.GuildID); err != nil {
		_, _ = s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("슬래시 커맨드 동기화 실패: %v", err))
		return true
	}

	_, _ = s.ChannelMessageSend(m.ChannelID, "슬래시 커맨드를 이 서버에 동기화했습니다.")
	return true
}
