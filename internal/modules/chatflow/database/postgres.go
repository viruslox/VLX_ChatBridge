package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"VLX_ChatBridge/internal/core/config"

	"github.com/lib/pq"
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
var dbDriverName = "postgres"

// NewConnection creates, configures, and tests a new connection.
func NewConnection(cfg config.DatabaseConfig, logger *zap.Logger) (*DB, error) {
	u := url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		User:   url.UserPassword(cfg.User, cfg.Password),
		Path:   cfg.DBName,
	}
	q := u.Query()
	q.Set("sslmode", cfg.SSLMode)
	u.RawQuery = q.Encode()

	dsn := u.String()

	sqlDB, err := sql.Open(dbDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open DB connection: %w", err)
	}

	if err = sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping DB: %w", err)
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
	query := `SELECT access_token, refresh_token, expires_at FROM twitch_credentials WHERE user_id = $1`
	err := db.sql.QueryRow(query, userID).Scan(&creds.AccessToken, &creds.RefreshToken, &creds.ExpiresAt)
	return creds, err
}

func (db *DB) UpsertTwitchCredentials(creds *TwitchCredentials) error {
	query := `
		INSERT INTO twitch_credentials (user_id, access_token, refresh_token, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			expires_at = EXCLUDED.expires_at
	`
	_, err := db.sql.Exec(query, creds.UserID, creds.AccessToken, creds.RefreshToken, creds.ExpiresAt)
	return err
}

func (db *DB) CreateSubscription(sub *TwitchSubscription) error {
	query := `
		INSERT INTO twitch_subscriptions (id, user_id, event_type, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := db.sql.Exec(query, sub.ID, sub.UserID, sub.EventType, sub.Status, sub.CreatedAt)
	return err
}

func (db *DB) DeleteSubscription(subscriptionID string) error {
	query := `DELETE FROM twitch_subscriptions WHERE id = $1`
	_, err := db.sql.Exec(query, subscriptionID)
	return err
}

func (db *DB) GetYouTubeState(channelID string) (*YouTubeState, error) {
	state := &YouTubeState{ChannelID: channelID}
	query := `SELECT live_chat_id, next_page_token, updated_at FROM youtube_state WHERE channel_id = $1`
	err := db.sql.QueryRow(query, channelID).Scan(&state.LiveChatID, &state.NextPageToken, &state.UpdatedAt)
	return state, err
}

func (db *DB) UpsertYouTubeState(state *YouTubeState) error {
	query := `
		INSERT INTO youtube_state (channel_id, live_chat_id, next_page_token, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (channel_id) DO UPDATE SET
			live_chat_id = EXCLUDED.live_chat_id,
			next_page_token = EXCLUDED.next_page_token,
			updated_at = EXCLUDED.updated_at
	`
	_, err := db.sql.Exec(query, state.ChannelID, state.LiveChatID, state.NextPageToken, state.UpdatedAt)
	return err
}

func (db *DB) GetEnabledSubscriptionsByUsers(userIDs []string) (map[string]map[string]bool, error) {
	result := make(map[string]map[string]bool)
	if len(userIDs) == 0 {
		return result, nil
	}

	query := `SELECT user_id, event_type FROM twitch_subscriptions WHERE user_id = ANY($1) AND status = 'enabled'`

	rows, err := db.sql.Query(query, pq.Array(userIDs))
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

// SetDBDriverNameForTest allows tests to override the sql driver name.
func SetDBDriverNameForTest(driverName string) {
	dbDriverName = driverName
}
