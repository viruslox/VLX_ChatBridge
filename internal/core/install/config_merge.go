package install

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func handleConfigurationFiles(etcDir string) {
	fmt.Println("Processing configuration templates...")
	configDir := "config"

	// Find template files in config/ directory
	err := filepath.WalkDir(configDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".template") {
			// Determine target file name and path
			targetName := strings.TrimSuffix(d.Name(), ".template")
			targetPath := filepath.Join(etcDir, targetName)

			// Check if target file exists
			if _, err := os.Stat(targetPath); os.IsNotExist(err) {
				// Target doesn't exist, just copy
				if err := copyFile(path, targetPath, 0644); err != nil {
					log.Printf("Failed to copy template %s to %s: %v", path, targetPath, err)
				} else {
					fmt.Printf("Copied %s to %s\n", d.Name(), targetPath)
				}
			} else {
				// Target exists, merge
				fmt.Printf("Merging %s with existing %s\n", d.Name(), targetPath)
				if err := mergeYamlConfig(targetPath, path); err != nil {
					log.Printf("Failed to merge config %s: %v", targetPath, err)
				}
			}
		}
		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Config directory %s not found, skipping template processing", configDir)
		} else {
			log.Printf("Error processing config directory: %v", err)
		}
	}
}

func mergeYamlConfig(existingPath, templatePath string) error {
	existingData, err := os.ReadFile(existingPath)
	if err != nil {
		return err
	}
	templateData, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}

	var existingNode yaml.Node
	var templateNode yaml.Node

	if err := yaml.Unmarshal(existingData, &existingNode); err != nil {
		return err
	}
	if err := yaml.Unmarshal(templateData, &templateNode); err != nil {
		return err
	}

	if len(existingNode.Content) > 0 && len(templateNode.Content) > 0 {
		mergeNodes(existingNode.Content[0], templateNode.Content[0])
	}

	outData, err := yaml.Marshal(&existingNode)
	if err != nil {
		return err
	}

	return os.WriteFile(existingPath, outData, 0644)
}

func mergeNodes(dst, src *yaml.Node) {
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		return
	}

	for i := 0; i < len(src.Content); i += 2 {
		srcKey := src.Content[i].Value
		srcVal := src.Content[i+1]

		found := false
		for j := 0; j < len(dst.Content); j += 2 {
			dstKey := dst.Content[j].Value
			if dstKey == srcKey {
				found = true
				if dst.Content[j+1].Kind == yaml.MappingNode && srcVal.Kind == yaml.MappingNode {
					mergeNodes(dst.Content[j+1], srcVal)
				}
				break
			}
		}

		if !found {
			dst.Content = append(dst.Content, src.Content[i], srcVal)
		}
	}
}
