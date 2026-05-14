package twitch

import "log"

type TwitchClient struct {
}

func NewClient() *TwitchClient {
    return &TwitchClient{}
}

func (c *TwitchClient) Connect() error {
    log.Println("[ChatFlow] Twitch client connecting...")
    return nil
}

func (c *TwitchClient) Disconnect() error {
    log.Println("[ChatFlow] Twitch client disconnecting...")
    return nil
}
