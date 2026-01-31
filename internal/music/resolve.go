package music

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

var ErrResolveFailed = errors.New("failed to resolve track metadata")

type YTDLPResolver struct {
	Binary string
}

func NewYTDLPResolver() *YTDLPResolver {
	return &YTDLPResolver{
		Binary: "yt-dlp",
	}
}

func (r *YTDLPResolver) Resolve(ctx context.Context, input string, sourceHint TrackSource) (Track, error) {
	if strings.TrimSpace(input) == "" {
		return Track{}, fmt.Errorf("%w: empty input", ErrResolveFailed)
	}

	target := strings.TrimSpace(input)
	if !looksLikeURL(target) {
		switch sourceHint {
		case TrackSourceSoundCloud:
			target = "scsearch1:" + target
		default:
			target = "ytsearch1:" + target
		}
	}

	args := []string{
		"--no-warnings",
		"--dump-single-json",
		"--skip-download",
		"--no-playlist",
		"--paths",
		"/app/tmp",
		target,
	}

	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Env = append(os.Environ(), "TMPDIR=/app/tmp", "TEMP=/app/tmp", "TMP=/app/tmp")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Track{}, fmt.Errorf("%w: yt-dlp failed: %v: %s", ErrResolveFailed, err, strings.TrimSpace(string(output)))
	}

	var root ytDLPItem
	if err := json.Unmarshal(output, &root); err != nil {
		return Track{}, fmt.Errorf("%w: invalid json: %v", ErrResolveFailed, err)
	}

	item, err := pickYTDLPItem(root)
	if err != nil {
		return Track{}, err
	}

	link := item.WebpageURL
	if link == "" {
		link = item.URL
	}
	if link == "" {
		return Track{}, fmt.Errorf("%w: missing track url", ErrResolveFailed)
	}

	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = "Unknown Title"
	}

	source := sourceHint
	if source == TrackSourceUnknown || source == "" {
		source = detectSourceFromURL(link)
	}

	duration := time.Duration(item.Duration * float64(time.Second))
	if duration < 0 {
		duration = 0
	}

	return Track{
		ID:          item.ID,
		Title:       title,
		URL:         link,
		Source:      source,
		Duration:    duration,
		Thumbnail:   item.Thumbnail,
		RequestedBy: "",
	}, nil
}

func (r *YTDLPResolver) ResolveSearch(ctx context.Context, input string, sourceHint TrackSource, limit int) ([]Track, error) {
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("%w: empty input", ErrResolveFailed)
	}

	if limit <= 0 {
		limit = 6
	}
	if limit > 10 {
		limit = 10
	}

	target := strings.TrimSpace(input)
	if looksLikeURL(target) {
		track, err := r.Resolve(ctx, target, sourceHint)
		if err != nil {
			return nil, err
		}
		return []Track{track}, nil
	}

	switch sourceHint {
	case TrackSourceSoundCloud:
		target = fmt.Sprintf("scsearch%d:%s", limit, target)
	default:
		target = fmt.Sprintf("ytsearch%d:%s", limit, target)
	}

	args := []string{
		"--no-warnings",
		"--dump-single-json",
		"--skip-download",
		"--no-playlist",
		"--flat-playlist",
		"--paths",
		"/app/tmp",
		target,
	}

	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Env = append(os.Environ(), "TMPDIR=/app/tmp", "TEMP=/app/tmp", "TMP=/app/tmp")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: yt-dlp failed: %v: %s", ErrResolveFailed, err, strings.TrimSpace(string(output)))
	}

	var root ytDLPItem
	if err := json.Unmarshal(output, &root); err != nil {
		return nil, fmt.Errorf("%w: invalid json: %v", ErrResolveFailed, err)
	}

	items, err := pickYTDLPItems(root, limit)
	if err != nil {
		return nil, err
	}

	results := make([]Track, 0, len(items))
	for _, item := range items {
		link := item.WebpageURL
		if link == "" {
			link = item.URL
		}
		if link == "" {
			continue
		}

		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = "Unknown Title"
		}

		source := sourceHint
		if source == TrackSourceUnknown || source == "" {
			source = detectSourceFromURL(link)
		}

		duration := time.Duration(item.Duration * float64(time.Second))
		if duration < 0 {
			duration = 0
		}

		results = append(results, Track{
			ID:          item.ID,
			Title:       title,
			URL:         link,
			Source:      source,
			Duration:    duration,
			Thumbnail:   item.Thumbnail,
			RequestedBy: "",
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("%w: no usable entries", ErrResolveFailed)
	}

	return results, nil
}

func (r *YTDLPResolver) ResolveStreamURL(ctx context.Context, input string, sourceHint TrackSource) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("%w: empty input", ErrResolveFailed)
	}

	target := strings.TrimSpace(input)
	if !looksLikeURL(target) {
		switch sourceHint {
		case TrackSourceSoundCloud:
			target = "scsearch1:" + target
		default:
			target = "ytsearch1:" + target
		}
	}

	args := []string{
		"--no-warnings",
		"-f",
		"bestaudio",
		"-g",
		"--no-playlist",
		"--paths",
		"/app/tmp",
		target,
	}

	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Env = append(os.Environ(), "TMPDIR=/app/tmp", "TEMP=/app/tmp", "TMP=/app/tmp")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: yt-dlp failed: %v: %s", ErrResolveFailed, err, strings.TrimSpace(string(output)))
	}

	streamURL := strings.TrimSpace(string(output))
	if streamURL == "" {
		return "", fmt.Errorf("%w: empty stream url", ErrResolveFailed)
	}

	return streamURL, nil
}

type ytDLPItem struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	WebpageURL string      `json:"webpage_url"`
	URL        string      `json:"url"`
	Duration   float64     `json:"duration"`
	Thumbnail  string      `json:"thumbnail"`
	Entries    []ytDLPItem `json:"entries"`
}

func pickYTDLPItem(root ytDLPItem) (ytDLPItem, error) {
	if len(root.Entries) == 0 {
		return root, nil
	}

	for _, entry := range root.Entries {
		if entry.WebpageURL != "" || entry.URL != "" || entry.Title != "" {
			return entry, nil
		}
	}

	return ytDLPItem{}, fmt.Errorf("%w: no usable entries", ErrResolveFailed)
}

func pickYTDLPItems(root ytDLPItem, limit int) ([]ytDLPItem, error) {
	if limit <= 0 {
		limit = 1
	}

	if len(root.Entries) == 0 {
		if root.WebpageURL != "" || root.URL != "" || root.Title != "" {
			return []ytDLPItem{root}, nil
		}
		return nil, fmt.Errorf("%w: no usable entries", ErrResolveFailed)
	}

	items := make([]ytDLPItem, 0, limit)
	for _, entry := range root.Entries {
		if entry.WebpageURL == "" && entry.URL == "" && entry.Title == "" {
			continue
		}
		items = append(items, entry)
		if len(items) >= limit {
			break
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("%w: no usable entries", ErrResolveFailed)
	}

	return items, nil
}

func looksLikeURL(value string) bool {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return true
	}

	u, err := url.Parse(value)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func detectSourceFromURL(raw string) TrackSource {
	u, err := url.Parse(raw)
	if err != nil {
		return TrackSourceUnknown
	}

	host := strings.ToLower(u.Host)
	switch {
	case strings.Contains(host, "youtube.com"), strings.Contains(host, "youtu.be"):
		return TrackSourceYouTube
	case strings.Contains(host, "soundcloud.com"):
		return TrackSourceSoundCloud
	case strings.Contains(host, "spotify.com"):
		return TrackSourceSpotify
	default:
		return TrackSourceUnknown
	}
}
