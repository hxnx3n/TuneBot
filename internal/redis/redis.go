package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	redislib "github.com/redis/go-redis/v9"
)

var (
	client *redislib.Client
	once   sync.Once
)

type Config struct {
	Host     string
	Port     int
	Password string
	DB       int
}

func Init(cfg Config) (*redislib.Client, error) {
	var initErr error

	once.Do(func() {
		addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		client = redislib.NewClient(&redislib.Options{
			Addr:     addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		})

		attempts := 5
		backoff := 200 * time.Millisecond

		for attempt := 1; attempt <= attempts; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err := client.Ping(ctx).Err()
			cancel()

			if err == nil {
				initErr = nil
				return
			}

			initErr = err
			if attempt < attempts {
				time.Sleep(backoff)
				backoff *= 2
			}
		}

		_ = client.Close()
		client = nil
	})

	if client == nil && initErr == nil {
		return nil, fmt.Errorf("redis client not initialized")
	}

	return client, initErr
}

func Client() *redislib.Client {
	return client
}

func Close() error {
	if client == nil {
		return nil
	}
	return client.Close()
}
