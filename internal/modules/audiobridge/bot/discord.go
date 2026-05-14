package bot

import "log"

type DiscordBot struct {
}

func NewBot() *DiscordBot {
    return &DiscordBot{}
}

func (b *DiscordBot) Connect() error {
    log.Println("[AudioBridge] Discord bot connecting...")
    return nil
}

func (b *DiscordBot) Disconnect() error {
    log.Println("[AudioBridge] Discord bot disconnecting...")
    return nil
}
