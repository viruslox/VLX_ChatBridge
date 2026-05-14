package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Modules      ModulesConfig           `yaml:"modules"`
	Server       ServerConfig            `yaml:"server"`
	Database     DatabaseConfig          `yaml:"database"`
	Twitch       TwitchConfig            `yaml:"twitch"`
	YouTube      YouTubeConfig           `yaml:"youtube"`
	Overlay      OverlayConfig           `yaml:"overlay"`
	Discord      DiscordConfig           `yaml:"discord"`
	Streaming    StreamingConfig         `yaml:"streaming"`
	AudioSources map[string]AudioSource  `yaml:"audio_source"`
}

type ModulesConfig struct {
	ChatFlowEnabled    bool `yaml:"chatflow_enabled"`
	AudioBridgeEnabled bool `yaml:"audiobridge_enabled"`
}

type ServerConfig struct {
	BaseURL  string `yaml:"base_url"`
	Port     string `yaml:"port"`
	TestPort string `yaml:"test_port"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
}

type TwitchConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type YouTubeConfig struct {
	APIKey    string `yaml:"api_key"`
	ChannelID string `yaml:"channel_id"`
}

type OverlayConfig struct {
	Enable bool `yaml:"enable"`
}

type DiscordConfig struct {
	Token  string   `yaml:"token"`
	Prefix string   `yaml:"prefix"`
	Admins []string `yaml:"admins"`
}

type StreamingConfig struct {
	DestinationURL string `yaml:"destination_url"`
	Bitrate        string `yaml:"bitrate"`
}

type AudioSource struct {
	Enable bool   `yaml:"enable"`
	Type   string `yaml:"type"`
	Volume int    `yaml:"volume"`
	URL    string `yaml:"url"`
}

// LoadConfig reads and parses the configuration file at the given path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Simple environment variable expansion
	expandedData := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expandedData), &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}
