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
