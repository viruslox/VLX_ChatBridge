package install

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func finalizeInstallation(etcDir, baseDir, chosenUser string) {
	fmt.Println("\nFinalizing installation...")

	// Update chatbridge_USER in settings
	settingsPath := filepath.Join(etcDir, "chatbridge.settings")
	if err := updateSetting(settingsPath, "chatbridge_USER", chosenUser); err != nil {
		log.Printf("Warning: Failed to update chatbridge_USER in %s: %v", settingsPath, err)
	} else {
		fmt.Printf("Updated chatbridge_USER to '%s' in %s\n", chosenUser, settingsPath)
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
		found := false
		for i := 0; i < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == key {
				mapping.Content[i+1].Value = value
				mapping.Content[i+1].Style = 0
				found = true
				break
			}
		}

		// Note: we just update it if it exists, if it doesn't we append
		if !found {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
			valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: value}
			mapping.Content = append(mapping.Content, keyNode, valNode)
		}
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, out, 0644)
}
