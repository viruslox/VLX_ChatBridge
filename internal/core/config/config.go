package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type YesNoBool bool

func (b *YesNoBool) UnmarshalYAML(value *yaml.Node) error {
	if value.Style == yaml.DoubleQuotedStyle || value.Style == yaml.SingleQuotedStyle {
		return fmt.Errorf("quotes are not allowed for boolean values: %q", value.Value)
	}
	val := strings.ToLower(value.Value)
	if val == "yes" {
		*b = true
	} else if val == "no" {
		*b = false
	} else {
		return fmt.Errorf("invalid boolean value %q, only 'yes' and 'no' are allowed", value.Value)
	}
	return nil
}

func (b YesNoBool) MarshalYAML() (interface{}, error) {
	if b {
		return "yes", nil
	}
	return "no", nil
}

type Config struct {
	ChatBridgeDIR string                  `yaml:"chatbridge_DIR"`
	Modules      ModulesConfig           `yaml:"modules"`
	Server       ServerConfig            `yaml:"server"`
	Database     DatabaseConfig          `yaml:"database"`
	Twitch       TwitchConfig            `yaml:"twitch"`
	YouTube      YouTubeConfig           `yaml:"youtube"`
	Overlay      OverlayConfig           `yaml:"overlay"`
	Discord     DiscordConfig     `yaml:"discord"`
	Streaming   StreamingConfig   `yaml:"streaming"`
	AudioSource AudioSourceConfig `yaml:"audio_source"`
}

type ModulesConfig struct {
	ChatFlowEnabled    YesNoBool `yaml:"chatflow_enabled"`
	AudioBridgeEnabled YesNoBool `yaml:"audiobridge_enabled"`
	ServerEnabled      YesNoBool `yaml:"server_enabled"`
	StreamingEnabled   YesNoBool `yaml:"streaming_enabled"`
	AudioSourceEnabled YesNoBool `yaml:"audio_source_enabled"`
}

type ServerConfig struct {
	BaseURL        string   `yaml:"base_url"`
	Port           string   `yaml:"port"`
	TestPort       string   `yaml:"test_port"`
	PathPrefix     string   `yaml:"path_prefix"`
	WebsocketPath  string   `yaml:"websocket_path"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	OverlayVolume  int      `yaml:"overlay_volume"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type TwitchConfig struct {
	ClientID      string           `yaml:"client_id"`
	ClientSecret  string           `yaml:"client_secret"`
	WebhookSecret string           `yaml:"webhook_secret"`
	ChannelName   string           `yaml:"channel_name"`
	Chat          TwitchChatConfig `yaml:"chat"`
}

type TwitchChatConfig struct {
	BotUsername     string `yaml:"bot_username"`
	BotToken        string `yaml:"bot_token"`
	ChannelToJoin   string `yaml:"channel_to_join"`
	CommandCooldown int    `yaml:"command_cooldown"`
}

type YouTubeConfig struct {
	APIKey          string               `yaml:"api_key"`
	ChannelID       string               `yaml:"channel_id"`
	PollingInterval int                  `yaml:"polling_interval"`
	Monitor         YouTubeMonitorConfig `yaml:"monitor"`
}

type YouTubeMonitorConfig struct {
	ChannelIDs []string `yaml:"channel_ids"`
}

type OverlayConfig struct {
	Enable YesNoBool                `yaml:"enable"`
	Emotes OverlayEmotesConfig `yaml:"emotes"`
	Alerts OverlayTargetConfig `yaml:"alerts"`
	Chat   OverlayTargetConfig `yaml:"chat"`
}

type OverlayEmotesConfig struct {
	HTML YesNoBool `yaml:"html"`
}

type OverlayTargetConfig struct {
	HTML      YesNoBool `yaml:"html"`
	Discord   YesNoBool `yaml:"discord"`
	Streaming YesNoBool `yaml:"streaming"`
	Volume    int  `yaml:"volume"`
}

type DiscordConfig struct {
	Token         string   `yaml:"token"`
	Prefix        string   `yaml:"prefix"`
	Admins        []string `yaml:"admins"`
	GuildID       string   `yaml:"guild_id"`
	Streaming     YesNoBool     `yaml:"streaming"`
	ExcludedUsers []string `yaml:"excluded_users"`
}

type StreamingConfig struct {
	Enable         YesNoBool `yaml:"enable"`
	DestinationURL string    `yaml:"destination_url"`
	Bitrate        string    `yaml:"bitrate"`
	Volume         int       `yaml:"volume"`
}

type AudioSourceConfig struct {
	Enable    YesNoBool   `yaml:"enable"`
	Discord   YesNoBool   `yaml:"discord"`
	Streaming YesNoBool   `yaml:"streaming"`
	Volume    int    `yaml:"volume"`
	URL       string `yaml:"url"`
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
