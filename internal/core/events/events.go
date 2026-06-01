package events

// ControlBroadcastChan is used to broadcast event payloads (like alerts or commands)
// globally, allowing other modules (like the connector) to listen to them.
var ControlBroadcastChan = make(chan []byte, 1024)
