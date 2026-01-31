package music

import "time"

type TrackSource string

const (
	TrackSourceYouTube    TrackSource = "youtube"
	TrackSourceSpotify    TrackSource = "spotify"
	TrackSourceSoundCloud TrackSource = "soundcloud"
	TrackSourceUnknown    TrackSource = "unknown"
)

type RepeatMode string

const (
	RepeatModeNone  RepeatMode = "none"
	RepeatModeTrack RepeatMode = "track"
	RepeatModeQueue RepeatMode = "queue"
)

type Track struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	URL         string        `json:"url"`
	Source      TrackSource   `json:"source"`
	Duration    time.Duration `json:"duration"`
	Thumbnail   string        `json:"thumbnail"`
	RequestedBy string        `json:"requested_by"`
}

type QueueItem struct {
	Track      Track     `json:"track"`
	Priority   int       `json:"priority"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

type QueueSettings struct {
	RepeatMode RepeatMode `json:"repeat_mode"`
	Shuffle    bool       `json:"shuffle"`
	Volume     int        `json:"volume"`
}

type PlaybackState struct {
	Track     *Track        `json:"track"`
	StartedAt time.Time     `json:"started_at"`
	PausedAt  *time.Time    `json:"paused_at,omitempty"`
	Position  time.Duration `json:"position"`
	Volume    int           `json:"volume"`
	IsPlaying bool          `json:"is_playing"`
}
