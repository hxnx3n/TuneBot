package ping

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

func BuildPingComponentsV2(s *discordgo.Session) []discordgo.MessageComponent {
	latency := s.HeartbeatLatency().Round(time.Millisecond)
	apiLatency := latency

	gatewayLatency := latency
	if !s.LastHeartbeatAck.IsZero() {
		gatewayLatency = time.Since(s.LastHeartbeatAck).Round(time.Millisecond)
	}

	guilds := len(s.State.Guilds)
	shards := s.ShardCount
	if shards == 0 {
		shards = 1
	}

	colorLilac := 0xC8A2C8
	divider := true
	spacing := discordgo.SeparatorSpacingSizeSmall

	return []discordgo.MessageComponent{
		discordgo.Container{
			AccentColor: &colorLilac,
			Components: []discordgo.MessageComponent{
				discordgo.TextDisplay{Content: "**퐁!**"},
				discordgo.TextDisplay{Content: "현재 상태를 확인해 주세요."},
				discordgo.Separator{Divider: &divider, Spacing: &spacing},
				discordgo.Section{
					Components: []discordgo.MessageComponent{
						discordgo.TextDisplay{Content: fmt.Sprintf("**API 지연:** %s", apiLatency)},
						discordgo.TextDisplay{Content: fmt.Sprintf("**게이트웨이 지연:** %s", gatewayLatency)},
						discordgo.TextDisplay{Content: fmt.Sprintf("**서버 수:** %d • **샤드 수:** %d", guilds, shards)},
					},
					Accessory: discordgo.Button{
						Style:    discordgo.PrimaryButton,
						Label:    "새로고침",
						CustomID: "ping_refresh",
					},
				},
				discordgo.TextDisplay{Content: fmt.Sprintf("갱신됨 <t:%d:R>", time.Now().Unix())},
			},
		},
	}
}

func RespondPing(s *discordgo.Session, i *discordgo.InteractionCreate, respType discordgo.InteractionResponseType) {
	if s == nil || i == nil {
		return
	}

	components := BuildPingComponentsV2(s)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: respType,
		Data: &discordgo.InteractionResponseData{
			Components: components,
			Flags:      discordgo.MessageFlagsIsComponentsV2 | discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("failed to respond to ping: %v", err)
	}
}
