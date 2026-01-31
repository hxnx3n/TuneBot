package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hxnx/tunebot/config"
	"github.com/hxnx/tunebot/internal/bot"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("TuneBot - Discord Music Bot")
	log.Println("============================")

	cfg, err := config.Load()
	if err != nil {
		log.Printf("Error: Failed to load configuration: %v", err)
		log.Println("")
		log.Println("Please ensure you have set the following environment variables:")
		log.Println("  DISCORD_TOKEN          - Your Discord bot token (required)")
		log.Println("  DISCORD_APPLICATION_ID - Your Discord application ID (required)")
		log.Println("")
		log.Println("Optional environment variables:")
		log.Println("  DISCORD_GUILD_ID       - Guild ID for development (registers commands to specific guild)")
		log.Println("  SHARD_COUNT            - Number of shards (0 = auto-detect)")
		log.Println("  LOG_LEVEL              - Log level (debug, info, warn, error)")
		log.Println("  DEFAULT_VOLUME         - Default volume level (0-200, default: 100)")
		log.Println("  MAX_QUEUE_SIZE         - Maximum queue size per guild (default: 500)")
		log.Println("  AUTO_LEAVE_TIMEOUT     - Auto-leave timeout in seconds (0 = disabled, default: 300)")
		log.Println("")
		log.Println("Database configuration:")
		log.Println("  DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME, DB_SSLMODE")
		log.Println("")
		log.Println("Redis configuration:")
		log.Println("  REDIS_HOST, REDIS_PORT, REDIS_PASSWORD, REDIS_DB")
		log.Println("")
		log.Println("Spotify configuration:")
		log.Println("  SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET")
		os.Exit(1)
	}

	log.Println("")
	log.Println("Configuration loaded successfully")
	log.Println("---------------------------------")

	if cfg.IsDevelopment() {
		log.Printf("Mode: Development (Guild ID: %s)", cfg.GuildID)
	} else {
		log.Printf("Mode: Production (global commands)")
	}
	log.Printf("Log Level: %s", cfg.LogLevel)

	log.Println("")
	log.Println("Bot Settings:")
	log.Printf("  Default Volume: %d%%", cfg.DefaultVolume)
	log.Printf("  Max Queue Size: %d", cfg.MaxQueueSize)
	if cfg.AutoLeaveTimeout > 0 {
		log.Printf("  Auto Leave Timeout: %d seconds", cfg.AutoLeaveTimeout)
	} else {
		log.Printf("  Auto Leave Timeout: disabled")
	}

	if cfg.ShardCount > 0 {
		log.Printf("  Shard Count: %d (manual)", cfg.ShardCount)
	} else {
		log.Printf("  Shard Count: auto-detect")
	}

	log.Println("")
	log.Println("Database:")
	log.Printf("  Host: %s:%d", cfg.DBHost, cfg.DBPort)
	log.Printf("  Database: %s", cfg.DBName)
	log.Printf("  User: %s", cfg.DBUser)
	log.Printf("  SSL Mode: %s", cfg.DBSSLMode)

	log.Println("")
	log.Println("Redis:")
	log.Printf("  Host: %s:%d", cfg.RedisHost, cfg.RedisPort)
	log.Printf("  Database: %d", cfg.RedisDB)

	log.Println("")
	log.Println("Spotify:")
	if cfg.SpotifyClientID != "" && cfg.SpotifyClientSecret != "" {
		log.Printf("  Status: configured")
	} else {
		log.Printf("  Status: not configured (Spotify links will not work)")
	}

	log.Println("")
	log.Println("---------------------------------")

	b, err := bot.New(cfg)
	if err != nil {
		log.Fatalf("Error: Failed to create bot: %v", err)
	}

	log.Println("Starting bot...")
	if err := b.Start(); err != nil {
		log.Fatalf("Error: Bot error: %v", err)
	}

	log.Println("Bot is running. Press CTRL+C to exit.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	if err := b.Stop(); err != nil {
		log.Printf("Error: Failed to stop bot: %v", err)
	}
}
