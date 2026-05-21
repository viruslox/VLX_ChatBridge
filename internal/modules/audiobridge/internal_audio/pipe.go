package internal_audio

import (
	"log"

	"VLX_ChatBridge/internal/core/audio"
)

type Pipe struct {
	stopChan chan struct{}
	srtChan chan audio.StreamData
	discordChan chan audio.StreamData
}

func NewPipe(srtChan chan audio.StreamData, discordChan chan audio.StreamData) *Pipe {
	return &Pipe{
		stopChan: make(chan struct{}),
		srtChan: srtChan,
		discordChan: discordChan,
	}
}

func (p *Pipe) Start() error {
	log.Println("[AudioBridge] Internal audio pipe starting...")
	go p.run()
	return nil
}

func (p *Pipe) run() {
	for {
		select {
		case data := <-audio.PCMChannel:
			if data.RouteSRT {
				select {
				case p.srtChan <- data:
				default:
				}
			}
			if data.RouteDiscord {
				select {
				case p.discordChan <- data:
				default:
				}
			}
		case <-p.stopChan:
			return
		}
	}
}

func (p *Pipe) Stop() error {
	log.Println("[AudioBridge] Internal audio pipe stopping...")
	close(p.stopChan)
	return nil
}
