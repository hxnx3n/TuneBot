package bot

import (
	"log"

	"github.com/bwmarrin/discordgo"
	"github.com/hxnx/tunebot/config"
	"github.com/hxnx/tunebot/internal/database"
	commands "github.com/hxnx/tunebot/internal/features"
	"github.com/hxnx/tunebot/internal/redis"
)

type Bot struct {
	config       *config.Config
	sessions     []*discordgo.Session
	started      bool
	presenceStop chan struct{}
}

func New(cfg *config.Config) (*Bot, error) {

	dbConfig := &database.Config{
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		DBName:   cfg.DBName,
		SSLMode:  cfg.DBSSLMode,
	}

	if err := database.Initalize(dbConfig); err != nil {
		log.Printf("Warning: Database initialization failed: %v", err)
	}

	redisConfig := redis.Config{
		Host:     cfg.RedisHost,
		Port:     cfg.RedisPort,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}

	if _, err := redis.Init(redisConfig); err != nil {
		log.Printf("Warning: Redis initialization failed: %v", err)
	}

	shardCount := cfg.ShardCount
	if shardCount < 1 {
		s, err := discordgo.New("Bot " + cfg.DiscordToken)
		if err != nil {
			return nil, err
		}

		if gw, err := s.GatewayBot(); err == nil && gw.Shards > 0 {
			shardCount = gw.Shards
		} else {
			log.Printf("Warning: failed to auto-detect shard count, defaulting to 1: %v", err)
			shardCount = 1
		}
	}

	if shardCount < 1 {
		shardCount = 1
	}

	sessions := make([]*discordgo.Session, 0, shardCount)
	for shard := 0; shard < shardCount; shard++ {
		s, err := discordgo.New("Bot " + cfg.DiscordToken)
		if err != nil {
			return nil, err
		}

		s.Identify.Intents = discordgo.IntentsGuilds |
			discordgo.IntentsGuildVoiceStates |
			discordgo.IntentsGuildMessages |
			discordgo.IntentsMessageContent

		if shardCount > 1 {
			s.Identify.Shard = &[2]int{shard, shardCount}
			s.ShardCount = shardCount
		}

		sessions = append(sessions, s)
	}

	return &Bot{
		config:   cfg,
		sessions: sessions,
	}, nil
}

func (b *Bot) Start() error {
	if b.started {
		return nil
	}

	if len(b.sessions) == 0 {
		return nil
	}

	for _, s := range b.sessions {
		b.registerHandlers(s)
		commands.AddHandlers(s)
	}

	if _, err := commands.RegisterCommands(b.sessions[0], b.config.ApplicationID, b.config.GuildID); err != nil {
		log.Printf("Warning: failed to register slash commands: %v", err)
	}

	for _, s := range b.sessions {
		if err := s.Open(); err != nil {
			return err
		}
	}

	b.startPresenceUpdater()
	b.started = true
	log.Printf("Bot session opened (%d shard(s))", len(b.sessions))
	return nil
}

func (b *Bot) registerHandlers(s *discordgo.Session) {
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		if s.State != nil && s.State.User != nil {
			log.Printf("Bot ready as %s#%s", s.State.User.Username, s.State.User.Discriminator)
		} else {
			log.Printf("Bot ready")
		}
		b.updatePresence()
	})
}

func (b *Bot) Stop() error {
	if !b.started {
		return nil
	}

	b.started = false
	b.stopPresenceUpdater()
	for _, s := range b.sessions {
		if err := s.Close(); err != nil {
			return err
		}
	}

	if err := database.Close(); err != nil {
		log.Printf("Warning: failed to close database: %v", err)
	}

	if err := redis.Close(); err != nil {
		log.Printf("Warning: failed to close redis: %v", err)
	}

	log.Printf("Bot session closed (%d shard(s))", len(b.sessions))
	return nil
}
