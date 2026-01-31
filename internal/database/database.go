package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	db   *sql.DB
	once sync.Once
)

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func (cfg *Config) ConnectionString() string {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.DBName, cfg.SSLMode,
	)

	if cfg.Password != "" {
		connStr += fmt.Sprintf(" password=%s", cfg.Password)
	}
	return connStr
}

func Initalize(cfg *Config) error {
	var initError error

	once.Do(func() {
		var err error
		db, err = sql.Open("postgres", cfg.ConnectionString())
		if err != nil {
			initError = fmt.Errorf("failed to open database: %w", err)
			return
		}

		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			initError = fmt.Errorf("failed to ping database: %w", err)
			return
		}

		if err := runMigrations(); err != nil {
			initError = fmt.Errorf("failed to run migrations: %w", err)
			return
		}

		log.Printf("Database connection established")
	})

	return initError
}

func runMigrations() error {
	migrations := []string{
		`
		CREATE TABLE IF NOT EXISTS dashboard_entries (
			guild_id TEXT PRIMARY KEY,
			channel_id TEXT NOT NULL,
			message_id TEXT NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("failed to execute migration: %w\nQuery: %s", err, m)
		}
	}
	log.Printf("Database migrations completed (no migrations registered)")
	return nil
}

func GetDB() *sql.DB {
	return db
}

func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}
