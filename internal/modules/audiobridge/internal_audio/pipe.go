package internal_audio

import "log"

type Pipe struct {
}

func NewPipe() *Pipe {
    return &Pipe{}
}

func (p *Pipe) Start() error {
    log.Println("[AudioBridge] Internal audio pipe starting...")
    return nil
}

func (p *Pipe) Stop() error {
    log.Println("[AudioBridge] Internal audio pipe stopping...")
    return nil
}
