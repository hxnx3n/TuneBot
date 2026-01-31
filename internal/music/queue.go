package music

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	internalredis "github.com/hxnx/tunebot/internal/redis"
	redislib "github.com/redis/go-redis/v9"
)

var ErrQueueEmpty = errors.New("queue is empty")

const (
	queueKeyPrefix    = "music:queue:"
	settingsKeyPrefix = "music:settings:"
	priorityWeight    = int64(1_000_000_000_000)
)

type QueueStore struct {
	client *redislib.Client
}

func NewQueueStore(client *redislib.Client) *QueueStore {
	return &QueueStore{client: client}
}

func NewQueueStoreFromDefault() *QueueStore {
	return &QueueStore{client: internalredis.Client()}
}

func (q *QueueStore) ensureClient() error {
	if q.client != nil {
		return nil
	}

	q.client = internalredis.Client()
	if q.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	return nil
}

func (q *QueueStore) Enqueue(ctx context.Context, guildID string, item QueueItem) error {
	if err := q.ensureClient(); err != nil {
		return err
	}
	if guildID == "" {
		return fmt.Errorf("guild id is required")
	}
	if item.EnqueuedAt.IsZero() {
		item.EnqueuedAt = time.Now().UTC()
	}

	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}

	score := buildScore(item.Priority, item.EnqueuedAt)

	return q.client.ZAdd(ctx, queueKey(guildID), redislib.Z{
		Score:  score,
		Member: payload,
	}).Err()
}

func (q *QueueStore) Dequeue(ctx context.Context, guildID string) (*QueueItem, error) {
	if err := q.ensureClient(); err != nil {
		return nil, err
	}
	if guildID == "" {
		return nil, fmt.Errorf("guild id is required")
	}

	results, err := q.client.ZPopMin(ctx, queueKey(guildID), 1).Result()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrQueueEmpty
	}

	item, err := decodeQueueItem(results[0].Member)
	if err != nil {
		return nil, err
	}

	return item, nil
}

func (q *QueueStore) DequeueRandom(ctx context.Context, guildID string) (*QueueItem, error) {
	if err := q.ensureClient(); err != nil {
		return nil, err
	}
	if guildID == "" {
		return nil, fmt.Errorf("guild id is required")
	}

	script := redislib.NewScript(`
		local key = KEYS[1]
		local len = redis.call('ZCARD', key)
		if len == 0 then
			return nil
		end
		math.randomseed(tonumber(ARGV[1]))
		local idx = math.random(0, len - 1)
		local member = redis.call('ZRANGE', key, idx, idx)[1]
		if member then
			redis.call('ZREM', key, member)
		end
		return member
	`)

	seed := time.Now().UnixNano() + rand.Int63()
	result, err := script.Run(ctx, q.client, []string{queueKey(guildID)}, seed).Result()
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, ErrQueueEmpty
	}

	item, err := decodeQueueItem(result)
	if err != nil {
		return nil, err
	}

	return item, nil
}

func (q *QueueStore) Peek(ctx context.Context, guildID string) (*QueueItem, error) {
	if err := q.ensureClient(); err != nil {
		return nil, err
	}
	if guildID == "" {
		return nil, fmt.Errorf("guild id is required")
	}

	results, err := q.client.ZRangeWithScores(ctx, queueKey(guildID), 0, 0).Result()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrQueueEmpty
	}

	item, err := decodeQueueItem(results[0].Member)
	if err != nil {
		return nil, err
	}

	return item, nil
}

func (q *QueueStore) List(ctx context.Context, guildID string, limit int64) ([]QueueItem, error) {
	if err := q.ensureClient(); err != nil {
		return nil, err
	}
	if guildID == "" {
		return nil, fmt.Errorf("guild id is required")
	}

	stop := int64(-1)
	if limit > 0 {
		stop = limit - 1
	}

	results, err := q.client.ZRange(ctx, queueKey(guildID), 0, stop).Result()
	if err != nil {
		return nil, err
	}

	items := make([]QueueItem, 0, len(results))
	for _, raw := range results {
		item, err := decodeQueueItem(raw)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}

	return items, nil
}

func (q *QueueStore) QueueSize(ctx context.Context, guildID string) (int64, error) {
	if err := q.ensureClient(); err != nil {
		return 0, err
	}
	if guildID == "" {
		return 0, fmt.Errorf("guild id is required")
	}

	return q.client.ZCard(ctx, queueKey(guildID)).Result()
}

func (q *QueueStore) Clear(ctx context.Context, guildID string) error {
	if err := q.ensureClient(); err != nil {
		return err
	}
	if guildID == "" {
		return fmt.Errorf("guild id is required")
	}

	return q.client.Del(ctx, queueKey(guildID)).Err()
}

func (q *QueueStore) GetSettings(ctx context.Context, guildID string) (QueueSettings, error) {
	if err := q.ensureClient(); err != nil {
		return QueueSettings{}, err
	}
	if guildID == "" {
		return QueueSettings{}, fmt.Errorf("guild id is required")
	}

	data, err := q.client.HGetAll(ctx, settingsKey(guildID)).Result()
	if err != nil {
		return QueueSettings{}, err
	}

	settings := QueueSettings{
		RepeatMode: RepeatModeNone,
		Shuffle:    false,
		Volume:     100,
	}

	if v, ok := data["repeat_mode"]; ok && v != "" {
		settings.RepeatMode = RepeatMode(v)
	}
	if v, ok := data["shuffle"]; ok && v != "" {
		settings.Shuffle = v == "true"
	}
	if v, ok := data["volume"]; ok && v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			settings.Volume = parsed
		}
	}

	return settings, nil
}

func (q *QueueStore) SetSettings(ctx context.Context, guildID string, settings QueueSettings) error {
	if err := q.ensureClient(); err != nil {
		return err
	}
	if guildID == "" {
		return fmt.Errorf("guild id is required")
	}

	values := map[string]interface{}{
		"repeat_mode": string(settings.RepeatMode),
		"shuffle":     fmt.Sprintf("%t", settings.Shuffle),
		"volume":      fmt.Sprintf("%d", settings.Volume),
	}

	return q.client.HSet(ctx, settingsKey(guildID), values).Err()
}

func queueKey(guildID string) string {
	return queueKeyPrefix + guildID
}

func settingsKey(guildID string) string {
	return settingsKeyPrefix + guildID
}

func buildScore(priority int, enqueuedAt time.Time) float64 {
	if enqueuedAt.IsZero() {
		enqueuedAt = time.Now().UTC()
	}

	millis := enqueuedAt.UnixMilli()
	score := int64(priority)*priorityWeight + millis
	return float64(score)
}

func decodeQueueItem(raw interface{}) (*QueueItem, error) {
	var b []byte
	switch v := raw.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		return nil, fmt.Errorf("unexpected queue payload type: %T", raw)
	}

	var item QueueItem
	if err := json.Unmarshal(b, &item); err != nil {
		return nil, err
	}

	return &item, nil
}
