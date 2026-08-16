package bot

import (
	"context"
	"errors"
	"log"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/handler"
	"go.uber.org/zap"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/godave/golibdave"

	"VLX_ChatBridge/internal/core/module"
)

type DiscordBot struct {
	token                   string
	admins                  []string
	client                  *bot.Client
	controller              module.Controller
	discordStreamingEnabled bool
	excludedUsers           []string
	discordOutChan          <-chan []byte
	guildID                 string
	chatBridgeDIR           string
	zapLogger               *zap.Logger
	slashRouter             *handler.Mux
}

func NewBot(token string, admins []string, discordStreamingEnabled bool, excludedUsers []string, guildID string, chatBridgeDIR string, logger *zap.Logger, ctrl module.Controller, discordOutChan <-chan []byte) *DiscordBot {
	return &DiscordBot{
		token:                   token,
		admins:                  admins,
		controller:              ctrl,
		discordStreamingEnabled: discordStreamingEnabled,
		excludedUsers:           excludedUsers,
		discordOutChan:          discordOutChan,
		guildID:                 guildID,
		chatBridgeDIR:           chatBridgeDIR,
		zapLogger:               logger,
	}
}

func (b *DiscordBot) Connect() error {
	log.Println("[AudioBridge] Discord bot connecting...")

	if b.token == "" || b.token == "YOUR_DISCORD_BOT_TOKEN" {
		err := errors.New("invalid or empty discord token")
		log.Printf("[AudioBridge] Discord connection failed: %v", err)
		return err
	}

	b.slashRouter = b.registerSlashHandlers()

	client, err := disgo.New(b.token,
		bot.WithGatewayConfigOpts(gateway.WithIntents(gateway.IntentsAll)),
		bot.WithCacheConfigOpts(cache.WithCaches(cache.FlagsAll)),
		bot.WithEventListenerFunc(b.onReady),
		bot.WithEventListeners(b.slashRouter),
		bot.WithVoiceManagerConfigOpts(voice.WithDaveSessionCreateFunc(golibdave.NewSession)),
	)
	if err != nil {
		log.Printf("[AudioBridge] Failed to create Discord session: %v", err)
		return err
	}

	b.client = client

	log.Println("[AudioBridge] Opening Discord connection...")
	if err := b.client.OpenGateway(context.TODO()); err != nil {
		log.Printf("[AudioBridge] Failed to open Discord connection: %v", err)
		return err
	}

	if err := b.syncSlashCommands(); err != nil {
		log.Printf("[AudioBridge] Slash command registration skipped/failed: %v", err)
	} else {
		log.Println("[AudioBridge] Slash commands registered for guild.")
	}

	log.Println("[AudioBridge] Discord bot connected successfully.")
	return nil
}

func (b *DiscordBot) Disconnect() error {
	log.Println("[AudioBridge] Discord bot disconnecting...")
	if b.client != nil {
		b.client.Close(context.TODO())
		log.Println("[AudioBridge] Discord bot disconnected successfully.")
	}
	return nil
}

func (b *DiscordBot) onReady(event *events.Ready) {
	log.Printf("[AudioBridge] Discord bot ready! Logged in as: %s", event.User.Username)
}