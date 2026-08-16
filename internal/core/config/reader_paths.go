package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFlags reads the settings file once and resolves each requested dotted key
// path to its YesNoBool value. Keys that are absent from the file are simply
// omitted from the result (the caller treats a missing key as "not set") rather
// than causing an error, so an older settings file missing an optional flag
// still yields a usable status snapshot.
//
// It reuses the same node walker as the writer, so a value is read exactly as it
// is stored, without expanding ${ENV} references or requiring the typed Config.
func LoadFlags(configPath string, paths map[string][]string) (map[string]bool, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read settings file: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse settings file: %w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("settings file root is not a YAML mapping")
	}

	result := make(map[string]bool, len(paths))
	for key, kp := range paths {
		leaf, err := findScalar(root.Content[0], kp)
		if err != nil {
			continue
		}
		result[key] = leaf.Value == "yes"
	}
	return result, nil
}
