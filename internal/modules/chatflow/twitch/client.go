package twitch

import (
	"log"
	"VLX_ChatBridge/internal/core/config"
	irc "github.com/gempir/go-twitch-irc/v4"
)

type TwitchClient struct {
	config config.TwitchConfig
	client *irc.Client
}

func NewClient(cfg config.TwitchConfig) *TwitchClient {
	return &TwitchClient{
		config: cfg,
	}
}

func (c *TwitchClient) Connect() error {
	log.Println("[ChatFlow] Twitch client connecting...")

	if c.config.Chat.BotUsername != "" && c.config.Chat.BotToken != "" {
		c.client = irc.NewClient(c.config.Chat.BotUsername, c.config.Chat.BotToken)
	} else {
		// Anonymous connection
		c.client = irc.NewAnonymousClient()
	}

	c.client.OnPrivateMessage(func(message irc.PrivateMessage) {
		log.Printf("[ChatFlow] [Twitch] %s: %s\n", message.User.DisplayName, message.Message)
	})

	c.client.OnConnect(func() {
		log.Println("[ChatFlow] Twitch client successfully connected")
	})

	if c.config.Chat.ChannelToJoin != "" {
		c.client.Join(c.config.Chat.ChannelToJoin)
		log.Printf("[ChatFlow] Joining Twitch channel: %s", c.config.Chat.ChannelToJoin)
	} else if c.config.ChannelName != "" {
		c.client.Join(c.config.ChannelName)
		log.Printf("[ChatFlow] Joining Twitch channel: %s", c.config.ChannelName)
	}

	go func() {
		err := c.client.Connect()
		if err != nil {
			log.Printf("[ChatFlow] Twitch client connection error: %v", err)
		}
	}()

	return nil
}

func (c *TwitchClient) Disconnect() error {
	log.Println("[ChatFlow] Twitch client disconnecting...")
	if c.client != nil {
		return c.client.Disconnect()
	}
	return nil
}
