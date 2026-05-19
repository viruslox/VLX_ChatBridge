package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"testing"
	"time"

	"VLX_ChatBridge/internal/core/config"
	"github.com/lib/pq"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestGetYouTubeState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	logger, _ := zap.NewDevelopment()
	testDB := &DB{
		sql:    db,
		logger: logger,
	}

	channelID := "test_channel"
	now := time.Now()

	t.Run("successful retrieval", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"live_chat_id", "next_page_token", "updated_at"}).
			AddRow("chat_123", "token_456", now)

		mock.ExpectQuery("^SELECT live_chat_id, next_page_token, updated_at FROM youtube_state WHERE channel_id = \\$1$").
			WithArgs(channelID).
			WillReturnRows(rows)

		state, err := testDB.GetYouTubeState(channelID)

		if err != nil {
			t.Errorf("error was not expected while getting youtube state: %s", err)
		}

		if state.ChannelID != channelID {
			t.Errorf("expected ChannelID %s, got %s", channelID, state.ChannelID)
		}
		if state.LiveChatID.String != "chat_123" {
			t.Errorf("expected LiveChatID chat_123, got %s", state.LiveChatID.String)
		}
		if state.NextPageToken.String != "token_456" {
			t.Errorf("expected NextPageToken token_456, got %s", state.NextPageToken.String)
		}
		if !state.UpdatedAt.Equal(now) {
			t.Errorf("expected UpdatedAt %v, got %v", now, state.UpdatedAt)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})

	t.Run("no rows error", func(t *testing.T) {
		mock.ExpectQuery("^SELECT live_chat_id, next_page_token, updated_at FROM youtube_state WHERE channel_id = \\$1$").
			WithArgs(channelID).
			WillReturnError(sql.ErrNoRows)

		state, err := testDB.GetYouTubeState(channelID)

		if err != sql.ErrNoRows {
			t.Errorf("expected error %v, got %v", sql.ErrNoRows, err)
		}
		if state.ChannelID != channelID {
			t.Errorf("expected ChannelID %s, got %s", channelID, state.ChannelID)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})
}

func TestNewConnection(t *testing.T) {
	oldDriver := dbDriverName
	dbDriverName = "sqlmock"
	defer func() { dbDriverName = oldDriver }()

	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "user",
		Password: "password",
		DBName:   "testdb",
		SSLMode:  "disable",
	}

	// Calculate expected DSN
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

	t.Run("successful connection", func(t *testing.T) {
		dbDriverName = "sqlmock"
		db, mock, err := sqlmock.NewWithDSN(dsn, sqlmock.MonitorPingsOption(true))
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		mock.ExpectPing()

		logger, _ := zap.NewDevelopment()
		conn, err := NewConnection(cfg, logger)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if conn == nil {
			t.Fatal("expected connection, got nil")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %s", err)
		}
	})

	t.Run("ping failure", func(t *testing.T) {
		dbDriverName = "sqlmock"

		cfgPingFail := cfg
		cfgPingFail.DBName = "testdb_ping_fail"

		uPingFail := url.URL{
			Scheme: "postgres",
			Host:   fmt.Sprintf("%s:%s", cfgPingFail.Host, cfgPingFail.Port),
			User:   url.UserPassword(cfgPingFail.User, cfgPingFail.Password),
			Path:   cfgPingFail.DBName,
		}
		qPingFail := uPingFail.Query()
		qPingFail.Set("sslmode", cfgPingFail.SSLMode)
		uPingFail.RawQuery = qPingFail.Encode()

		dsnPingFail := uPingFail.String()

		db, mock, err := sqlmock.NewWithDSN(dsnPingFail, sqlmock.MonitorPingsOption(true))
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		mock.ExpectPing().WillReturnError(fmt.Errorf("ping error"))
		mock.ExpectClose() // We expect Close to be called after ping fails.

		logger, _ := zap.NewDevelopment()
		conn, err := NewConnection(cfgPingFail, logger)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if conn != nil {
			t.Fatal("expected nil connection, got conn")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %s", err)
		}
	})

	t.Run("open failure", func(t *testing.T) {
		dbDriverName = "invalid_driver_test_coverage_fix" // Ensures failure triggers correct error branch
		logger, _ := zap.NewDevelopment()
		conn, err := NewConnection(cfg, logger)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if conn != nil {
			t.Fatal("expected nil connection, got conn")
		}
	})
}

func TestGetEnabledSubscriptionsByUsers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	logger, _ := zap.NewDevelopment()
	testDB := &DB{
		sql:    db,
		logger: logger,
	}

	t.Run("empty user ids", func(t *testing.T) {
		result, err := testDB.GetEnabledSubscriptionsByUsers([]string{})
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got %v", result)
		}
	})

	t.Run("successful retrieval", func(t *testing.T) {
		userIDs := []string{"user1", "user2"}

		rows := sqlmock.NewRows([]string{"user_id", "event_type"}).
			AddRow("user1", "channel.follow").
			AddRow("user1", "channel.subscribe").
			AddRow("user2", "channel.cheer")

		mock.ExpectQuery("^SELECT user_id, event_type FROM twitch_subscriptions WHERE user_id = ANY\\(\\$1\\) AND status = 'enabled'$").
			WithArgs(pq.Array(userIDs)).
			WillReturnRows(rows)

		result, err := testDB.GetEnabledSubscriptionsByUsers(userIDs)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}

		if len(result) != 2 {
			t.Errorf("expected 2 users in result, got %d", len(result))
		}

		if !result["user1"]["channel.follow"] {
			t.Errorf("expected user1 to have channel.follow")
		}
		if !result["user1"]["channel.subscribe"] {
			t.Errorf("expected user1 to have channel.subscribe")
		}
		if !result["user2"]["channel.cheer"] {
			t.Errorf("expected user2 to have channel.cheer")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})

	t.Run("query error", func(t *testing.T) {
		userIDs := []string{"user3"}

		mock.ExpectQuery("^SELECT user_id, event_type FROM twitch_subscriptions WHERE user_id = ANY\\(\\$1\\) AND status = 'enabled'$").
			WithArgs(pq.Array(userIDs)).
			WillReturnError(fmt.Errorf("db error"))

		_, err := testDB.GetEnabledSubscriptionsByUsers(userIDs)
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})
}

func TestGetTwitchCredentials(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	logger, _ := zap.NewDevelopment()
	testDB := &DB{
		sql:    db,
		logger: logger,
	}

	userID := "user123"
	now := time.Now()

	t.Run("successful retrieval", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"access_token", "refresh_token", "expires_at"}).
			AddRow("access123", "refresh456", now)

		mock.ExpectQuery("^SELECT access_token, refresh_token, expires_at FROM twitch_credentials WHERE user_id = \\$1$").
			WithArgs(userID).
			WillReturnRows(rows)

		creds, err := testDB.GetTwitchCredentials(userID)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}

		if creds.UserID != userID {
			t.Errorf("expected UserID %s, got %s", userID, creds.UserID)
		}
		if creds.AccessToken != "access123" {
			t.Errorf("expected AccessToken access123, got %s", creds.AccessToken)
		}
		if creds.RefreshToken != "refresh456" {
			t.Errorf("expected RefreshToken refresh456, got %s", creds.RefreshToken)
		}
		if !creds.ExpiresAt.Equal(now) {
			t.Errorf("expected ExpiresAt %v, got %v", now, creds.ExpiresAt)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})

	t.Run("no rows error", func(t *testing.T) {
		mock.ExpectQuery("^SELECT access_token, refresh_token, expires_at FROM twitch_credentials WHERE user_id = \\$1$").
			WithArgs(userID).
			WillReturnError(sql.ErrNoRows)

		creds, err := testDB.GetTwitchCredentials(userID)

		if err != sql.ErrNoRows {
			t.Errorf("expected error %v, got %v", sql.ErrNoRows, err)
		}
		if creds.UserID != userID {
			t.Errorf("expected UserID %s, got %s", userID, creds.UserID)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})
}

func TestUpsertTwitchCredentials(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	logger, _ := zap.NewDevelopment()
	testDB := &DB{
		sql:    db,
		logger: logger,
	}

	creds := &TwitchCredentials{
		UserID:       "user123",
		AccessToken:  "access123",
		RefreshToken: "refresh456",
		ExpiresAt:    time.Now(),
	}

	t.Run("successful upsert", func(t *testing.T) {
		mock.ExpectExec("^\\s*INSERT INTO twitch_credentials \\(user_id, access_token, refresh_token, expires_at\\)\\s*VALUES \\(\\$1, \\$2, \\$3, \\$4\\)\\s*ON CONFLICT \\(user_id\\) DO UPDATE SET\\s*access_token = EXCLUDED\\.access_token,\\s*refresh_token = EXCLUDED\\.refresh_token,\\s*expires_at = EXCLUDED\\.expires_at\\s*$").
			WithArgs(creds.UserID, creds.AccessToken, creds.RefreshToken, creds.ExpiresAt).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := testDB.UpsertTwitchCredentials(creds)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})

	t.Run("upsert error", func(t *testing.T) {
		mock.ExpectExec("^\\s*INSERT INTO twitch_credentials \\(user_id, access_token, refresh_token, expires_at\\)\\s*VALUES \\(\\$1, \\$2, \\$3, \\$4\\)\\s*ON CONFLICT \\(user_id\\) DO UPDATE SET\\s*access_token = EXCLUDED\\.access_token,\\s*refresh_token = EXCLUDED\\.refresh_token,\\s*expires_at = EXCLUDED\\.expires_at\\s*$").
			WithArgs(creds.UserID, creds.AccessToken, creds.RefreshToken, creds.ExpiresAt).
			WillReturnError(fmt.Errorf("db error"))

		err := testDB.UpsertTwitchCredentials(creds)
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})
}
