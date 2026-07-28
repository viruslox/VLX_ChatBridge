package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"VLX_ChatBridge/internal/core/config"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// DB is a wrapper around the sql.DB connection pool.
type DB struct {
	sql    *sql.DB
	logger *zap.Logger
}

// ExportedForTesting creates a new DB instance from an existing sql.DB and zap.Logger.
// This should only be used for testing.
func ExportedForTesting(db *sql.DB, logger *zap.Logger) *DB {
	return &DB{
		sql:    db,
		logger: logger,
	}
}

// TwitchCredentials maps to the 'twitch_credentials' table
type TwitchCredentials struct {
	UserID       string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// TwitchSubscription maps to the 'twitch_subscriptions' table
type TwitchSubscription struct {
	ID        string
	UserID    string
	EventType string
	Status    string
	CreatedAt time.Time
}

// YouTubeState maps to the 'youtube_state' table
type YouTubeState struct {
	ChannelID     string
	LiveChatID    sql.NullString
	NextPageToken sql.NullString
	UpdatedAt     time.Time
}

// dbDriverName allows testing by overriding the sql driver.
var dbDriverName = "sqlite3"

// NewConnection creates, configures, and tests a new connection.
func NewConnection(cfg config.DatabaseConfig, logger *zap.Logger) (*DB, error) {
	dsn := cfg.Path
	if dsn == "" {
		dsn = "file::memory:?cache=shared"
	}

	sqlDB, err := sql.Open(dbDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open DB connection: %w", err)
	}

	if err = sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping DB: %w", err)
	}

	if _, err := sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS announce_log (
			platform TEXT NOT NULL,
			stream_id TEXT NOT NULL,
			announced_at DATETIME NOT NULL,
			PRIMARY KEY (platform, stream_id)
		);`); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ensure announce_log table: %w", err)
	}

	logger.Info("Database connection established")
	return &DB{sql: sqlDB, logger: logger}, nil
}

// Close gracefully closes the database connection pool.
func (db *DB) Close() {
	if err := db.sql.Close(); err != nil {
		db.logger.Error("Error closing DB", zap.Error(err))
	}
}

func (db *DB) GetTwitchCredentials(userID string) (*TwitchCredentials, error) {
	creds := &TwitchCredentials{UserID: userID}
	query := `SELECT access_token, refresh_token, expires_at FROM twitch_credentials WHERE user_id = ?`
	err := db.sql.QueryRow(query, userID).Scan(&creds.AccessToken, &creds.RefreshToken, &creds.ExpiresAt)
	return creds, err
}

func (db *DB) UpsertTwitchCredentials(creds *TwitchCredentials) error {
	query := `
		INSERT INTO twitch_credentials (user_id, access_token, refresh_token, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			expires_at = excluded.expires_at
	`
	_, err := db.sql.Exec(query, creds.UserID, creds.AccessToken, creds.RefreshToken, creds.ExpiresAt)
	return err
}

func (db *DB) CreateSubscription(sub *TwitchSubscription) error {
	query := `
		INSERT INTO twitch_subscriptions (id, user_id, event_type, status, created_at)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err := db.sql.Exec(query, sub.ID, sub.UserID, sub.EventType, sub.Status, sub.CreatedAt)
	return err
}

func (db *DB) DeleteSubscription(subscriptionID string) error {
	query := `DELETE FROM twitch_subscriptions WHERE id = ?`
	_, err := db.sql.Exec(query, subscriptionID)
	return err
}

func (db *DB) GetYouTubeState(channelID string) (*YouTubeState, error) {
	state := &YouTubeState{ChannelID: channelID}
	query := `SELECT live_chat_id, next_page_token, updated_at FROM youtube_state WHERE channel_id = ?`
	err := db.sql.QueryRow(query, channelID).Scan(&state.LiveChatID, &state.NextPageToken, &state.UpdatedAt)
	return state, err
}

func (db *DB) UpsertYouTubeState(state *YouTubeState) error {
	query := `
		INSERT INTO youtube_state (channel_id, live_chat_id, next_page_token, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (channel_id) DO UPDATE SET
			live_chat_id = excluded.live_chat_id,
			next_page_token = excluded.next_page_token,
			updated_at = excluded.updated_at
	`
	_, err := db.sql.Exec(query, state.ChannelID, state.LiveChatID, state.NextPageToken, state.UpdatedAt)
	return err
}

func (db *DB) GetEnabledSubscriptionsByUsers(userIDs []string) (map[string]map[string]bool, error) {
	result := make(map[string]map[string]bool)
	if len(userIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(userIDs))
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT user_id, event_type FROM twitch_subscriptions WHERE user_id IN (%s) AND status = 'enabled'`, strings.Join(placeholders, ","))

	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var userID, eventType string
		if err := rows.Scan(&userID, &eventType); err != nil {
			return nil, err
		}
		if _, exists := result[userID]; !exists {
			result[userID] = make(map[string]bool)
		}
		result[userID][eventType] = true
	}
	return result, rows.Err()
}

// AnnounceLog maps to the 'announce_log' table (cross-restart de-dup).
type AnnounceLog struct {
	Platform    string
	StreamID    string
	AnnouncedAt time.Time
}

// AlreadyAnnounced reports whether (platform, streamID) was previously recorded.
// Satisfies announcer.Store.
func (db *DB) AlreadyAnnounced(platform, streamID string) (bool, error) {
	var one int
	query := `SELECT 1 FROM announce_log WHERE platform = ? AND stream_id = ? LIMIT 1`
	err := db.sql.QueryRow(query, platform, streamID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MarkAnnounced records (platform, streamID) as announced now. Idempotent.
// Satisfies announcer.Store.
func (db *DB) MarkAnnounced(platform, streamID string) error {
	query := `
		INSERT INTO announce_log (platform, stream_id, announced_at)
		VALUES (?, ?, ?)
		ON CONFLICT (platform, stream_id) DO UPDATE SET announced_at = excluded.announced_at
	`
	_, err := db.sql.Exec(query, platform, streamID, time.Now())
	return err
}

// PruneAnnounceLog deletes announce records older than maxAge (TTL cleanup).
// A maxAge <= 0 is a no-op.
func (db *DB) PruneAnnounceLog(maxAge time.Duration) error {
	if maxAge <= 0 {
		return nil
	}
	cutoff := time.Now().Add(-maxAge)
	_, err := db.sql.Exec(`DELETE FROM announce_log WHERE announced_at < ?`, cutoff)
	return err
}

// SetDBDriverNameForTest allows tests to override the sql driver name.
func SetDBDriverNameForTest(driverName string) {
	dbDriverName = driverName
}
