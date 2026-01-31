package database

import (
	"context"
	"database/sql"
	"time"
)

const guildRepoTimeout = 2 * time.Second

type GuildRepository struct {
	db *sql.DB
}

func NewGuildRepository() *GuildRepository {
	return &GuildRepository{db: GetDB()}
}

func (r *GuildRepository) UpsertDashboardEntry(guildID, channelID, messageID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	if guildID == "" || channelID == "" || messageID == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), guildRepoTimeout)
	defer cancel()

	const query = `
		INSERT INTO dashboard_entries (guild_id, channel_id, message_id, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (guild_id)
		DO UPDATE SET
			channel_id = EXCLUDED.channel_id,
			message_id = EXCLUDED.message_id,
			updated_at = NOW();
	`

	_, err := r.db.ExecContext(ctx, query, guildID, channelID, messageID)
	return err
}

func (r *GuildRepository) GetDashboardEntry(guildID string) (string, string, bool, error) {
	if r == nil || r.db == nil {
		return "", "", false, nil
	}
	if guildID == "" {
		return "", "", false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), guildRepoTimeout)
	defer cancel()

	const query = `
		SELECT channel_id, message_id
		FROM dashboard_entries
		WHERE guild_id = $1
	`

	var channelID, messageID string
	err := r.db.QueryRowContext(ctx, query, guildID).Scan(&channelID, &messageID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", false, nil
		}
		return "", "", false, err
	}

	return channelID, messageID, true, nil
}

func (r *GuildRepository) DeleteDashboardEntry(guildID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	if guildID == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), guildRepoTimeout)
	defer cancel()

	const query = `
		DELETE FROM dashboard_entries
		WHERE guild_id = $1
	`

	_, err := r.db.ExecContext(ctx, query, guildID)
	return err
}
