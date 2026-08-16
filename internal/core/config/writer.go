package config

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// writeMu serializes concurrent writes to the settings file so two toggles
// arriving at once cannot interleave a read-modify-write and lose an update.
var writeMu sync.Mutex

// ModuleEnabledPaths maps a module's manager name (Module.Name()) to the dotted
// key path of its enabled flag in the settings file. It is the single source of
// truth shared by the boot loop and the control API.
var ModuleEnabledPaths = map[string][]string{
	"Server":      {"modules", "server_enabled"},
	"ChatFlow":    {"modules", "chatflow_enabled"},
	"AudioBridge": {"modules", "audiobridge_enabled"},
	"Streaming":   {"modules", "streaming_enabled"},
	"AudioSource": {"modules", "audio_source_enabled"},
	"Connector":   {"modules", "connector_enabled"},
}

// yesNo renders a Go bool as the settings file's YesNoBool scalar.
func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// SetModuleEnabled persists a top-level module's enabled flag to the settings
// file at configPath.
func SetModuleEnabled(configPath, moduleName string, enabled bool) error {
	keys, ok := ModuleEnabledPaths[moduleName]
	if !ok {
		return fmt.Errorf("unknown module %q", moduleName)
	}
	return SetBoolByPath(configPath, keys, enabled)
}

// SetBoolByPath atomically sets a YesNoBool flag identified by a dotted key path
// (e.g. []string{"overlay","alerts","discord"}) in the settings file.
//
// It edits the YAML node tree in place and re-serializes it, so comments, key
// order, and unexpanded ${ENV} references are all preserved: the file is never
// round-tripped through the typed Config struct, which would bake in expanded
// environment values and strip comments. Only the scalar's value is changed
// (never its style), so the flag stays an unquoted plain scalar -- required,
// because YesNoBool rejects quoted values on load.
//
// The leaf key must already exist; a missing key is reported rather than
// inserted, so the control API only offers toggles for flags that are present.
func SetBoolByPath(configPath string, keys []string, enabled bool) error {
	if len(keys) == 0 {
		return fmt.Errorf("empty settings key path")
	}

	writeMu.Lock()
	defer writeMu.Unlock()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read settings file: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("failed to parse settings file: %w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("settings file root is not a YAML mapping")
	}

	leaf, err := findScalar(root.Content[0], keys)
	if err != nil {
		return err
	}
	leaf.Value = yesNo(enabled)

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("failed to serialize settings file: %w", err)
	}

	stat, err := os.Stat(configPath)
	mode := os.FileMode(0o644)
	if err == nil {
		mode = stat.Mode()
	}

	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, out, mode); err != nil {
		return fmt.Errorf("failed to write temporary settings file: %w", err)
	}
	if err := os.Rename(tmp, configPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to replace settings file: %w", err)
	}
	return nil
}

// findScalar walks a mapping node along keys and returns the leaf scalar value
// node, or an error naming the first path segment that could not be resolved.
func findScalar(mapping *yaml.Node, keys []string) (*yaml.Node, error) {
	cur := mapping
	for depth, key := range keys {
		if cur.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("settings path %q: %q is not a mapping",
				strings.Join(keys, "."), strings.Join(keys[:depth], "."))
		}

		var next *yaml.Node
		for i := 0; i+1 < len(cur.Content); i += 2 {
			if cur.Content[i].Value == key {
				next = cur.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil, fmt.Errorf("settings key %q not found", strings.Join(keys[:depth+1], "."))
		}

		if depth == len(keys)-1 {
			if next.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("settings key %q is not a scalar", strings.Join(keys, "."))
			}
			return next, nil
		}
		cur = next
	}
	return nil, fmt.Errorf("settings path %q not resolved", strings.Join(keys, "."))
}
