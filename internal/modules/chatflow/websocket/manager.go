package websocket

import "log"

type WebSocketManager struct {
}

func NewManager() *WebSocketManager {
    return &WebSocketManager{}
}

func (m *WebSocketManager) Start() error {
    log.Println("[ChatFlow] WebSocket manager starting...")
    return nil
}

func (m *WebSocketManager) Stop() error {
    log.Println("[ChatFlow] WebSocket manager stopping...")
    return nil
}
