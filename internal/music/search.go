package music

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const defaultSearchLimit = 4
const maxSearchLimit = 10
const searchCacheTTL = 5 * time.Minute

type searchCacheEntry struct {
	results   []Track
	expiresAt time.Time
}

var searchCache = struct {
	mu   sync.RWMutex
	data map[string]searchCacheEntry
}{
	data: make(map[string]searchCacheEntry),
}

func SearchTracks(
	ctx context.Context,
	query string,
	sourceHint TrackSource,
	limit int,
	resolver *YTDLPResolver,
	spotify *SpotifyClient,
) ([]Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrMissingInput
	}

	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	key := cacheKey(query, sourceHint)
	if cached, ok := getCachedResults(key); ok {
		return cached, nil
	}

	if isSpotifyInput(query, sourceHint) {
		if spotify == nil {
			return nil, ErrSpotifyClientNil
		}
		lower := strings.ToLower(query)
		if strings.Contains(lower, "spotify.com") || strings.HasPrefix(lower, "spotify:track:") {
			track, err := spotify.ResolveTrack(ctx, query)
			if err != nil {
				return nil, err
			}
			results := []Track{track}
			setCachedResults(key, results)
			return results, nil
		}
		results, err := spotify.SearchTracks(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		setCachedResults(key, results)
		return results, nil
	}

	if resolver == nil {
		return nil, ErrResolverNil
	}

	results, err := resolver.ResolveSearch(ctx, query, sourceHint, limit)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("%w: no search results", ErrResolveFailed)
	}

	setCachedResults(key, results)
	return results, nil
}

func cacheKey(query string, sourceHint TrackSource) string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	return string(sourceHint) + ":" + normalized
}

func getCachedResults(key string) ([]Track, bool) {
	searchCache.mu.RLock()
	entry, ok := searchCache.data[key]
	searchCache.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		searchCache.mu.Lock()
		delete(searchCache.data, key)
		searchCache.mu.Unlock()
		return nil, false
	}
	return entry.results, true
}

func setCachedResults(key string, results []Track) {
	searchCache.mu.Lock()
	searchCache.data[key] = searchCacheEntry{
		results:   results,
		expiresAt: time.Now().Add(searchCacheTTL),
	}
	searchCache.mu.Unlock()
}
