package music

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrSpotifyResolveFailed = errors.New("failed to resolve spotify track")

type SpotifyClient struct {
	ClientID     string
	ClientSecret string
	HTTPClient   *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func NewSpotifyClient(clientID, clientSecret string) *SpotifyClient {
	return &SpotifyClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *SpotifyClient) ResolveTrack(ctx context.Context, input string) (Track, error) {
	trackID := extractSpotifyTrackID(input)
	if trackID == "" {
		return Track{}, fmt.Errorf("%w: unsupported spotify input", ErrSpotifyResolveFailed)
	}

	token, err := c.getAccessToken(ctx)
	if err != nil {
		return Track{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.spotify.com/v1/tracks/"+trackID, nil)
	if err != nil {
		return Track{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Track{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Track{}, fmt.Errorf("%w: spotify api status %d", ErrSpotifyResolveFailed, resp.StatusCode)
	}

	var payload spotifyTrackResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Track{}, err
	}

	title := strings.TrimSpace(payload.Name)
	if title == "" {
		title = "Unknown Title"
	}
	if artist := payload.artistNames(); artist != "" {
		title = fmt.Sprintf("%s — %s", title, artist)
	}

	thumb := payload.albumImageURL()
	duration := time.Duration(payload.DurationMS) * time.Millisecond

	return Track{
		ID:        payload.ID,
		Title:     title,
		URL:       "https://open.spotify.com/track/" + payload.ID,
		Source:    TrackSourceSpotify,
		Duration:  duration,
		Thumbnail: thumb,
	}, nil
}

func (c *SpotifyClient) SearchTracks(ctx context.Context, query string, limit int) ([]Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: empty query", ErrSpotifyResolveFailed)
	}

	if limit <= 0 {
		limit = 1
	}
	if limit > 10 {
		limit = 10
	}

	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := "https://api.spotify.com/v1/search"
	params := url.Values{}
	params.Set("type", "track")
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("q", query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: spotify api status %d", ErrSpotifyResolveFailed, resp.StatusCode)
	}

	var payload spotifySearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	if len(payload.Tracks.Items) == 0 {
		return nil, fmt.Errorf("%w: no search results", ErrSpotifyResolveFailed)
	}

	tracks := make([]Track, 0, len(payload.Tracks.Items))
	for _, item := range payload.Tracks.Items {
		title := strings.TrimSpace(item.Name)
		if title == "" {
			title = "Unknown Title"
		}
		if artist := item.artistNames(); artist != "" {
			title = fmt.Sprintf("%s — %s", title, artist)
		}

		thumb := item.albumImageURL()
		duration := time.Duration(item.DurationMS) * time.Millisecond

		tracks = append(tracks, Track{
			ID:        item.ID,
			Title:     title,
			URL:       "https://open.spotify.com/track/" + item.ID,
			Source:    TrackSourceSpotify,
			Duration:  duration,
			Thumbnail: thumb,
		})
	}

	return tracks, nil
}

func (c *SpotifyClient) SearchTrack(ctx context.Context, query string) (Track, error) {
	results, err := c.SearchTracks(ctx, query, 1)
	if err != nil {
		return Track{}, err
	}
	if len(results) == 0 {
		return Track{}, fmt.Errorf("%w: no search results", ErrSpotifyResolveFailed)
	}
	return results[0], nil
}

func (c *SpotifyClient) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.expiresAt) {
		return c.accessToken, nil
	}

	if c.ClientID == "" || c.ClientSecret == "" {
		return "", fmt.Errorf("%w: missing spotify client credentials", ErrSpotifyResolveFailed)
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://accounts.spotify.com/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+basicAuth(c.ClientID, c.ClientSecret))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: token status %d", ErrSpotifyResolveFailed, resp.StatusCode)
	}

	var payload spotifyTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	if payload.AccessToken == "" {
		return "", fmt.Errorf("%w: empty access token", ErrSpotifyResolveFailed)
	}

	c.accessToken = payload.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(payload.ExpiresIn-30) * time.Second)

	return c.accessToken, nil
}

func extractSpotifyTrackID(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	if trackID, ok := strings.CutPrefix(input, "spotify:track:"); ok {
		return trackID
	}

	u, err := url.Parse(input)
	if err != nil {
		return ""
	}
	if !strings.Contains(strings.ToLower(u.Host), "spotify.com") {
		return ""
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := range len(parts) {
		if parts[i] == "track" && i+1 < len(parts) {
			return parts[i+1]
		}
	}

	return ""
}

func basicAuth(clientID, clientSecret string) string {
	raw := clientID + ":" + clientSecret
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

type spotifyTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type spotifySearchResponse struct {
	Tracks struct {
		Items []spotifyTrackResponse `json:"items"`
	} `json:"tracks"`
}

type spotifyTrackResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
	Artists    []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	} `json:"album"`
}

func (t spotifyTrackResponse) artistNames() string {
	if len(t.Artists) == 0 {
		return ""
	}
	names := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return strings.Join(names, ", ")
}

func (t spotifyTrackResponse) albumImageURL() string {
	if len(t.Album.Images) == 0 {
		return ""
	}
	return t.Album.Images[0].URL
}
