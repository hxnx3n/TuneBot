package config

import (
	"errors"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordToken  string
	ApplicationID string

	GuildID string

	ShardCount int

	LogLevel         string
	AutoLeaveTimeout int
	DefaultVolume    int
	MaxQueueSize     int

	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int

	SpotifyClientID     string
	SpotifyClientSecret string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DiscordToken:  os.Getenv("DISCORD_TOKEN"),
		ApplicationID: os.Getenv("DISCORD_APPLICATION_ID"),

		GuildID: os.Getenv("DISCORD_GUILD_ID"),

		ShardCount: getEnvAsIntWithDefault("SHARD_COUNT", 0),

		LogLevel:         getEnvWithDefault("LOG_LEVEL", "info"),
		AutoLeaveTimeout: getEnvAsIntWithDefault("AUTO_LEAVE_TIMEOUT", 300),
		DefaultVolume:    getEnvAsIntWithDefault("DEFAULT_VOLUME", 100),
		MaxQueueSize:     getEnvAsIntWithDefault("MAX_QUEUE_SIZE", 500),

		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     getEnvAsInt("DB_PORT"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		DBSSLMode:  os.Getenv("DB_SSLMODE"),

		RedisHost:     os.Getenv("REDIS_HOST"),
		RedisPort:     getEnvAsInt("REDIS_PORT"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       getEnvAsIntWithDefault("REDIS_DB", 0),

		SpotifyClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		SpotifyClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.DiscordToken == "" {
		return errors.New("DISCORD_TOKEN is required")
	}

	if c.ApplicationID == "" {
		return errors.New("DISCORD_APPLICATION_ID is required")
	}

	if c.DefaultVolume < 0 || c.DefaultVolume > 200 {
		return errors.New("DEFAULT_VOLUME must be between 0 and 200")
	}

	if c.MaxQueueSize < 1 {
		return errors.New("MAX_QUEUE_SIZE must be at least 1")
	}

	return nil
}

func (c *Config) IsDevelopment() bool {
	return c.GuildID != ""
}

func getEnvAsInt(key string) int {
	return getEnvAsIntWithDefault(key, 0)
}

func getEnvAsIntWithDefault(key string, defaultValue int) int {
	if value, ok := os.LookupEnv(key); ok {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvWithDefault(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

func getEnvAsBool(key string) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return false
}

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

func (c *Config) GetDBConfig() *DBConfig {
	return &DBConfig{
		Host:     c.DBHost,
		Port:     c.DBPort,
		User:     c.DBUser,
		Password: c.DBPassword,
		Name:     c.DBName,
		SSLMode:  c.DBSSLMode,
	}
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
	Enabled  bool
}

func (c *Config) GetRedisConfig() *RedisConfig {
	return &RedisConfig{
		Host:     c.RedisHost,
		Port:     c.RedisPort,
		Password: c.RedisPassword,
		DB:       c.RedisDB,
	}
}
