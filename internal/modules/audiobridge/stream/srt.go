package stream

import "log"

type SRTManager struct {
}

func NewSRTManager() *SRTManager {
    return &SRTManager{}
}

func (s *SRTManager) Start() error {
    log.Println("[AudioBridge] SRT manager starting...")
    return nil
}

func (s *SRTManager) Stop() error {
    log.Println("[AudioBridge] SRT manager stopping...")
    return nil
}
