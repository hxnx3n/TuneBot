package botinfo

import (
	"fmt"
	"log"
	"runtime"
	"runtime/metrics"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var botStartedAt = time.Now()

var cpuSampleMu sync.Mutex
var cpuSample = []metrics.Sample{{Name: "/process/cpu:seconds"}}
var lastCPUSeconds float64
var lastCPUTime time.Time

func cpuUsagePercent() (float64, bool) {
	cpuSampleMu.Lock()
	defer cpuSampleMu.Unlock()

	metrics.Read(cpuSample)
	v := cpuSample[0].Value
	if v.Kind() != metrics.KindFloat64 {
		return 0, false
	}

	seconds := v.Float64()
	now := time.Now()
	if lastCPUTime.IsZero() {
		lastCPUTime = now
		lastCPUSeconds = seconds
		return 0, false
	}

	elapsed := now.Sub(lastCPUTime).Seconds()
	if elapsed <= 0 {
		lastCPUTime = now
		lastCPUSeconds = seconds
		return 0, false
	}

	delta := seconds - lastCPUSeconds
	lastCPUTime = now
	lastCPUSeconds = seconds
	if delta < 0 {
		return 0, false
	}

	usage := (delta / elapsed) / float64(runtime.NumCPU()) * 100
	if usage < 0 {
		usage = 0
	} else if usage > 100 {
		usage = 100
	}
	return usage, true
}

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

	currentShard := 1
	if s.ShardID >= 0 {
		currentShard = s.ShardID + 1
	}
	if currentShard > shards {
		currentShard = shards
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	uptime := time.Since(botStartedAt).Round(time.Second)

	cpuUsage, cpuOK := cpuUsagePercent()
	cpuUsageText := "측정중"
	if cpuOK {
		cpuUsageText = fmt.Sprintf("%.1f%%", cpuUsage)
	}

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
						discordgo.TextDisplay{Content: fmt.Sprintf("**서버 수:** %d", guilds)},
						discordgo.TextDisplay{Content: fmt.Sprintf("**현재 샤드 / 총 샤드:** %d / %d", currentShard, shards)},
						discordgo.TextDisplay{Content: fmt.Sprintf("**업타임:** %s", uptime)},
						discordgo.TextDisplay{Content: fmt.Sprintf("**CPU 사용량:** %s", cpuUsageText)},
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
