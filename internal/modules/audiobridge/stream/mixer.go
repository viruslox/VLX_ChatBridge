package stream

import (
	"log"
	"sync"

	"VLX_ChatBridge/internal/core/audio"
)

type Mixer struct {
	stopChan chan struct{}
	stopOnce sync.Once
}

func NewMixer() *Mixer {
	return &Mixer{
		stopChan: make(chan struct{}),
	}
}

func (m *Mixer) Start() error {
	log.Println("[AudioBridge] Mixer starting...")

	go func() {
		for {
			select {
			case chunk := <-audio.PCMChannel:
				// In a real implementation, this would mix the audio chunks
				log.Printf("[AudioBridge] Mixer received %d bytes of PCM data", len(chunk))
			case <-m.stopChan:
				log.Println("[AudioBridge] Mixer stopped reading PCM data.")
				return
			}
		}
	}()

	return nil
}

func (m *Mixer) Stop() error {
	log.Println("[AudioBridge] Mixer stopping...")
	m.stopOnce.Do(func() {
		close(m.stopChan)
	})
	return nil
}
