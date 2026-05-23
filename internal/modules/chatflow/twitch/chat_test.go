package twitch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gempir/go-twitch-irc/v4"
	"go.uber.org/zap"
)

func TestScanAudioCommands(t *testing.T) {
	// 1. Setup temporary directory structure
	tmpDir := t.TempDir()

	subDirs := []string{"everyone", "subscribers", "vips"}
	for _, dir := range subDirs {
		if err := os.Mkdir(filepath.Join(tmpDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// 2. Create dummy files
	files := []struct {
		path string
	}{
		{"everyone/hello.mp3"},
		{"subscribers/secret.wav"},
		{"vips/exclusive.mp4"},
		{"everyone/ignored.txt"}, // Should be ignored
	}

	for _, f := range files {
		fullPath := filepath.Join(tmpDir, f.path)
		if err := os.WriteFile(fullPath, []byte("dummy"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 3. Run Scan
	logger := zap.NewNop()
	cmds, err := ScanAudioCommands(tmpDir, logger)
	if err != nil {
		t.Fatalf("ScanAudioCommands failed: %v", err)
	}

	// 4. Assertions
	expectedCount := 3
	if len(cmds) != expectedCount {
		t.Errorf("Expected %d commands, got %d", expectedCount, len(cmds))
	}

	if data, ok := cmds["hello"]; !ok || data.Permission != PermissionEveryone || data.MediaType != "audio" {
		t.Errorf("Incorrect parsing for 'hello' command")
	}

	if data, ok := cmds["exclusive"]; !ok || data.Permission != PermissionVIP || data.MediaType != "video" {
		t.Errorf("Incorrect parsing for 'exclusive' command")
	}
}

func TestHasPermission(t *testing.T) {
	// Helper to create a ChatClient with minimal config
	client := &ChatClient{}

	tests := []struct {
		name          string
		userBadges    map[string]int
		requiredLevel string
		expected      bool
	}{
		// Everyone Level
		{"Everyone_NoBadges", map[string]int{}, PermissionEveryone, true},
		{"Everyone_WithSubBadge", map[string]int{"subscriber": 1}, PermissionEveryone, true},
		{"Everyone_WithBroadcaster", map[string]int{"broadcaster": 1}, PermissionEveryone, true},

		// Subscriber Level
		{"Sub_NoBadges", map[string]int{}, PermissionSubscriber, false},
		{"Sub_WithSubBadge", map[string]int{"subscriber": 1}, PermissionSubscriber, true},
		{"Sub_WithFounderBadge", map[string]int{"founder": 1}, PermissionSubscriber, true},
		{"Sub_WithVIPBadge", map[string]int{"vip": 1}, PermissionSubscriber, false},
		{"Sub_WithModBadge", map[string]int{"moderator": 1}, PermissionSubscriber, true},      // Mods inherit all
		{"Sub_WithBroadcaster", map[string]int{"broadcaster": 1}, PermissionSubscriber, true}, // Broadcaster inherits all

		// VIP Level
		{"VIP_NoBadges", map[string]int{}, PermissionVIP, false},
		{"VIP_WithVIPBadge", map[string]int{"vip": 1}, PermissionVIP, true},
		{"VIP_WithSubBadge", map[string]int{"subscriber": 1}, PermissionVIP, false},
		{"VIP_WithModBadge", map[string]int{"moderator": 1}, PermissionVIP, true},
		{"VIP_WithBroadcaster", map[string]int{"broadcaster": 1}, PermissionVIP, true},

		// Edge Cases
		{"InvalidLevel_Broadcaster", map[string]int{"broadcaster": 1}, "invalid", true}, // Broadcaster bypasses checks
		{"EmptyLevel_Moderator", map[string]int{"moderator": 1}, "", true},              // Moderator bypasses checks
		{"NoBadges_InvalidLevel", map[string]int{}, "invalid", false},
		{"NilBadges_Everyone", nil, PermissionEveryone, true},
		{"NilBadges_Subscriber", nil, PermissionSubscriber, false},
		{"NilBadges_VIP", nil, PermissionVIP, false},
		{"ZeroVersion_SubBadge", map[string]int{"subscriber": 0}, PermissionSubscriber, true},
		{"ZeroVersion_FounderBadge", map[string]int{"founder": 0}, PermissionSubscriber, true},
		{"ZeroVersion_VIPBadge", map[string]int{"vip": 0}, PermissionVIP, true},
		{"ZeroVersion_ModBadge", map[string]int{"moderator": 0}, PermissionSubscriber, true},
		{"ZeroVersion_BroadcasterBadge", map[string]int{"broadcaster": 0}, PermissionVIP, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := twitch.User{Badges: tt.userBadges}
			if got := client.hasPermission(user, tt.requiredLevel); got != tt.expected {
				t.Errorf("hasPermission() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFormatCommandList(t *testing.T) {
	tests := []struct {
		name     string
		commands AudioCommandsMap
		expected string
	}{
		{
			name:     "Empty commands",
			commands: make(AudioCommandsMap),
			expected: "No active commands found.",
		},
		{
			name: "All permission levels",
			commands: AudioCommandsMap{
				"hello": {Permission: PermissionEveryone},
				"bye":   {Permission: PermissionEveryone},
				"sub":   {Permission: PermissionSubscriber},
				"vip":   {Permission: PermissionVIP},
			},
			expected: "!bye, !hello / Subscribers: !sub / Vips: !vip",
		},
		{
			name: "Only everyone commands",
			commands: AudioCommandsMap{
				"hello": {Permission: PermissionEveryone},
			},
			expected: "!hello",
		},
		{
			name: "Only subscriber commands",
			commands: AudioCommandsMap{
				"sub": {Permission: PermissionSubscriber},
			},
			expected: "Subscribers: !sub",
		},
		{
			name: "Only vip commands",
			commands: AudioCommandsMap{
				"vip": {Permission: PermissionVIP},
			},
			expected: "Vips: !vip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &ChatClient{commands: tt.commands}
			got := client.formatCommandList()
			if got != tt.expected {
				t.Errorf("formatCommandList() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestScanAnnouncements(t *testing.T) {
	// 1. Setup temporary directory structure
	tmpDir := t.TempDir()

	announcementDir := filepath.Join(tmpDir, "announcements")
	if err := os.Mkdir(announcementDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 2. Create dummy files
	files := []struct {
		name    string
		content string
	}{
		{"socials_30.txt", "Follow me on twitter!"},
		{"discord_60.txt", "Join our discord!"},
		{"ignored.mp3", "dummy audio"}, // Should be ignored
		{"badformat.txt", "bad"},       // Should be ignored
		{"badinterval_abc.txt", "bad"}, // Should be ignored
		{"empty_10.txt", ""},           // Should be ignored
	}

	for _, f := range files {
		fullPath := filepath.Join(announcementDir, f.name)
		if err := os.WriteFile(fullPath, []byte(f.content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 3. Run Scan
	logger := zap.NewNop()
	cmds, err := ScanAnnouncements(tmpDir, logger)
	if err != nil {
		t.Fatalf("ScanAnnouncements failed: %v", err)
	}

	// 4. Assertions
	expectedCount := 2
	if len(cmds) != expectedCount {
		t.Errorf("Expected %d announcements, got %d", expectedCount, len(cmds))
	}

	if data, ok := cmds["socials"]; !ok || data.Interval != 30 || data.Content != "Follow me on twitter!" {
		t.Errorf("Incorrect parsing for 'socials' command: %+v", data)
	}

	if data, ok := cmds["discord"]; !ok || data.Interval != 60 || data.Content != "Join our discord!" {
		t.Errorf("Incorrect parsing for 'discord' command: %+v", data)
	}
}

func TestScanCommandFolder_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	logger := zap.NewNop()

	t.Run("Non-existent directory", func(t *testing.T) {
		cmds := make(AudioCommandsMap)
		scanCommandFolder(tmpDir, "doesnotexist", PermissionEveryone, cmds, logger)
		if len(cmds) != 0 {
			t.Errorf("Expected 0 commands, got %d", len(cmds))
		}
	})

	t.Run("ReadDir failure (path is a file)", func(t *testing.T) {
		cmds := make(AudioCommandsMap)
		folderName := "notafile"
		fullPath := filepath.Join(tmpDir, folderName)
		if err := os.WriteFile(fullPath, []byte("dummy"), 0644); err != nil {
			t.Fatal(err)
		}
		scanCommandFolder(tmpDir, folderName, PermissionEveryone, cmds, logger)
		if len(cmds) != 0 {
			t.Errorf("Expected 0 commands, got %d", len(cmds))
		}
	})

	t.Run("Subdirectory inside command folder is ignored", func(t *testing.T) {
		cmds := make(AudioCommandsMap)
		folderName := "subdirexists"
		fullPath := filepath.Join(tmpDir, folderName)
		if err := os.Mkdir(fullPath, 0755); err != nil {
			t.Fatal(err)
		}
		subDirPath := filepath.Join(fullPath, "ignored_dir")
		if err := os.Mkdir(subDirPath, 0755); err != nil {
			t.Fatal(err)
		}
		scanCommandFolder(tmpDir, folderName, PermissionEveryone, cmds, logger)
		if len(cmds) != 0 {
			t.Errorf("Expected 0 commands, got %d", len(cmds))
		}
	})

	t.Run("Duplicate command", func(t *testing.T) {
		cmds := make(AudioCommandsMap)
		cmds["hello"] = CommandData{Filename: "original", Permission: PermissionEveryone, MediaType: "audio"}
		folderName := "dupfolder"
		fullPath := filepath.Join(tmpDir, folderName)
		if err := os.Mkdir(fullPath, 0755); err != nil {
			t.Fatal(err)
		}
		filePath := filepath.Join(fullPath, "hello.mp3")
		if err := os.WriteFile(filePath, []byte("dummy"), 0644); err != nil {
			t.Fatal(err)
		}
		scanCommandFolder(tmpDir, folderName, PermissionEveryone, cmds, logger)
		if cmds["hello"].Filename != "original" {
			t.Errorf("Expected original command to be retained, got %s", cmds["hello"].Filename)
		}
	})
}

func TestScanAnnouncements_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	logger := zap.NewNop()

	t.Run("Non-existent directory", func(t *testing.T) {
		cmds, err := ScanAnnouncements(tmpDir, logger)
		if err != nil {
			t.Errorf("Expected nil error for non-existent directory, got %v", err)
		}
		if cmds != nil {
			t.Errorf("Expected nil cmds, got %v", cmds)
		}
	})

	t.Run("ReadDir failure (path is a file)", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "announcements")
		if err := os.WriteFile(filePath, []byte("dummy"), 0644); err != nil {
			t.Fatal(err)
		}

		cmds, err := ScanAnnouncements(tmpDir, logger)
		if err == nil {
			t.Errorf("Expected error for ReadDir failure, got nil")
		}
		if cmds != nil {
			t.Errorf("Expected nil cmds, got %v", cmds)
		}
		os.Remove(filePath)
	})

	t.Run("Subdirectory is ignored", func(t *testing.T) {
		announcementDir := filepath.Join(tmpDir, "announcements")
		if err := os.Mkdir(announcementDir, 0755); err != nil {
			t.Fatal(err)
		}

		subDirPath := filepath.Join(announcementDir, "ignored_dir")
		if err := os.Mkdir(subDirPath, 0755); err != nil {
			t.Fatal(err)
		}

		cmds, err := ScanAnnouncements(tmpDir, logger)
		if err != nil {
			t.Fatalf("Expected nil error, got %v", err)
		}
		if len(cmds) != 0 {
			t.Errorf("Expected 0 announcements, got %d", len(cmds))
		}
		os.RemoveAll(announcementDir)
	})

	t.Run("Duplicate announcement", func(t *testing.T) {
		announcementDir := filepath.Join(tmpDir, "announcements")
		if err := os.Mkdir(announcementDir, 0755); err != nil {
			t.Fatal(err)
		}

		file1 := filepath.Join(announcementDir, "socials_30.txt")
		file2 := filepath.Join(announcementDir, "socials_60.txt")

		if err := os.WriteFile(file1, []byte("first"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file2, []byte("second"), 0644); err != nil {
			t.Fatal(err)
		}

		cmds, err := ScanAnnouncements(tmpDir, logger)
		if err != nil {
			t.Fatalf("Expected nil error, got %v", err)
		}

		if len(cmds) != 1 {
			t.Errorf("Expected 1 announcement, got %d", len(cmds))
		}

		if data, ok := cmds["socials"]; !ok || (data.Interval != 30 && data.Interval != 60) {
			t.Errorf("Expected socials announcement to be parsed, got: %+v", data)
		}
		os.RemoveAll(announcementDir)
	})

	t.Run("Unreadable file error", func(t *testing.T) {
		announcementDir := filepath.Join(tmpDir, "announcements")
		if err := os.Mkdir(announcementDir, 0755); err != nil {
			t.Fatal(err)
		}

		filePath := filepath.Join(announcementDir, "unreadable_30.txt")
		if err := os.WriteFile(filePath, []byte("dummy"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filePath, 0222); err != nil { // write-only
			t.Fatal(err)
		}

		if _, err := os.ReadFile(filePath); err == nil {
			t.Skip("Skipping test as environment bypasses read restrictions")
		}

		// Attempting to read a file with restrictive permissions
		cmds, err := ScanAnnouncements(tmpDir, logger)
		if err != nil {
			t.Fatalf("Expected nil error, got %v", err)
		}
		if len(cmds) != 0 {
			t.Errorf("Expected 0 announcements due to read error, got %d", len(cmds))
		}

		os.RemoveAll(announcementDir)
	})
}
