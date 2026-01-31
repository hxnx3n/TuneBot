package music

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	ErrNoVoiceChannel     = errors.New("user is not in a voice channel")
	ErrVoiceNotConnected  = errors.New("voice connection not established")
	ErrPlaybackInProgress = errors.New("playback already in progress")
	ErrPlaybackStopped    = errors.New("playback stopped")
	ErrPlaybackSkipped    = errors.New("playback skipped")
	ErrPlaybackRestarted  = errors.New("playback restarted")
)

var DefaultPlayerManager = NewPlayerManager(nil)

type PlayerManager struct {
	mu       sync.Mutex
	players  map[string]*Player
	service  *Service
	resolver *YTDLPResolver
}

func NewPlayerManager(service *Service) *PlayerManager {
	if service == nil {
		service = NewDefaultService()
	}
	return &PlayerManager{
		players:  make(map[string]*Player),
		service:  service,
		resolver: NewYTDLPResolver(),
	}
}

func (m *PlayerManager) WithSpotify(client *SpotifyClient) *PlayerManager {
	m.service.WithSpotify(client)
	return m
}

func (m *PlayerManager) Get(guildID string) *Player {
	m.mu.Lock()
	defer m.mu.Unlock()

	if p, ok := m.players[guildID]; ok {
		return p
	}

	p := &Player{
		guildID:   guildID,
		service:   m.service,
		resolver:  m.resolver,
		volume:    100,
		stopCh:    make(chan struct{}, 1),
		skipCh:    make(chan struct{}, 1),
		pauseCh:   make(chan struct{}, 1),
		resumeCh:  make(chan struct{}, 1),
		restartCh: make(chan struct{}, 1),
	}
	m.players[guildID] = p
	return p
}

type Player struct {
	guildID  string
	service  *Service
	resolver *YTDLPResolver
	volume   int

	mu      sync.Mutex
	session *discordgo.Session
	vc      *discordgo.VoiceConnection
	state   PlaybackState

	stopCh    chan struct{}
	skipCh    chan struct{}
	pauseCh   chan struct{}
	resumeCh  chan struct{}
	restartCh chan struct{}

	playCtx    context.Context
	playCancel context.CancelFunc

	ffmpegCmd    *exec.Cmd
	ffmpegStdout io.ReadCloser
	ffmpegCancel context.CancelFunc

	frameCount int64
	paused     bool

	wakeCh  chan struct{}
	cancel  context.CancelFunc
	running bool
}

func safeSpeaking(vc *discordgo.VoiceConnection, speaking bool) {
	if vc == nil || !vc.Ready {
		return
	}
	_ = vc.Speaking(speaking)
}

func (p *Player) HasVoiceConnection() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.vc != nil
}

func (p *Player) JoinVoice(s *discordgo.Session, channelID string) error {
	if s == nil {
		return fmt.Errorf("discord session is nil")
	}
	if channelID == "" {
		return fmt.Errorf("channel ID is empty")
	}

	vc, err := s.ChannelVoiceJoin(p.guildID, channelID, false, true)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.session = s
	p.vc = vc
	p.mu.Unlock()
	return nil
}

func (p *Player) EnqueueAndPlay(ctx context.Context, s *discordgo.Session, userID string, input string, sourceHint TrackSource, priority int) (QueueItem, error) {
	if p.service == nil {
		return QueueItem{}, ErrQueueStoreNil
	}
	if s == nil {
		return QueueItem{}, fmt.Errorf("discord session is nil")
	}
	p.session = s

	item, err := p.service.ResolveAndEnqueue(ctx, p.guildID, input, sourceHint, userID, priority)
	if err != nil {
		return QueueItem{}, err
	}

	if err := p.ensureVoiceConnection(userID); err != nil {
		return QueueItem{}, err
	}

	p.ensureWorker()
	p.signalWake()
	return item, nil
}

func (p *Player) Skip() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.skipCh == nil {
		return ErrPlaybackStopped
	}
	select {
	case p.skipCh <- struct{}{}:
	default:
	}
	return nil
}

func (p *Player) Stop(clearQueue bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopCh == nil {
		return ErrPlaybackStopped
	}
	select {
	case p.stopCh <- struct{}{}:
	default:
	}

	if clearQueue && p.service != nil {
		_ = p.service.Clear(context.Background(), p.guildID)
	}

	p.cleanupVoiceLocked()
	return nil
}

func (p *Player) TogglePause() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.paused {
		p.paused = false
		p.state.PausedAt = nil
		select {
		case p.resumeCh <- struct{}{}:
		default:
		}
		return nil
	}

	p.paused = true
	now := time.Now().UTC()
	p.state.PausedAt = &now
	select {
	case p.pauseCh <- struct{}{}:
	default:
	}
	return nil
}

func (p *Player) State() PlaybackState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *Player) ensureWorker() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return
	}

	p.wakeCh = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.running = true

	go p.workerLoop(ctx)
}

func (p *Player) workerLoop(ctx context.Context) {
	defer func() {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		default:
		}

		item, err := p.nextQueueItem(ctx)
		if err != nil {
			if errors.Is(err, ErrQueueEmpty) {
				select {
				case <-ctx.Done():
					return
				case <-p.stopCh:
					return
				case <-p.wakeCh:
					continue
				}
			}
			log.Printf("music worker error: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-p.stopCh:
				return
			case <-p.wakeCh:
				continue
			case <-time.After(1 * time.Second):
				continue
			}
		}

		if item == nil {
			continue
		}

		settings := QueueSettings{
			RepeatMode: RepeatModeNone,
		}
		if p.service != nil {
			if s, err := p.service.GetSettings(ctx, p.guildID); err == nil {
				settings = s
			}
		}
		if settings.RepeatMode == RepeatModeTrack {
			for {
				if err := p.playItem(ctx, *item); err != nil {
					if errors.Is(err, ErrPlaybackSkipped) {
						break
					}
					if errors.Is(err, ErrPlaybackStopped) {
						return
					}
					log.Printf("music playback error: %v", err)
					break
				}

				if p.service == nil {
					break
				}
				nextSettings, err := p.service.GetSettings(ctx, p.guildID)
				if err != nil || nextSettings.RepeatMode != RepeatModeTrack {
					break
				}
			}
			continue
		}

		if err := p.playItem(ctx, *item); err != nil {
			if errors.Is(err, ErrPlaybackSkipped) {
				continue
			}
			if errors.Is(err, ErrPlaybackStopped) {
				return
			}
			log.Printf("music playback error: %v", err)
		}

		if p.service != nil {
			latestSettings, err := p.service.GetSettings(ctx, p.guildID)
			if err == nil {
				settings = latestSettings
			}
		}

		if settings.RepeatMode == RepeatModeTrack {
			for {
				if err := p.playItem(ctx, *item); err != nil {
					if errors.Is(err, ErrPlaybackSkipped) {
						break
					}
					if errors.Is(err, ErrPlaybackStopped) {
						return
					}
					log.Printf("music playback error: %v", err)
					break
				}

				if p.service == nil {
					break
				}
				nextSettings, err := p.service.GetSettings(ctx, p.guildID)
				if err != nil || nextSettings.RepeatMode != RepeatModeTrack {
					break
				}
			}
		}

		if p.service != nil {
			latestSettings, err := p.service.GetSettings(ctx, p.guildID)
			if err == nil {
				settings = latestSettings
			}
		}

		if settings.RepeatMode == RepeatModeQueue && p.service != nil {
			_, _ = p.service.ResolveAndEnqueue(ctx, p.guildID, item.Track.URL, item.Track.Source, item.Track.RequestedBy, item.Priority)
		}
	}
}

func (p *Player) nextQueueItem(ctx context.Context) (*QueueItem, error) {
	if p.service == nil {
		return nil, ErrQueueStoreNil
	}

	settings, _ := p.service.GetSettings(ctx, p.guildID)
	if settings.Shuffle {
		if store := p.service.queue; store != nil {
			return store.DequeueRandom(ctx, p.guildID)
		}
	}

	return p.service.Dequeue(ctx, p.guildID)
}

func (p *Player) playItem(ctx context.Context, item QueueItem) error {
	p.mu.Lock()
	if p.vc == nil {
		p.mu.Unlock()
		return ErrVoiceNotConnected
	}
	p.state = PlaybackState{
		Track:     &item.Track,
		StartedAt: time.Now().UTC(),
		Position:  0,
		Volume:    p.volume,
		IsPlaying: true,
	}
	p.mu.Unlock()

	streamURL, err := p.resolver.ResolveStreamURL(ctx, item.Track.URL, item.Track.Source)
	if err != nil {
		return err
	}

	p.frameCount = 0
	p.paused = false
	p.state.PausedAt = nil

	playCtx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	p.playCtx = playCtx
	p.playCancel = cancel
	p.mu.Unlock()
	defer cancel()

	for {
		err := p.streamAudio(playCtx, streamURL)
		if errors.Is(err, ErrPlaybackRestarted) {
			continue
		}
		return err
	}
}

func (p *Player) streamAudio(ctx context.Context, url string) error {
	p.mu.Lock()
	vc := p.vc
	p.mu.Unlock()
	if vc == nil {
		return ErrVoiceNotConnected
	}

	ffmpegCtx, ffmpegCancel := context.WithCancel(ctx)
	defer ffmpegCancel()

	volume := 1.0
	args := []string{
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "5",
		"-i", url,
		"-af", fmt.Sprintf("volume=%.2f", volume),
		"-c:a", "libopus",
		"-ar", "48000",
		"-ac", "2",
		"-b:a", "96k",
		"-vbr", "on",
		"-frame_duration", "20",
		"-application", "audio",
		"-f", "ogg",
		"-loglevel", "warning",
		"pipe:1",
	}

	cmd := exec.CommandContext(ffmpegCtx, "ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create ffmpeg stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create ffmpeg stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	go func() {
		reader := bufio.NewReader(stderr)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if line != "" {
			}
		}
	}()

	p.mu.Lock()
	p.ffmpegCmd = cmd
	p.ffmpegStdout = stdout
	p.ffmpegCancel = ffmpegCancel
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		if p.ffmpegCmd != nil && p.ffmpegCmd.Process != nil {
			_ = p.ffmpegCmd.Process.Kill()
			_ = p.ffmpegCmd.Wait()
		}
		p.ffmpegCmd = nil
		p.ffmpegStdout = nil
		p.ffmpegCancel = nil
		p.mu.Unlock()
	}()

	safeSpeaking(vc, true)
	defer safeSpeaking(vc, false)

	return p.streamOpusFromOgg(ctx, stdout, vc)
}

func (p *Player) streamOpusFromOgg(ctx context.Context, r io.Reader, vc *discordgo.VoiceConnection) error {
	reader := bufio.NewReaderSize(r, 65536)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	framesSent := 0

	for {
		select {
		case <-ctx.Done():
			log.Printf("Context done, stopping stream after %d frames", framesSent)
			return nil
		case <-p.stopCh:
			log.Printf("Stop signal received, stopping stream after %d frames", framesSent)
			return nil
		case <-p.skipCh:
			log.Printf("Skip signal received, stopping stream after %d frames", framesSent)
			return nil
		case <-p.restartCh:
			log.Printf("Restart signal received, restarting stream after %d frames", framesSent)
			return ErrPlaybackRestarted
		default:
		}

		p.mu.Lock()
		isPaused := p.paused
		p.mu.Unlock()

		if isPaused {
			safeSpeaking(vc, false)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		header, err := p.readOggPageHeader(reader)
		if err != nil {
			if err == io.EOF {
				log.Printf("Audio stream ended (EOF) after %d frames", framesSent)
				return nil
			}
			log.Printf("Error reading OGG header: %v (after %d frames)", err, framesSent)
			return err
		}

		if header.isHeader {
			continue
		}

		safeSpeaking(vc, true)

		for _, packet := range header.packets {
			if len(packet) == 0 {
				continue
			}

			select {
			case <-ctx.Done():
				return nil
			case <-p.stopCh:
				return nil
			case <-p.skipCh:
				return nil
			case <-p.restartCh:
				return ErrPlaybackRestarted
			default:
			}

			p.mu.Lock()
			isPaused = p.paused
			p.mu.Unlock()

			if isPaused {
				safeSpeaking(vc, false)
				for isPaused {
					time.Sleep(50 * time.Millisecond)
					select {
					case <-ctx.Done():
						return nil
					case <-p.stopCh:
						return nil
					case <-p.skipCh:
						return nil
					default:
					}
					p.mu.Lock()
					isPaused = p.paused
					p.mu.Unlock()
				}
				safeSpeaking(vc, true)
			}

			<-ticker.C

			select {
			case vc.OpusSend <- packet:
				p.frameCount++
				framesSent++
				position := time.Duration(p.frameCount) * 20 * time.Millisecond
				p.state.Position = position
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
				log.Printf("Timeout sending opus frame %d", framesSent)
			}
		}
	}
}

type oggPage struct {
	isHeader bool
	packets  [][]byte
}

func (p *Player) readOggPageHeader(reader *bufio.Reader) (*oggPage, error) {
	if err := p.syncToOggPage(reader); err != nil {
		return nil, err
	}

	headerRest := make([]byte, 23)
	if _, err := io.ReadFull(reader, headerRest); err != nil {
		return nil, err
	}

	headerType := headerRest[1]
	pageSegments := headerRest[22]

	segmentTable := make([]byte, pageSegments)
	if _, err := io.ReadFull(reader, segmentTable); err != nil {
		return nil, err
	}

	pageSize := 0
	for _, seg := range segmentTable {
		pageSize += int(seg)
	}

	pageData := make([]byte, pageSize)
	if _, err := io.ReadFull(reader, pageData); err != nil {
		return nil, err
	}

	isHeader := headerType&0x02 != 0
	if len(pageData) >= 8 {
		magic := string(pageData[:8])
		if magic == "OpusHead" || magic == "OpusTags" {
			isHeader = true
		}
	}

	packets := p.extractPacketsFromPage(segmentTable, pageData)
	return &oggPage{
		isHeader: isHeader,
		packets:  packets,
	}, nil
}

func (p *Player) syncToOggPage(reader *bufio.Reader) error {
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return err
		}

		if b != 'O' {
			continue
		}

		peek, err := reader.Peek(3)
		if err != nil {
			return err
		}

		if string(peek) == "ggS" {
			reader.Discard(3)
			return nil
		}
	}
}

func (p *Player) extractPacketsFromPage(segmentTable []byte, pageData []byte) [][]byte {
	var packets [][]byte
	var currentPacket []byte
	offset := 0

	for _, segSize := range segmentTable {
		size := int(segSize)
		if offset+size > len(pageData) {
			break
		}

		currentPacket = append(currentPacket, pageData[offset:offset+size]...)
		offset += size

		if segSize < 255 {
			if len(currentPacket) > 0 {
				packet := make([]byte, len(currentPacket))
				copy(packet, currentPacket)
				packets = append(packets, packet)
				currentPacket = currentPacket[:0]
			}
		}
	}

	if len(currentPacket) > 0 {
		packet := make([]byte, len(currentPacket))
		copy(packet, currentPacket)
		packets = append(packets, packet)
	}

	return packets
}

func (p *Player) ensureVoiceConnection(userID string) error {
	p.mu.Lock()
	if p.vc != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	if p.session == nil {
		return fmt.Errorf("discord session is nil")
	}

	channelID, err := findUserVoiceChannel(p.session, p.guildID, userID)
	if err != nil {
		return err
	}

	vc, err := p.session.ChannelVoiceJoin(p.guildID, channelID, false, true)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.vc = vc
	p.mu.Unlock()
	return nil
}

func (p *Player) cleanupVoiceLocked() {
	if p.ffmpegCancel != nil {
		p.ffmpegCancel()
		p.ffmpegCancel = nil
	}
	if p.ffmpegCmd != nil && p.ffmpegCmd.Process != nil {
		_ = p.ffmpegCmd.Process.Kill()
		_ = p.ffmpegCmd.Wait()
		p.ffmpegCmd = nil
	}
	if p.ffmpegStdout != nil {
		_ = p.ffmpegStdout.Close()
		p.ffmpegStdout = nil
	}
	if p.vc != nil {
		_ = p.vc.Disconnect()
		p.vc = nil
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if p.playCancel != nil {
		p.playCancel()
		p.playCancel = nil
	}
}

func (p *Player) signalWake() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.wakeCh == nil {
		return
	}
	select {
	case p.wakeCh <- struct{}{}:
	default:
	}
}

func findUserVoiceChannel(s *discordgo.Session, guildID string, userID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("discord session is nil")
	}

	var guild *discordgo.Guild
	if s.State != nil {
		if g, err := s.State.Guild(guildID); err == nil {
			guild = g
		}
	}
	if guild == nil {
		g, err := s.Guild(guildID)
		if err != nil {
			return "", err
		}
		guild = g
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID && vs.ChannelID != "" {
			return vs.ChannelID, nil
		}
	}

	return "", ErrNoVoiceChannel
}
