package botinfo

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/bwmarrin/discordgo"
)

var botStartedAt = time.Now()

func BuildBotInfoComponents(s *discordgo.Session) []discordgo.MessageComponent {
	latency := s.HeartbeatLatency().Round(time.Millisecond)
	apiLatency := latency

	gatewayLatency := latency
	if !s.LastHeartbeatAck.IsZero() {
		gatewayLatency = time.Since(s.LastHeartbeatAck).Round(time.Millisecond)
	}

	guilds := 0
	if s.State != nil {
		guilds = len(s.State.Guilds)
	}

	shards := s.ShardCount
	if shards == 0 {
		shards = 1
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	uptime := time.Since(botStartedAt).Round(time.Second)

	colorLilac := 0xC8A2C8
	divider := true
	spacing := discordgo.SeparatorSpacingSizeSmall

	return []discordgo.MessageComponent{
		discordgo.Container{
			AccentColor: &colorLilac,
			Components: []discordgo.MessageComponent{
				discordgo.TextDisplay{Content: "**TuneBot 정보**"},
				discordgo.TextDisplay{Content: "현재 상태를 확인해 주세요."},
				discordgo.Separator{Divider: &divider, Spacing: &spacing},
				discordgo.Section{
					Components: []discordgo.MessageComponent{
						discordgo.TextDisplay{Content: fmt.Sprintf("**API 지연:** %s", apiLatency)},
						discordgo.TextDisplay{Content: fmt.Sprintf("**게이트웨이 지연:** %s", gatewayLatency)},
						discordgo.TextDisplay{Content: fmt.Sprintf("**서버 수:** %d • **샤드 수:** %d", guilds, shards)},
						discordgo.TextDisplay{Content: fmt.Sprintf("**업타임:** %s", uptime)},
						discordgo.TextDisplay{Content: fmt.Sprintf("**메모리 사용량:** %.2f MB", float64(mem.Alloc)/1024.0/1024.0)},
					},
				},
				discordgo.TextDisplay{Content: fmt.Sprintf("갱신됨 <t:%d:R>", time.Now().Unix())},
			},
		},
	}
}

func RespondBotInfo(s *discordgo.Session, i *discordgo.InteractionCreate, respType discordgo.InteractionResponseType) {
	if s == nil || i == nil {
		return
	}

	components := BuildBotInfoComponents(s)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: respType,
		Data: &discordgo.InteractionResponseData{
			Components: components,
			Flags:      discordgo.MessageFlagsIsComponentsV2 | discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("failed to respond to bot info: %v", err)
	}
}
