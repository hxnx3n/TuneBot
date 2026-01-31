package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/internal/database"
	"github.com/hxnx/tunebot/internal/music"
	internalredis "github.com/hxnx/tunebot/internal/redis"
	redislib "github.com/redis/go-redis/v9"
)

const DefaultDashboardChannelName = "🎵-tunebot"

type DashboardEntry struct {
	ChannelID string
	MessageID string
}

var dashboardState = struct {
	mu      sync.RWMutex
	byGuild map[string]DashboardEntry
}{
	byGuild: make(map[string]DashboardEntry),
}

var nowPlayingUpdater = struct {
	mu     sync.Mutex
	cancel map[string]context.CancelFunc
}{
	cancel: make(map[string]context.CancelFunc),
}

type dashboardRenderState struct {
	lastHash    string
	lastUpdated time.Time
}

var dashboardRenderCache = struct {
	mu      sync.Mutex
	byGuild map[string]dashboardRenderState
}{
	byGuild: make(map[string]dashboardRenderState),
}

type dashboardStoreCacheEntry struct {
	settings        music.QueueSettings
	queueCount      int64
	settingsExpires time.Time
	queueExpires    time.Time
	hasSettings     bool
	hasQueueCount   bool
}

var dashboardStoreCache = struct {
	mu      sync.Mutex
	byGuild map[string]dashboardStoreCacheEntry
}{
	byGuild: make(map[string]dashboardStoreCacheEntry),
}

const (
	dashboardUpdateTickerInterval = 8 * time.Second
	dashboardAutoUpdateBucket     = 8 * time.Second
	dashboardMinUpdateInterval    = 5 * time.Second
	dashboardSettingsCacheTTL     = 30 * time.Second
	dashboardQueueCacheTTL        = 15 * time.Second
	dashboardSettingsCacheKey     = "dashboard:cache:settings:"
	dashboardQueueCacheKey        = "dashboard:cache:queue:"
)

func GetDashboardEntry(guildID string) (DashboardEntry, bool) {
	dashboardState.mu.RLock()
	defer dashboardState.mu.RUnlock()
	entry, ok := dashboardState.byGuild[guildID]
	return entry, ok
}

func SetDashboardEntry(guildID string, entry DashboardEntry) {
	dashboardState.mu.Lock()
	dashboardState.byGuild[guildID] = entry
	dashboardState.mu.Unlock()
}

func ClearDashboardEntry(guildID string) {
	stopDashboardAutoUpdater(guildID)
	clearDashboardRenderState(guildID)
	clearDashboardStoreCache(guildID)
	dashboardState.mu.Lock()
	delete(dashboardState.byGuild, guildID)
	dashboardState.mu.Unlock()
}

func DeletePreviousDashboard(s *discordgo.Session, guildID string) error {
	if s == nil || guildID == "" {
		return fmt.Errorf("invalid dashboard delete parameters")
	}

	entry, ok := GetDashboardEntry(guildID)
	if !ok || entry.ChannelID == "" || entry.MessageID == "" {
		return nil
	}

	return s.ChannelMessageDelete(entry.ChannelID, entry.MessageID)
}

type dashboardSnapshot struct {
	LoopLabel           string
	QueueCount          int64
	NowPlayingTitle     string
	NowPlayingStatus    string
	NowPlayingMeta      string
	NowPlayingRequester string
	NowPlayingProgress  string
	NowPlayingThumb     string
	HasTrack            bool
	IsPaused            bool
	IsVoiceConnected    bool
}

func BuildDashboardComponents(guildID string) []discordgo.MessageComponent {
	snapshot := buildDashboardSnapshot(guildID)
	return buildDashboardComponentsFromSnapshot(snapshot)
}

func buildDashboardSnapshot(guildID string) dashboardSnapshot {
	snapshot := dashboardSnapshot{
		LoopLabel:          "꺼짐",
		QueueCount:         0,
		NowPlayingTitle:    "🟡 **재생 중인 곡이 없습니다**",
		NowPlayingStatus:   "",
		NowPlayingMeta:     "",
		NowPlayingProgress: "",
		NowPlayingThumb:    "",
		HasTrack:           false,
		IsPaused:           false,
		IsVoiceConnected:   false,
	}

	if guildID == "" {
		return snapshot
	}

	settings, hasSettings := getCachedSettings(guildID)
	if hasSettings {
		switch settings.RepeatMode {
		case music.RepeatModeTrack:
			snapshot.LoopLabel = "곡 반복"
		case music.RepeatModeQueue:
			snapshot.LoopLabel = "대기열 반복"
		default:
			snapshot.LoopLabel = "꺼짐"
		}
	}

	if count, ok := getCachedQueueCount(guildID); ok {
		snapshot.QueueCount = count
	}

	player := music.DefaultPlayerManager.Get(guildID)
	state := player.State()
	snapshot.IsVoiceConnected = player.HasVoiceConnection()
	if state.IsPlaying && state.Track != nil {
		snapshot.HasTrack = true
		snapshot.IsPaused = state.PausedAt != nil

		title := strings.TrimSpace(state.Track.Title)
		if title == "" {
			title = "알 수 없는 제목"
		}
		safeTitle := escapeDashboardText(title)
		if state.Track.URL != "" {
			snapshot.NowPlayingTitle = fmt.Sprintf("🎧 **[%s](%s)**", safeTitle, state.Track.URL)
		} else {
			snapshot.NowPlayingTitle = fmt.Sprintf("🎧 **%s**", safeTitle)
		}

		if snapshot.IsPaused {
			snapshot.NowPlayingStatus = "⏸️ **일시정지됨**"
		} else {
			snapshot.NowPlayingStatus = "▶️ **재생 중**"
		}

		source := strings.TrimSpace(string(state.Track.Source))
		sourceKey := strings.ToLower(source)
		label := "알 수 없음"
		switch sourceKey {
		case "youtube":
			label = "유튜브"
		case "spotify":
			label = "스포티파이"
		case "soundcloud":
			label = "사운드클라우드"
		default:
			if sourceKey != "" {
				label = strings.ToUpper(sourceKey)
			}
		}

		snapshot.NowPlayingMeta = fmt.Sprintf("🎵 %s", label)

		requester := strings.TrimSpace(state.Track.RequestedBy)
		if requester != "" {
			snapshot.NowPlayingRequester = fmt.Sprintf("`요청자: <@%s>`", requester)
		}

		progressBar := buildProgressBar(state.Position, state.Track.Duration, 12)
		snapshot.NowPlayingProgress = fmt.Sprintf("`%s` `%s` `%s`", formatDuration(state.Position), progressBar, formatDuration(state.Track.Duration))

		snapshot.NowPlayingThumb = strings.TrimSpace(state.Track.Thumbnail)
	}

	return snapshot
}

func buildDashboardComponentsFromSnapshot(snapshot dashboardSnapshot) []discordgo.MessageComponent {
	accent := 0x3C6AA1
	divider := true
	spacing := discordgo.SeparatorSpacingSizeSmall

	components := []discordgo.MessageComponent{
		discordgo.TextDisplay{Content: "▶️ **현재 재생 중**"},
		discordgo.Separator{Divider: &divider, Spacing: &spacing},
	}

	nowPlayingComponents := []discordgo.MessageComponent{
		discordgo.TextDisplay{Content: snapshot.NowPlayingTitle},
	}
	if snapshot.NowPlayingProgress != "" {
		nowPlayingComponents = append(nowPlayingComponents, discordgo.TextDisplay{Content: snapshot.NowPlayingProgress})
	}

	if snapshot.NowPlayingThumb != "" {
		nowPlayingSection := discordgo.Section{
			Components: nowPlayingComponents,
			Accessory: discordgo.Thumbnail{
				Media: discordgo.UnfurledMediaItem{URL: snapshot.NowPlayingThumb},
			},
		}
		components = append(components, nowPlayingSection)
	} else {
		components = append(components, nowPlayingComponents...)
	}

	pauseLabel := "일시정지"
	pauseStyle := discordgo.SecondaryButton
	if snapshot.IsPaused {
		pauseLabel = "재개"
		pauseStyle = discordgo.SuccessButton
	}
	pauseDisabled := !snapshot.HasTrack
	joinLabel := "참가"
	if snapshot.IsVoiceConnected {
		joinLabel = "퇴장"
	}

	components = append(components,
		discordgo.Separator{Divider: &divider, Spacing: &spacing},
		discordgo.TextDisplay{Content: fmt.Sprintf("🔁 반복 **%s**", snapshot.LoopLabel)},
		discordgo.TextDisplay{Content: fmt.Sprintf("📋 대기열 **%d곡**", snapshot.QueueCount)},
	)
	statusMeta := []string{}
	if snapshot.NowPlayingStatus != "" {
		statusMeta = append(statusMeta, snapshot.NowPlayingStatus)
	}
	if len(statusMeta) > 0 {
		components = append(components, discordgo.TextDisplay{Content: strings.Join(statusMeta, " • ")})
	}
	if snapshot.NowPlayingRequester != "" {
		components = append(components, discordgo.TextDisplay{Content: snapshot.NowPlayingRequester})
	}
	components = append(components,
		discordgo.Separator{Divider: &divider, Spacing: &spacing},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Style:    discordgo.SuccessButton,
					Label:    joinLabel,
					CustomID: "dashboard_join",
				},
				discordgo.Button{
					Style:    discordgo.PrimaryButton,
					Label:    "검색",
					CustomID: "dashboard_search",
				},
				discordgo.Button{
					Style:    pauseStyle,
					Label:    pauseLabel,
					CustomID: "dashboard_pause",
					Disabled: pauseDisabled,
				},
				discordgo.Button{
					Style:    discordgo.SecondaryButton,
					Label:    "대기열",
					CustomID: "dashboard_queue",
				},
				discordgo.Button{
					Style:    discordgo.SecondaryButton,
					Label:    "반복",
					CustomID: "dashboard_loop",
				},
			},
		},
	)

	return []discordgo.MessageComponent{
		discordgo.Container{
			AccentColor: &accent,
			Components:  components,
		},
	}
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "실시간"
	}
	totalSeconds := int(d.Seconds())
	min := totalSeconds / 60
	sec := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d", min, sec)
}

func buildProgressBar(position, duration time.Duration, size int) string {
	if size <= 0 {
		size = 12
	}
	if duration <= 0 {
		return "○" + strings.Repeat("─", size)
	}
	ratio := float64(position) / float64(duration)
	ratio = max(0.0, min(1.0, ratio))
	marker := int(ratio * float64(size))
	marker = min(size, max(0, marker))
	left := strings.Repeat("━", marker)
	right := strings.Repeat("─", size-marker)
	return fmt.Sprintf("%s◉%s", left, right)
}

func escapeDashboardText(text string) string {
	replacer := strings.NewReplacer(
		"*", "\\*",
		"_", "\\_",
		"`", "\\`",
		"~", "\\~",
		"|", "\\|",
		">", "\\>",
	)
	return replacer.Replace(text)
}

func buildPlaybackHash(guildID string, state music.PlaybackState) string {
	if !state.IsPlaying || state.Track == nil {
		return "stopped"
	}
	bucket := int(state.Position / dashboardAutoUpdateBucket)
	paused := state.PausedAt != nil

	if loopLabel, queueCount, ok := peekDashboardStoreSummary(guildID); ok {
		return fmt.Sprintf("playing:%s:%d:paused=%t:loop=%s:queue=%d", state.Track.ID, bucket, paused, loopLabel, queueCount)
	}

	return fmt.Sprintf("playing:%s:%d:%t", state.Track.ID, bucket, paused)
}

func shouldAutoUpdateDashboard(guildID string, state music.PlaybackState) bool {
	if guildID == "" {
		return false
	}
	hash := buildPlaybackHash(guildID, state)

	dashboardRenderCache.mu.Lock()
	prev, ok := dashboardRenderCache.byGuild[guildID]
	dashboardRenderCache.mu.Unlock()

	if ok && prev.lastHash == hash && time.Since(prev.lastUpdated) < dashboardMinUpdateInterval {
		return false
	}

	return !ok || prev.lastHash != hash
}

func recordDashboardRenderState(guildID string, state music.PlaybackState) {
	if guildID == "" {
		return
	}
	hash := buildPlaybackHash(guildID, state)

	dashboardRenderCache.mu.Lock()
	dashboardRenderCache.byGuild[guildID] = dashboardRenderState{
		lastHash:    hash,
		lastUpdated: time.Now(),
	}
	dashboardRenderCache.mu.Unlock()
}

func clearDashboardRenderState(guildID string) {
	if guildID == "" {
		return
	}
	dashboardRenderCache.mu.Lock()
	delete(dashboardRenderCache.byGuild, guildID)
	dashboardRenderCache.mu.Unlock()
}

func getRedisClient() *redislib.Client {
	return internalredis.Client()
}

func getRedisSettingsCache(guildID string) (music.QueueSettings, bool) {
	if guildID == "" {
		return music.QueueSettings{}, false
	}
	client := getRedisClient()
	if client == nil {
		return music.QueueSettings{}, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	raw, err := client.Get(ctx, dashboardSettingsCacheKey+guildID).Result()
	if err != nil {
		return music.QueueSettings{}, false
	}

	var settings music.QueueSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return music.QueueSettings{}, false
	}

	return settings, true
}

func setRedisSettingsCache(guildID string, settings music.QueueSettings) {
	if guildID == "" {
		return
	}
	client := getRedisClient()
	if client == nil {
		return
	}

	payload, err := json.Marshal(settings)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = client.Set(ctx, dashboardSettingsCacheKey+guildID, payload, dashboardSettingsCacheTTL).Err()
}

func getRedisQueueCountCache(guildID string) (int64, bool) {
	if guildID == "" {
		return 0, false
	}
	client := getRedisClient()
	if client == nil {
		return 0, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	raw, err := client.Get(ctx, dashboardQueueCacheKey+guildID).Result()
	if err != nil {
		return 0, false
	}

	count, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}

	return count, true
}

func setRedisQueueCountCache(guildID string, count int64) {
	if guildID == "" {
		return
	}
	client := getRedisClient()
	if client == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = client.Set(ctx, dashboardQueueCacheKey+guildID, strconv.FormatInt(count, 10), dashboardQueueCacheTTL).Err()
}

func clearRedisDashboardCache(guildID string) {
	if guildID == "" {
		return
	}
	client := getRedisClient()
	if client == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = client.Del(ctx, dashboardSettingsCacheKey+guildID, dashboardQueueCacheKey+guildID).Err()
}

func getCachedSettings(guildID string) (music.QueueSettings, bool) {
	if guildID == "" {
		return music.QueueSettings{}, false
	}

	if settings, ok := getRedisSettingsCache(guildID); ok {
		return settings, true
	}

	dashboardStoreCache.mu.Lock()
	entry := dashboardStoreCache.byGuild[guildID]
	dashboardStoreCache.mu.Unlock()

	if entry.hasSettings && time.Now().Before(entry.settingsExpires) {
		return entry.settings, true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	store := music.NewQueueStoreFromDefault()
	if store == nil {
		if entry.hasSettings {
			return entry.settings, true
		}
		return music.QueueSettings{}, false
	}

	settings, err := store.GetSettings(ctx, guildID)
	if err != nil {
		if entry.hasSettings {
			return entry.settings, true
		}
		return music.QueueSettings{}, false
	}

	dashboardStoreCache.mu.Lock()
	entry.settings = settings
	entry.settingsExpires = time.Now().Add(dashboardSettingsCacheTTL)
	entry.hasSettings = true
	dashboardStoreCache.byGuild[guildID] = entry
	dashboardStoreCache.mu.Unlock()

	setRedisSettingsCache(guildID, settings)

	return settings, true
}

func getCachedQueueCount(guildID string) (int64, bool) {
	if guildID == "" {
		return 0, false
	}

	if count, ok := getRedisQueueCountCache(guildID); ok {
		return count, true
	}

	dashboardStoreCache.mu.Lock()
	entry := dashboardStoreCache.byGuild[guildID]
	dashboardStoreCache.mu.Unlock()

	if entry.hasQueueCount && time.Now().Before(entry.queueExpires) {
		return entry.queueCount, true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	store := music.NewQueueStoreFromDefault()
	if store == nil {
		if entry.hasQueueCount {
			return entry.queueCount, true
		}
		return 0, false
	}

	count, err := store.QueueSize(ctx, guildID)
	if err != nil {
		if entry.hasQueueCount {
			return entry.queueCount, true
		}
		return 0, false
	}

	dashboardStoreCache.mu.Lock()
	entry.queueCount = count
	entry.queueExpires = time.Now().Add(dashboardQueueCacheTTL)
	entry.hasQueueCount = true
	dashboardStoreCache.byGuild[guildID] = entry
	dashboardStoreCache.mu.Unlock()

	setRedisQueueCountCache(guildID, count)

	return count, true
}

func clearDashboardStoreCache(guildID string) {
	if guildID == "" {
		return
	}
	clearRedisDashboardCache(guildID)
	dashboardStoreCache.mu.Lock()
	delete(dashboardStoreCache.byGuild, guildID)
	dashboardStoreCache.mu.Unlock()
}

func peekDashboardStoreSummary(guildID string) (string, int64, bool) {
	if guildID == "" {
		return "", 0, false
	}

	settings, hasSettings := getRedisSettingsCache(guildID)
	queueCount, hasQueueCount := getRedisQueueCountCache(guildID)

	if !hasSettings || !hasQueueCount {
		dashboardStoreCache.mu.Lock()
		entry := dashboardStoreCache.byGuild[guildID]
		dashboardStoreCache.mu.Unlock()

		now := time.Now()
		if !hasSettings && entry.hasSettings && now.Before(entry.settingsExpires) {
			settings = entry.settings
			hasSettings = true
		}
		if !hasQueueCount && entry.hasQueueCount && now.Before(entry.queueExpires) {
			queueCount = entry.queueCount
			hasQueueCount = true
		}
	}

	loopLabel := ""
	ok := false

	if hasSettings {
		switch settings.RepeatMode {
		case music.RepeatModeTrack:
			loopLabel = "곡 반복"
		case music.RepeatModeQueue:
			loopLabel = "대기열 반복"
		default:
			loopLabel = "꺼짐"
		}
		ok = true
	}

	if hasQueueCount {
		ok = true
	}

	return loopLabel, queueCount, ok
}

func UpdateDashboardSettingsCache(guildID string, settings music.QueueSettings) {
	if guildID == "" {
		return
	}
	dashboardStoreCache.mu.Lock()
	entry := dashboardStoreCache.byGuild[guildID]
	entry.settings = settings
	entry.settingsExpires = time.Now().Add(dashboardSettingsCacheTTL)
	entry.hasSettings = true
	dashboardStoreCache.byGuild[guildID] = entry
	dashboardStoreCache.mu.Unlock()
	setRedisSettingsCache(guildID, settings)
}

func UpdateDashboardQueueCountCache(guildID string, count int64) {
	if guildID == "" {
		return
	}
	dashboardStoreCache.mu.Lock()
	entry := dashboardStoreCache.byGuild[guildID]
	entry.queueCount = count
	entry.queueExpires = time.Now().Add(dashboardQueueCacheTTL)
	entry.hasQueueCount = true
	dashboardStoreCache.byGuild[guildID] = entry
	dashboardStoreCache.mu.Unlock()
	setRedisQueueCountCache(guildID, count)
}

func RespondUpdateDashboardMessage(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if s == nil || i == nil {
		return
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Components: BuildDashboardComponents(i.GuildID),
			Flags:      discordgo.MessageFlagsIsComponentsV2,
		},
	})
	if err != nil {
		log.Printf("failed to update dashboard message: %v", err)
	}
}

func UpdateDashboardByGuild(s *discordgo.Session, guildID string) error {
	if s == nil || guildID == "" {
		return fmt.Errorf("invalid dashboard update parameters")
	}

	entry, ok := GetDashboardEntry(guildID)
	if !ok || entry.ChannelID == "" || entry.MessageID == "" {
		repo := database.NewGuildRepository()
		channelID, messageID, repoOK, err := repo.GetDashboardEntry(guildID)
		if err != nil {
			return fmt.Errorf("failed to load dashboard entry: %w", err)
		}
		if !repoOK || channelID == "" || messageID == "" {
			return fmt.Errorf("dashboard message not found")
		}
		entry = DashboardEntry{ChannelID: channelID, MessageID: messageID}
		SetDashboardEntry(guildID, entry)
	}

	state := music.DefaultPlayerManager.Get(guildID).State()
	if state.IsPlaying && state.Track != nil {
		startDashboardAutoUpdater(s, guildID)
	} else {
		stopDashboardAutoUpdater(guildID)
	}

	components := BuildDashboardComponents(guildID)

	_, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:         entry.MessageID,
		Channel:    entry.ChannelID,
		Components: &components,
		Flags:      discordgo.MessageFlagsIsComponentsV2,
	})
	if err == nil {
		recordDashboardRenderState(guildID, state)
	}
	return err
}

func startDashboardAutoUpdater(s *discordgo.Session, guildID string) {
	if s == nil || guildID == "" {
		return
	}

	nowPlayingUpdater.mu.Lock()
	if _, exists := nowPlayingUpdater.cancel[guildID]; exists {
		nowPlayingUpdater.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	nowPlayingUpdater.cancel[guildID] = cancel
	nowPlayingUpdater.mu.Unlock()

	go func() {
		ticker := time.NewTicker(dashboardUpdateTickerInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state := music.DefaultPlayerManager.Get(guildID).State()
				if state.IsPlaying && state.Track != nil {
					if shouldAutoUpdateDashboard(guildID, state) {
						if err := UpdateDashboardByGuild(s, guildID); err != nil {
							log.Printf("dashboard auto-update failed: %v", err)
						}
					}
					continue
				}
				stopDashboardAutoUpdater(guildID)
				_ = UpdateDashboardByGuild(s, guildID)
				return
			}
		}
	}()
}

func stopDashboardAutoUpdater(guildID string) {
	if guildID == "" {
		return
	}

	nowPlayingUpdater.mu.Lock()
	cancel, ok := nowPlayingUpdater.cancel[guildID]
	if ok {
		delete(nowPlayingUpdater.cancel, guildID)
	}
	nowPlayingUpdater.mu.Unlock()

	if ok && cancel != nil {
		cancel()
	}
}

func RespondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if s == nil || i == nil {
		return
	}

	var accentColor = 0xC9A0FF
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
