package youtube

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/modules/chatflow/twitch"
	"VLX_ChatBridge/internal/modules/chatflow/websocket"

	"go.uber.org/zap"
	"google.golang.org/api/youtube/v3"
)

func TestProcessMessages(t *testing.T) {
	// 1. Setup Hub to capture broadcasts
	os.WriteFile("test.mp3", []byte("dummy"), 0644)
	defer os.Remove("test.mp3")
	logger := zap.NewNop()
	hub := websocket.NewHub(logger)

	// Create a dummy command map
	commands := twitch.AudioCommandsMap{
		"test": {Filename: "test.mp3", Permission: twitch.PermissionEveryone, MediaType: "audio"},
	}

	cfg := &config.Config{
		Overlay: config.OverlayConfig{
			Enable: true,
			Chat: config.OverlayTargetConfig{HTML: true},
			Alerts: config.OverlayTargetConfig{HTML: true},
		},
	}

	client := &Client{
		config:   cfg,
		hub:      hub,
		commands: commands,
		logger:   logger,
	}

	// 2. Prepare mock messages
	messages := []*youtube.LiveChatMessage{
		{
			Snippet: &youtube.LiveChatMessageSnippet{
				DisplayMessage:   "!test",
				SuperChatDetails: nil,
			},
			AuthorDetails: &youtube.LiveChatMessageAuthorDetails{
				DisplayName: "User1",
			},
		},
		{
			Snippet: &youtube.LiveChatMessageSnippet{
				SuperChatDetails: &youtube.LiveChatSuperChatDetails{
					AmountDisplayString: "$5.00",
					UserComment:         "Great stream!",
					Tier:                1,
				},
			},
			AuthorDetails: &youtube.LiveChatMessageAuthorDetails{
				DisplayName: "Donator1",
			},
		},
	}

	// 3. Run processing in a goroutine to not block reading from channel
	go func() {
		client.processMessages(messages)
	}()

	// 4. Assertions (Read from Hub.Broadcast)
	timeout := time.After(1 * time.Second)
	receivedCount := 0

	for i := 0; i < 2; i++ {
		select {
		case msg := <-hub.Broadcast:
			var payload map[string]interface{}
			if err := json.Unmarshal(msg, &payload); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			msgType := payload["type"].(string)
			if msgType == "sound_command" {
				if payload["filename"] != "test.mp3" {
					t.Errorf("Expected filename test.mp3, got %v", payload["filename"])
				}
			} else if msgType == "youtube_super_chat" {
				if payload["amount_string"] != "$5.00" {
					t.Errorf("Expected amount $5.00, got %v", payload["amount_string"])
				}
			} else {
				t.Errorf("Unexpected message type: %s", msgType)
			}
			receivedCount++
		case <-timeout:
			t.Fatal("Timeout waiting for broadcasts")
		}
	}

	if receivedCount != 2 {
		t.Errorf("Expected 2 broadcasts, got %d", receivedCount)
	}
}

func TestHandleCommand(t *testing.T) {
	os.WriteFile("test.mp3", []byte("dummy"), 0644)
	defer os.Remove("test.mp3")
	os.WriteFile("vip.mp3", []byte("dummy"), 0644)
	defer os.Remove("vip.mp3")
	os.WriteFile("sub.mp3", []byte("dummy"), 0644)
	defer os.Remove("sub.mp3")
	logger := zap.NewNop()
	hub := websocket.NewHub(logger)

	commands := twitch.AudioCommandsMap{
		"test":   {Filename: "test.mp3", Permission: twitch.PermissionEveryone, MediaType: "audio"},
		"vipcmd": {Filename: "vip.mp3", Permission: twitch.PermissionVIP, MediaType: "audio"},
		"subcmd": {Filename: "sub.mp3", Permission: twitch.PermissionSubscriber, MediaType: "audio"},
	}

	cfg := &config.Config{
		Overlay: config.OverlayConfig{
			Enable: true,
			Chat: config.OverlayTargetConfig{HTML: true},
		},
	}

	client := &Client{
		config:   cfg,
		commands: commands,
		logger:   logger,
		hub:      hub,
	}

	tests := []struct {
		name          string
		message       string
		author        *youtube.LiveChatMessageAuthorDetails
		expectTrigger bool
		expectedFile  string
	}{
		{
			name:          "Everyone command - normal user",
			message:       "!test",
			author:        &youtube.LiveChatMessageAuthorDetails{},
			expectTrigger: true,
			expectedFile:  "test.mp3",
		},
		{
			name:          "Everyone command - with args",
			message:       "!test arg1 arg2",
			author:        &youtube.LiveChatMessageAuthorDetails{},
			expectTrigger: true,
			expectedFile:  "test.mp3",
		},
		{
			name:          "Case insensitive command",
			message:       "!TEST",
			author:        &youtube.LiveChatMessageAuthorDetails{},
			expectTrigger: true,
			expectedFile:  "test.mp3",
		},
		{
			name:          "Unknown command",
			message:       "!unknown",
			author:        &youtube.LiveChatMessageAuthorDetails{},
			expectTrigger: false,
		},
		{
			name:          "VIP command - normal user",
			message:       "!vipcmd",
			author:        &youtube.LiveChatMessageAuthorDetails{},
			expectTrigger: false,
		},
		{
			name:          "VIP command - moderator",
			message:       "!vipcmd",
			author:        &youtube.LiveChatMessageAuthorDetails{IsChatModerator: true},
			expectTrigger: true,
			expectedFile:  "vip.mp3",
		},
		{
			name:          "VIP command - owner",
			message:       "!vipcmd",
			author:        &youtube.LiveChatMessageAuthorDetails{IsChatOwner: true},
			expectTrigger: true,
			expectedFile:  "vip.mp3",
		},
		{
			name:          "Sub command - normal user",
			message:       "!subcmd",
			author:        &youtube.LiveChatMessageAuthorDetails{},
			expectTrigger: false,
		},
		{
			name:          "Sub command - sponsor",
			message:       "!subcmd",
			author:        &youtube.LiveChatMessageAuthorDetails{IsChatSponsor: true},
			expectTrigger: true,
			expectedFile:  "sub.mp3",
		},
		{
			name:          "Sub command - moderator",
			message:       "!subcmd",
			author:        &youtube.LiveChatMessageAuthorDetails{IsChatModerator: true},
			expectTrigger: true,
			expectedFile:  "sub.mp3",
		},
		{
			name:          "Sub command - owner",
			message:       "!subcmd",
			author:        &youtube.LiveChatMessageAuthorDetails{IsChatOwner: true},
			expectTrigger: true,
			expectedFile:  "sub.mp3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := websocket.NewHub(logger)
			// Replace unbuffered channel with buffered channel to prevent blocking
			hub.Broadcast = make(chan []byte, 1)
			client.hub = hub

			client.handleCommand(tt.message, tt.author)

			if tt.expectTrigger {
				select {
				case msg := <-hub.Broadcast:
					var payload map[string]interface{}
					if err := json.Unmarshal(msg, &payload); err != nil {
						t.Fatalf("Failed to unmarshal JSON: %v", err)
					}
					if payload["filename"] != tt.expectedFile {
						t.Errorf("Expected filename %s, got %v", tt.expectedFile, payload["filename"])
					}
					if payload["type"] != "sound_command" {
						t.Errorf("Expected type sound_command, got %v", payload["type"])
					}
				default:
					t.Errorf("Expected broadcast but got none")
				}
			} else {
				select {
				case <-hub.Broadcast:
					t.Errorf("Expected no broadcast, but received one")
				default:
					// Passed
				}
			}
		})
	}
}
