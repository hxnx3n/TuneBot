package music

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrMissingInput     = errors.New("input is required")
	ErrSpotifyClientNil = errors.New("spotify client is not configured")
	ErrResolverNil      = errors.New("resolver is not configured")
	ErrQueueStoreNil    = errors.New("queue store is not configured")
)

type Service struct {
	queue    *QueueStore
	resolver *YTDLPResolver
	spotify  *SpotifyClient
}

func NewService(queue *QueueStore, resolver *YTDLPResolver, spotify *SpotifyClient) *Service {
	return &Service{
		queue:    queue,
		resolver: resolver,
		spotify:  spotify,
	}
}

func NewDefaultService() *Service {
	return &Service{
		queue:    NewQueueStoreFromDefault(),
		resolver: NewYTDLPResolver(),
	}
}

func (s *Service) WithSpotify(client *SpotifyClient) *Service {
	s.spotify = client
	return s
}

func (s *Service) ResolveInput(ctx context.Context, input string, sourceHint TrackSource, requestedBy string) (Track, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Track{}, ErrMissingInput
	}
	if s.resolver == nil {
		return Track{}, ErrResolverNil
	}

	if isSpotifyInput(input, sourceHint) {
		if s.spotify == nil {
			return Track{}, ErrSpotifyClientNil
		}

		spotifyTrack, err := s.spotify.ResolveTrack(ctx, input)
		if err != nil {
			return Track{}, err
		}

		playable, err := s.resolver.Resolve(ctx, spotifyTrack.Title, TrackSourceYouTube)
		if err != nil {
			return Track{}, err
		}

		playable.Title = spotifyTrack.Title
		if spotifyTrack.Thumbnail != "" {
			playable.Thumbnail = spotifyTrack.Thumbnail
		}
		playable.RequestedBy = requestedBy
		return playable, nil
	}

	track, err := s.resolver.Resolve(ctx, input, sourceHint)
	if err != nil {
		return Track{}, err
	}

	track.RequestedBy = requestedBy
	return track, nil
}

func (s *Service) ResolveAndEnqueue(ctx context.Context, guildID string, input string, sourceHint TrackSource, requestedBy string, priority int) (QueueItem, error) {
	track, err := s.ResolveInput(ctx, input, sourceHint, requestedBy)
	if err != nil {
		return QueueItem{}, err
	}

	item := QueueItem{
		Track:      track,
		Priority:   priority,
		EnqueuedAt: time.Now().UTC(),
	}

	if s.queue == nil {
		return QueueItem{}, ErrQueueStoreNil
	}

	if err := s.queue.Enqueue(ctx, guildID, item); err != nil {
		return QueueItem{}, err
	}

	return item, nil
}

func (s *Service) Dequeue(ctx context.Context, guildID string) (*QueueItem, error) {
	if s.queue == nil {
		return nil, ErrQueueStoreNil
	}
	return s.queue.Dequeue(ctx, guildID)
}

func (s *Service) Peek(ctx context.Context, guildID string) (*QueueItem, error) {
	if s.queue == nil {
		return nil, ErrQueueStoreNil
	}
	return s.queue.Peek(ctx, guildID)
}

func (s *Service) List(ctx context.Context, guildID string, limit int64) ([]QueueItem, error) {
	if s.queue == nil {
		return nil, ErrQueueStoreNil
	}
	return s.queue.List(ctx, guildID, limit)
}

func (s *Service) Clear(ctx context.Context, guildID string) error {
	if s.queue == nil {
		return ErrQueueStoreNil
	}
	return s.queue.Clear(ctx, guildID)
}

func (s *Service) GetSettings(ctx context.Context, guildID string) (QueueSettings, error) {
	if s.queue == nil {
		return QueueSettings{}, ErrQueueStoreNil
	}
	return s.queue.GetSettings(ctx, guildID)
}

func (s *Service) SetSettings(ctx context.Context, guildID string, settings QueueSettings) error {
	if s.queue == nil {
		return ErrQueueStoreNil
	}
	return s.queue.SetSettings(ctx, guildID, settings)
}

func isSpotifyInput(input string, hint TrackSource) bool {
	if hint == TrackSourceSpotify {
		return true
	}
	return strings.Contains(strings.ToLower(input), "spotify.com") || strings.HasPrefix(strings.ToLower(input), "spotify:track:")
}

func (s *Service) ResolveSpotifySearch(ctx context.Context, query string, requestedBy string) (Track, error) {
	if s.spotify == nil {
		return Track{}, ErrSpotifyClientNil
	}
	if s.resolver == nil {
		return Track{}, ErrResolverNil
	}

	spotifyTrack, err := s.spotify.SearchTrack(ctx, query)
	if err != nil {
		return Track{}, err
	}

	playable, err := s.resolver.Resolve(ctx, spotifyTrack.Title, TrackSourceYouTube)
	if err != nil {
		return Track{}, err
	}

	playable.Title = spotifyTrack.Title
	if spotifyTrack.Thumbnail != "" {
		playable.Thumbnail = spotifyTrack.Thumbnail
	}
	playable.RequestedBy = requestedBy
	return playable, nil
}

func (s *Service) Validate() error {
	if s.queue == nil {
		return ErrQueueStoreNil
	}
	if s.resolver == nil {
		return ErrResolverNil
	}
	return nil
}

func (s *Service) DebugString() string {
	queueOK := s.queue != nil
	resolverOK := s.resolver != nil
	spotifyOK := s.spotify != nil

	return fmt.Sprintf("queue=%t resolver=%t spotify=%t", queueOK, resolverOK, spotifyOK)
}
