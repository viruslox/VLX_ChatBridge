package stream

import "log"

type Mixer struct {
}

func NewMixer() *Mixer {
    return &Mixer{}
}

func (m *Mixer) Start() error {
    log.Println("[AudioBridge] Mixer starting...")
    return nil
}

func (m *Mixer) Stop() error {
    log.Println("[AudioBridge] Mixer stopping...")
    return nil
}
