package events

// ControlBroadcastChan is used to broadcast event payloads (like alerts or commands)
// globally, allowing other modules (like the connector) to listen to them.
var ControlBroadcastChan = make(chan []byte, 1024)

// WebSocketBroadcastChan is used by external modules (like the Server) to
// send broadcast payloads directly to all connected WebSocket clients.
var WebSocketBroadcastChan = make(chan []byte, 1024)
