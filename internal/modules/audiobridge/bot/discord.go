package bot

import (
	"errors"
	"log"

	"github.com/bwmarrin/discordgo"
)

type DiscordBot struct {
	token   string
	session *discordgo.Session
}

func NewBot(token string) *DiscordBot {
	return &DiscordBot{
		token: token,
	}
}

func (b *DiscordBot) Connect() error {
	log.Println("[AudioBridge] Discord bot connecting...")

	if b.token == "" || b.token == "YOUR_DISCORD_BOT_TOKEN" {
		err := errors.New("invalid or empty discord token")
		log.Printf("[AudioBridge] Discord connection failed: %v", err)
		return err
	}

	session, err := discordgo.New("Bot " + b.token)
	if err != nil {
		log.Printf("[AudioBridge] Failed to create Discord session: %v", err)
		return err
	}

	// Set intents to receive necessary events (including privileged ones)
	session.Identify.Intents = discordgo.IntentsAll

	// Register event handlers
	session.AddHandler(b.onReady)
	session.AddHandler(b.onGuildCreate)
	session.AddHandler(b.onMessageCreate)

	b.session = session

	log.Println("[AudioBridge] Opening Discord connection...")
	if err := b.session.Open(); err != nil {
		log.Printf("[AudioBridge] Failed to open Discord connection: %v", err)
		return err
	}

	log.Println("[AudioBridge] Discord bot connected successfully.")
	return nil
}

func (b *DiscordBot) Disconnect() error {
	log.Println("[AudioBridge] Discord bot disconnecting...")
	if b.session != nil {
		if err := b.session.Close(); err != nil {
			log.Printf("[AudioBridge] Error closing Discord connection: %v", err)
			return err
		}
		log.Println("[AudioBridge] Discord bot disconnected successfully.")
	}
	return nil
}

func (b *DiscordBot) onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("[AudioBridge] Discord bot ready! Logged in as: %s#%s", event.User.Username, event.User.Discriminator)
}

func (b *DiscordBot) onGuildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	log.Printf("[AudioBridge] Connected to guild: %s (ID: %s)", event.Guild.Name, event.Guild.ID)
	log.Printf("[AudioBridge] Guild Owner ID: %s", event.Guild.OwnerID)
	log.Printf("[AudioBridge] Guild Channels count: %d", len(event.Guild.Channels))
	for _, c := range event.Guild.Channels {
		log.Printf("[AudioBridge] Channel: %s (ID: %s, Type: %d)", c.Name, c.ID, c.Type)
	}
}

func (b *DiscordBot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	log.Printf("[AudioBridge] Message received in channel %s from %s: %s", m.ChannelID, m.Author.Username, m.Content)
}
