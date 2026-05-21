package install

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v3"
)

func initializeDatabase(varDir string) error {
	dbPath := filepath.Join(varDir, "chatbridge.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	queries := []string{
		`CREATE TABLE IF NOT EXISTS twitch_credentials (
			user_id TEXT PRIMARY KEY,
			access_token TEXT NOT NULL,
			refresh_token TEXT NOT NULL,
			expires_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS twitch_subscriptions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS youtube_state (
			channel_id TEXT PRIMARY KEY,
			live_chat_id TEXT,
			next_page_token TEXT,
			updated_at DATETIME NOT NULL
		);`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %s, error: %w", query, err)
		}
	}

	return nil
}

func finalizeInstallation(etcDir, baseDir, chosenUser string) {
	fmt.Println("\nFinalizing installation...")

	varDir := filepath.Join(baseDir, "var")
	fmt.Println("Initializing database...")
	if err := initializeDatabase(varDir); err != nil {
		log.Printf("Warning: Failed to initialize database: %v", err)
	}

	// Update chatbridge_USER in settings
	settingsPath := filepath.Join(etcDir, "chatbridge.settings")
	if err := updateSetting(settingsPath, "chatbridge_USER", chosenUser); err != nil {
		log.Printf("Warning: Failed to update chatbridge_USER in %s: %v", settingsPath, err)
	} else {
		fmt.Printf("Updated chatbridge_USER to '%s' in %s\n", chosenUser, settingsPath)
	}

	dbPath := filepath.Join(varDir, "chatbridge.db")
	if err := updateSetting(settingsPath, "database.path", dbPath); err != nil {
		log.Printf("Warning: Failed to update database path in %s: %v", settingsPath, err)
	} else {
		fmt.Printf("Updated database path to '%s' in %s\n", dbPath, settingsPath)
	}

	// Change ownership recursively
	fmt.Printf("Changing ownership of %s to user '%s'...\n", baseDir, chosenUser)
	cmd := exec.Command("chown", "-R", fmt.Sprintf("%s:", chosenUser), baseDir)
	if err := cmd.Run(); err != nil {
		log.Printf("Warning: Failed to change ownership of %s: %v", baseDir, err)
	} else {
		fmt.Println("Ownership changed successfully.")
	}

	fmt.Println("\nInstallation complete.")
}

func updateSetting(settingsPath, key, value string) error {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, nothing to update
		}
		return err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	if len(root.Content) > 0 && root.Content[0].Kind == yaml.MappingNode {
		mapping := root.Content[0]
		keys := strings.Split(key, ".")

		updateNested(mapping, keys, value)
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, out, 0644)
}

func updateNested(mapping *yaml.Node, keys []string, value string) {
	if len(keys) == 0 {
		return
	}

	currentKey := keys[0]
	found := false

	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == currentKey {
			found = true
			if len(keys) == 1 {
				mapping.Content[i+1].Value = value
				mapping.Content[i+1].Style = 0
			} else {
				if mapping.Content[i+1].Kind == yaml.MappingNode {
					updateNested(mapping.Content[i+1], keys[1:], value)
				}
			}
			break
		}
	}

	if !found {
		if len(keys) == 1 {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: currentKey}
			valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: value}
			mapping.Content = append(mapping.Content, keyNode, valNode)
		} else {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: currentKey}
			valNode := &yaml.Node{Kind: yaml.MappingNode}
			mapping.Content = append(mapping.Content, keyNode, valNode)
			updateNested(valNode, keys[1:], value)
		}
	}
}
