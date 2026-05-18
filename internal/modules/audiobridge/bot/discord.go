package bot

import (
	"context"
	"errors"
	"log"
	"strings"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/godave/golibdave"
	"github.com/disgoorg/snowflake/v2"

	"VLX_ChatBridge/internal/core/module"
)

type DiscordBot struct {
	token           string
	prefix          string
	admins          []string
	client          *bot.Client
	controller      module.Controller
	pendingShutdown map[snowflake.ID]snowflake.ID // channelID -> authorID
}

func NewBot(token string, prefix string, admins []string, ctrl module.Controller) *DiscordBot {
	return &DiscordBot{
		token:           token,
		prefix:          prefix,
		admins:          admins,
		controller:      ctrl,
		pendingShutdown: make(map[snowflake.ID]snowflake.ID),
	}
}

func (b *DiscordBot) Connect() error {
	log.Println("[AudioBridge] Discord bot connecting...")

	if b.token == "" || b.token == "YOUR_DISCORD_BOT_TOKEN" {
		err := errors.New("invalid or empty discord token")
		log.Printf("[AudioBridge] Discord connection failed: %v", err)
		return err
	}

	client, err := disgo.New(b.token,
		bot.WithGatewayConfigOpts(gateway.WithIntents(gateway.IntentsAll)),
		bot.WithEventListenerFunc(b.onReady),
		bot.WithEventListenerFunc(b.onMessageCreate),
		bot.WithVoiceManagerConfigOpts(
			voice.WithDaveSessionCreateFunc(golibdave.NewSession),
		),
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

func (b *DiscordBot) onMessageCreate(event *events.MessageCreate) {
	if event.Message.Author.Bot {
		return
	}

	if event.GuildID == nil {
		return
	}

	guild, ok := event.Client().Caches.Guild(*event.GuildID)
	if !ok {
		return
	}

	// Only allow server owner
	if guild.OwnerID != event.Message.Author.ID {
		return
	}

	content := event.Message.Content

	log.Printf("[AudioBridge] Message received in channel %s from %s: %s", event.ChannelID, event.Message.Author.Username, content)

	// Handle pending shutdown confirmation
	if authorID, exists := b.pendingShutdown[event.ChannelID]; exists && authorID == event.Message.Author.ID {
		if strings.ToLower(content) == "yes" {
			log.Printf("[AudioBridge] Shutdown confirmed by owner.")
			event.Client().Rest.CreateMessage(event.ChannelID, discord.MessageCreate{Content: "Shutting down..."})
			syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
		} else {
			log.Printf("[AudioBridge] Shutdown declined by owner.")
			event.Client().Rest.CreateMessage(event.ChannelID, discord.MessageCreate{Content: "Shutdown declined."})
		}
		delete(b.pendingShutdown, event.ChannelID)
		return
	}

	if !strings.HasPrefix(content, b.prefix) {
		return
	}

	args := strings.Fields(content[len(b.prefix):])
	if len(args) == 0 {
		return
	}

	command := strings.ToLower(args[0])

	switch command {
	case "join":
		var channelID snowflake.ID
		if len(args) > 1 {
			parsedID, err := snowflake.Parse(args[1])
			if err != nil {
				log.Printf("[AudioBridge] Invalid channel ID provided: %v", err)
				return
			}
			channelID = parsedID
		} else {
			vs, ok := event.Client().Caches.VoiceState(*event.GuildID, event.Message.Author.ID)
			if !ok || vs.ChannelID == nil {
				log.Printf("[AudioBridge] Could not find user %s in a voice channel.", event.Message.Author.Username)
				return
			}
			channelID = *vs.ChannelID
		}

		conn := b.client.VoiceManager.CreateConn(*event.GuildID)
		err := conn.Open(context.TODO(), channelID, false, false)
		if err != nil {
			log.Printf("[AudioBridge] Failed to join voice channel %s: %v", channelID, err)
			return
		}
		log.Printf("[AudioBridge] Joined voice channel %s successfully.", channelID)

	case "leave":
		conn := b.client.VoiceManager.GetConn(*event.GuildID)
		if conn != nil {
			conn.Close(context.TODO())
			b.client.VoiceManager.RemoveConn(*event.GuildID)
			log.Printf("[AudioBridge] Left voice channel successfully.")
		} else {
			log.Printf("[AudioBridge] Not currently connected to a voice channel.")
		}

	case "reload":
		log.Printf("[AudioBridge] Reloading AudioBridge module...")
		go func() {
			if b.controller != nil {
				b.controller.StopModule("AudioBridge")
				b.controller.StartModule("AudioBridge")
			}
		}()

	case "shutdown":
		b.pendingShutdown[event.ChannelID] = event.Message.Author.ID
		log.Printf("[AudioBridge] Pending shutdown requested by %s", event.Message.Author.Username)
		event.Client().Rest.CreateMessage(event.ChannelID, discord.MessageCreate{Content: "Are you sure you want to shutdown VLX_ChatBridge? Reply 'yes' to confirm."})
	}
}
