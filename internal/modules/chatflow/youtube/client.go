package youtube

import "log"

type YouTubeClient struct {
}

func NewClient() *YouTubeClient {
    return &YouTubeClient{}
}

func (c *YouTubeClient) Connect() error {
    log.Println("[ChatFlow] YouTube client connecting...")
    return nil
}

func (c *YouTubeClient) Disconnect() error {
    log.Println("[ChatFlow] YouTube client disconnecting...")
    return nil
}
