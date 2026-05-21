package twitch

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/modules/chatflow/audio"
	"path/filepath"
	"VLX_ChatBridge/internal/modules/chatflow/database"
	"VLX_ChatBridge/internal/modules/chatflow/websocket"

	"github.com/nicklaw5/helix/v2"
	"go.uber.org/zap"
)

// EventSub constants
const (
	EventSubFollow     = "channel.follow"
	EventSubSubscribe  = "channel.subscribe"
	EventSubSubGift    = "channel.subscription.gift"
	EventSubSubMessage = "channel.subscription.message"
	EventSubCheer      = "channel.cheer"
	EventSubRaid       = "channel.raid"
)

// Client manages Twitch API interactions and EventSub webhooks.
type Client struct {
	config      *config.Config
	helix       *helix.Client
	hub         *websocket.Hub
	db          *database.DB
	selfBaseURL string
	logger      *zap.Logger
}

// setupHelixClient initializes the Helix client and sets the app access token.
func setupHelixClient(cfg config.TwitchConfig) (*helix.Client, error) {
	helixClient, err := helix.NewClient(&helix.Options{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create helix client: %w", err)
	}

	appToken, err := helixClient.RequestAppAccessToken(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate app access token: %w", err)
	}
	if appToken.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to generate app access token, status: %d, message: %s", appToken.StatusCode, appToken.ErrorMessage)
	}
	helixClient.SetAppAccessToken(appToken.Data.AccessToken)

	return helixClient, nil
}

// NewClient initializes the Twitch client with database-backed token management.
func NewClient(cfg *config.Config, monitoringChannels []string, baseURL string, hub *websocket.Hub, db *database.DB, logger *zap.Logger) (*Client, error) {
	helixClient, err := setupHelixClient(cfg.Twitch)
	if err != nil {
		return nil, err
	}

	c := &Client{
		helix:       helixClient,
		db:          db,
		hub:         hub,
		config:      cfg,
		selfBaseURL: baseURL,
		logger:      logger,
	}

	// 2. Verify User Permissions (using DB or Config)
	if len(monitoringChannels) == 0 {
		return nil, errors.New("monitoring channels list is empty")
	}
	primaryLogin := monitoringChannels[0]
	userID := c.resolveUserID(primaryLogin)

	// 3. Maintain User Token Lifecycle (Refresh if needed)
	c.setupUserToken(userID, cfg.Twitch)

	// 4. FINAL STEP: Ensure Client uses App Token
	helixClient.SetUserAccessToken("")
	logger.Info("Twitch Client initialized (App Access Token active)")

	return c, nil
}

// resolveUserID fetches the user ID for a given login.
func (c *Client) resolveUserID(login string) string {
	usersResp, err := c.helix.GetUsers(&helix.UsersParams{Logins: []string{login}})
	if err != nil || usersResp.StatusCode != http.StatusOK || len(usersResp.Data.Users) == 0 {
		c.logger.Error("Could not resolve user ID", zap.String("login", login))
		return ""
	}
	return usersResp.Data.Users[0].ID
}

// setupUserToken maintains the user token lifecycle.
func (c *Client) setupUserToken(userID string, cfg config.TwitchConfig) {
	if userID == "" {
		c.logger.Warn("Cannot maintain user token: userID is empty")
		return
	}

	err := c.maintainUserToken(userID, cfg)
	if err != nil {
		c.logger.Warn("User token maintenance failed. EventSub might still work if App authorized.", zap.Error(err))
	}
}

// validateAndSaveConfigToken verifies the config token and saves it to the database.
func (c *Client) validateAndSaveConfigToken(expectedUserID, token string) error {
	isValid, validateResp, err := c.helix.ValidateToken(token)
	if err != nil {
		return fmt.Errorf("failed to validate config token: %w", err)
	}
	if !isValid || validateResp == nil {
		return errors.New("config token is invalid")
	}

	if validateResp.Data.UserID != expectedUserID {
		return fmt.Errorf("config token belongs to user %s, but expected user %s", validateResp.Data.UserID, expectedUserID)
	}

	creds := &database.TwitchCredentials{
		UserID:       expectedUserID,
		AccessToken:  token,
		RefreshToken: "", // Config tokens usually don't have refresh tokens in config
		ExpiresAt:    time.Now().UTC().Add(time.Second * time.Duration(validateResp.Data.ExpiresIn)),
	}

	if err := c.db.UpsertTwitchCredentials(creds); err != nil {
		return fmt.Errorf("failed to save config token to DB: %w", err)
	}

	c.logger.Info("Successfully validated and saved config token to DB", zap.String("userID", expectedUserID))
	return nil
}

// maintainUserToken checks DB, validates/refreshes the user token to keep it alive.
func (c *Client) maintainUserToken(userID string, cfg config.TwitchConfig) error {
	creds, err := c.db.GetTwitchCredentials(userID)
	if err != nil {
		if err == sql.ErrNoRows {
			// Removed config token fallback as it was removed from configuration struct
			return errors.New("no credentials in DB")
		}
		return err
	}

	if time.Now().UTC().After(creds.ExpiresAt) {
		c.logger.Info("User access token expired in DB. Refreshing...")
		_, err := c.refreshToken(creds)
		if err != nil {
			return fmt.Errorf("refresh failed: %w", err)
		}
		c.logger.Info("User token refreshed in DB")
	} else {
		c.logger.Info("User token in DB is valid")
	}
	return nil
}

// refreshToken refreshes the User Access Token and updates the database.
func (c *Client) refreshToken(creds *database.TwitchCredentials) (*database.TwitchCredentials, error) {
	if creds.RefreshToken == "" {
		return nil, errors.New("empty refresh token")
	}

	token, err := c.helix.RefreshUserAccessToken(creds.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("helix refresh failed: %w", err)
	}
	if token.StatusCode >= 400 {
		return nil, fmt.Errorf("api refresh error %d: %s", token.StatusCode, token.ErrorMessage)
	}

	newCreds := &database.TwitchCredentials{
		UserID:       creds.UserID,
		AccessToken:  token.Data.AccessToken,
		RefreshToken: token.Data.RefreshToken,
		ExpiresAt:    time.Now().UTC().Add(time.Second * time.Duration(token.Data.ExpiresIn)),
	}

	if err := c.db.UpsertTwitchCredentials(newCreds); err != nil {
		return nil, fmt.Errorf("db update failed: %w", err)
	}

	return newCreds, nil
}

// StartMonitoring sets up EventSub subscriptions for the configured channels.
func (c *Client) StartMonitoring(channelLogins []string) error {
	if c.selfBaseURL == "" {
		return errors.New("baseURL is empty")
	}

	usersResp, err := c.helix.GetUsers(&helix.UsersParams{Logins: channelLogins})
	if err != nil || usersResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to resolve users: %w", err)
	}

	if len(usersResp.Data.Users) == 0 {
		return nil
	}

	userIDs := make([]string, 0, len(usersResp.Data.Users))
	for _, user := range usersResp.Data.Users {
		userIDs = append(userIDs, user.ID)
	}

	activeSubs, err := c.db.GetEnabledSubscriptionsByUsers(userIDs)
	if err != nil {
		c.logger.Error("Failed to fetch active subscriptions, proceeding with normal checks", zap.Error(err))
		activeSubs = make(map[string]map[string]bool)
	}

	callbackURL := c.selfBaseURL + "/webhooks/twitch"

	var wg sync.WaitGroup
	for _, user := range usersResp.Data.Users {
		wg.Add(1)
		go func(u helix.User) {
			defer wg.Done()
			c.logger.Info("Subscribing to events", zap.String("user", u.Login), zap.String("id", u.ID))
			userSubs := activeSubs[u.ID]
			c.subscribeToEvent(u.ID, EventSubFollow, "2", callbackURL, userSubs[EventSubFollow])
			c.subscribeToRaidEvent(u.ID, callbackURL, userSubs[EventSubRaid])
			c.subscribeToEvent(u.ID, EventSubSubscribe, "1", callbackURL, userSubs[EventSubSubscribe])
			c.subscribeToEvent(u.ID, EventSubSubGift, "1", callbackURL, userSubs[EventSubSubGift])
			c.subscribeToEvent(u.ID, EventSubSubMessage, "1", callbackURL, userSubs[EventSubSubMessage])
			c.subscribeToEvent(u.ID, EventSubCheer, "1", callbackURL, userSubs[EventSubCheer])
		}(user)
	}
	wg.Wait()
	return nil
}

// subscribeToEvent creates a subscription if not already active in the DB.
func (c *Client) subscribeToEvent(userID, eventType, version, callbackURL string, isActive bool) {
	if isActive {
		return // Already active
	}

	newSub, err := c.createSubscription(userID, eventType, version, callbackURL)
	if err != nil {
		c.logger.Error("Subscription failed", zap.String("type", eventType), zap.Error(err))
		return
	}

	if err := c.saveSubscriptionToDB(userID, eventType, newSub); err != nil {
		c.logger.Info("Synced subscription to DB", zap.String("type", eventType))
	}
}

// subscribeToRaidEvent handles the specific requirements for raid subscriptions.
func (c *Client) subscribeToRaidEvent(userID, callbackURL string, isActive bool) {
	if isActive {
		return
	}

	newSub, err := c.createRaidSubscription(userID, callbackURL)
	if err != nil {
		c.logger.Error("Raid subscription failed", zap.Error(err))
		return
	}

	if err := c.saveSubscriptionToDB(userID, EventSubRaid, newSub); err != nil {
		c.logger.Info("Synced raid subscription to DB")
	}
}

// saveSubscriptionToDB persists subscription details.
func (c *Client) saveSubscriptionToDB(userID, eventType string, sub *helix.EventSubSubscription) error {
	return c.db.CreateSubscription(&database.TwitchSubscription{
		ID:        sub.ID,
		UserID:    userID,
		EventType: eventType,
		Status:    sub.Status,
		CreatedAt: sub.CreatedAt.Time,
	})
}

// createSubscription performs the API call for standard events with 409 auto-recovery.
func (c *Client) createSubscription(userID, eventType, version, callbackURL string) (*helix.EventSubSubscription, error) {
	condition := helix.EventSubCondition{BroadcasterUserID: userID}
	if eventType == EventSubFollow {
		condition.ModeratorUserID = userID
	}


	webhookSecret := c.config.Twitch.WebhookSecret


	resp, err := c.helix.CreateEventSubSubscription(&helix.EventSubSubscription{
		Type:      eventType,
		Version:   version,
		Condition: condition,
		Transport: helix.EventSubTransport{
			Method:   "webhook",
			Callback: callbackURL,
			Secret:   webhookSecret,
		},
	})

	// Handle 409 Conflict (Already Exists) by fetching the existing one
	if resp != nil && resp.StatusCode == 409 {
		return c.fetchExistingSubscription(userID, eventType)
	}

	return c.handleSubscriptionResponse(resp, err)
}

// createRaidSubscription performs the API call for raid events with 409 auto-recovery.
func (c *Client) createRaidSubscription(userID, callbackURL string) (*helix.EventSubSubscription, error) {

	webhookSecret := c.config.Twitch.WebhookSecret


	resp, err := c.helix.CreateEventSubSubscription(&helix.EventSubSubscription{
		Type:    EventSubRaid,
		Version: "1",
		Condition: helix.EventSubCondition{
			ToBroadcasterUserID: userID,
		},
		Transport: helix.EventSubTransport{
			Method:   "webhook",
			Callback: callbackURL,
			Secret:   webhookSecret,
		},
	})

	// Handle 409 Conflict (Already Exists)
	if resp != nil && resp.StatusCode == 409 {
		return c.fetchExistingSubscription(userID, EventSubRaid)
	}

	return c.handleSubscriptionResponse(resp, err)
}

// fetchExistingSubscription retrieves all subs of a type and filters client-side to ensure we find it.
func (c *Client) fetchExistingSubscription(userID, eventType string) (*helix.EventSubSubscription, error) {
	opts := &helix.EventSubSubscriptionsParams{
		Type: eventType,
	}

	resp, err := c.helix.GetEventSubSubscriptions(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch existing sub list: %w", err)
	}

	for _, sub := range resp.Data.EventSubSubscriptions {
		// Special handling for Raid: The "UserID" we track is the TARGET of the raid (ToBroadcasterUserID)
		if eventType == EventSubRaid {
			if sub.Condition.ToBroadcasterUserID == userID {
				c.logger.Info("Found existing Raid subscription", zap.String("id", sub.ID), zap.String("status", sub.Status))
				return &sub, nil
			}
			continue
		}

		// Standard handling: The "UserID" is the BroadcasterUserID
		if sub.Condition.BroadcasterUserID == userID {
			c.logger.Info("Found existing subscription", zap.String("type", eventType), zap.String("id", sub.ID), zap.String("status", sub.Status))
			return &sub, nil
		}
	}

	return nil, fmt.Errorf("got 409 from Twitch but could not find subscription for user %s in the list", userID)
}

// handleSubscriptionResponse processes the Helix response.
func (c *Client) handleSubscriptionResponse(resp *helix.EventSubSubscriptionsResponse, err error) (*helix.EventSubSubscription, error) {
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("api status %d: %s", resp.StatusCode, resp.ErrorMessage)
	}
	if len(resp.Data.EventSubSubscriptions) == 0 {
		return nil, errors.New("no subscription data returned")
	}
	return &resp.Data.EventSubSubscriptions[0], nil
}

// HandleEventSubCallback processes incoming webhooks.
func (c *Client) HandleEventSubCallback(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	if !c.verifyEventSubSignature(r, body) {
		http.Error(w, "Invalid Signature", http.StatusUnauthorized)
		return
	}

	messageType := r.Header.Get("Twitch-Eventsub-Message-Type")

	switch messageType {
	case "webhook_callback_verification":
		c.handleWebhookVerification(w, body)

	case "notification":
		c.handleWebhookNotificationPayload(w, body)

	case "revocation":
		c.handleWebhookRevocation(w, body)

	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (c *Client) handleWebhookVerification(w http.ResponseWriter, body []byte) {
	var verification struct {
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(body, &verification); err == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(verification.Challenge))
	} else {
		http.Error(w, "Bad Request", http.StatusBadRequest)
	}
}

func (c *Client) handleWebhookNotificationPayload(w http.ResponseWriter, body []byte) {
	var notification struct {
		Subscription helix.EventSubSubscription `json:"subscription"`
		Event        json.RawMessage            `json:"event"`
	}
	if err := json.Unmarshal(body, &notification); err == nil {
		c.handleNotification(notification.Subscription.Type, notification.Event)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		http.Error(w, "Bad Request", http.StatusBadRequest)
	}
}

func (c *Client) handleWebhookRevocation(w http.ResponseWriter, body []byte) {
	var revocation struct {
		Subscription helix.EventSubSubscription `json:"subscription"`
	}
	if err := json.Unmarshal(body, &revocation); err == nil {
		c.logger.Warn("Subscription revoked", zap.String("id", revocation.Subscription.ID))
		c.db.DeleteSubscription(revocation.Subscription.ID)
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// verifyEventSubSignature validates the HMAC signature.
func (c *Client) verifyEventSubSignature(r *http.Request, body []byte) bool {
	id := r.Header.Get("Twitch-Eventsub-Message-Id")
	ts := r.Header.Get("Twitch-Eventsub-Message-Timestamp")
	sig := r.Header.Get("Twitch-Eventsub-Message-Signature")

	prefix := "sha256="
	if len(sig) < len(prefix) {
		return false
	}


	webhookSecret := c.config.Twitch.WebhookSecret


	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte(id + ts))
	mac.Write(body)
	expected := prefix + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expected))
}

// handleFollowEvent processes follow event payloads.
func (c *Client) handleFollowEvent(eventData json.RawMessage) (map[string]interface{}, error) {
	var e helix.EventSubChannelFollowEvent
	if err := json.Unmarshal(eventData, &e); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"type":      "twitch_follow",
		"user_name": e.UserName,
	}, nil
}

// handleSubscribeEvent processes subscription event payloads.
func (c *Client) handleSubscribeEvent(eventData json.RawMessage) (map[string]interface{}, error) {
	var e helix.EventSubChannelSubscribeEvent
	if err := json.Unmarshal(eventData, &e); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"type":      "twitch_subscribe",
		"user_name": e.UserName,
		"tier":      e.Tier,
		"is_gift":   e.IsGift,
	}, nil
}

// handleSubMessageEvent processes sub message event payloads.
func (c *Client) handleSubMessageEvent(eventData json.RawMessage) (map[string]interface{}, error) {
	var e helix.EventSubChannelSubscriptionMessageEvent
	if err := json.Unmarshal(eventData, &e); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"type":              "twitch_resubscribe",
		"user_name":         e.UserName,
		"tier":              e.Tier,
		"message":           e.Message.Text,
		"cumulative_months": e.CumulativeMonths,
	}, nil
}

// handleSubGiftEvent processes sub gift event payloads.
func (c *Client) handleSubGiftEvent(eventData json.RawMessage) (map[string]interface{}, error) {
	var e helix.EventSubChannelSubscriptionGiftEvent
	if err := json.Unmarshal(eventData, &e); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"type":        "twitch_gift_sub",
		"gifter_name": e.UserName,
		"total_gifts": e.Total,
		"tier":        e.Tier,
	}, nil
}

// handleCheerEvent processes cheer event payloads.
func (c *Client) handleCheerEvent(eventData json.RawMessage) (map[string]interface{}, error) {
	var e helix.EventSubChannelCheerEvent
	if err := json.Unmarshal(eventData, &e); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"type":      "twitch_cheer",
		"user_name": e.UserName,
		"bits":      e.Bits,
		"message":   e.Message,
	}, nil
}

// handleRaidEvent processes raid event payloads.
func (c *Client) handleRaidEvent(eventData json.RawMessage) (map[string]interface{}, error) {
	var e helix.EventSubChannelRaidEvent
	if err := json.Unmarshal(eventData, &e); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"type":        "twitch_raid",
		"raider_name": e.FromBroadcasterUserName,
		"viewers":     e.Viewers,
	}, nil
}

// handleNotification distributes events to the WebSocket hub.
func (c *Client) handleNotification(eventType string, eventData json.RawMessage) {
	var payload map[string]interface{}
	var err error

	switch eventType {
	case EventSubFollow:
		payload, err = c.handleFollowEvent(eventData)
	case EventSubSubscribe:
		payload, err = c.handleSubscribeEvent(eventData)
	case EventSubSubMessage:
		payload, err = c.handleSubMessageEvent(eventData)
	case EventSubSubGift:
		payload, err = c.handleSubGiftEvent(eventData)
	case EventSubCheer:
		payload, err = c.handleCheerEvent(eventData)
	case EventSubRaid:
		payload, err = c.handleRaidEvent(eventData)
	}

	if err != nil {
		c.logger.Error("Failed to parse event", zap.String("type", eventType), zap.Error(err))
		return
	}
	if payload != nil {

		htmlEnabled := c.config.Overlay.Enable
		streamingEnabled := c.config.Overlay.Alerts.Streaming
		discordEnabled := c.config.Overlay.Alerts.Discord


		if htmlEnabled {
			c.hub.BroadcastJSON(payload)
		}

		if streamingEnabled || discordEnabled {
			fullPath := filepath.Join(c.config.ChatBridgeDIR, "static", "alerts", "alert.mp3")
			go audio.PlayAlert("twitch_alert", fullPath, bool(streamingEnabled), bool(discordEnabled))
		}
	}
}
