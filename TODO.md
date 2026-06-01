# TODO: Implement `vlx_connector` (IPC Output Module)

## Overview
Implement a new internal package `vlx_connector` to handle high-performance, local Inter-Process Communication (IPC) with `VLX_VisionBridge`. This replaces the local SRT network overhead and HTTP/WebSocket overhead when both services run on the same Linux machine. 

## 1. Implement Audio IPC Out (Raw PCM via UDS)
**Goal:** Pipe the internal normalized audio directly to a Unix Domain Socket.
- [ ] Create a new configuration flag in the database/settings to enable `ipc_audio_out` (disabling SRT local output).
- [ ] Initialize a Unix Domain Socket writer pointing to `/tmp/vlx_audio.sock`.
- [ ] In the internal audio mixer loop, tee the raw PCM byte stream (`s16le`, 48kHz, 2 channels).
- [ ] Write the PCM buffer directly to `/tmp/vlx_audio.sock`.
- [ ] **Error Handling:** Implement a non-blocking write or small buffer. If `VLX_VisionBridge` is not reading the socket, drop the packets to prevent memory leaks or blocking the ChatBridge audio processing loop.

## 2. Implement Command IPC Out (Control Events via UDS/ZMQ)
**Goal:** Send discrete control events (e.g., alerts, layout changes, input toggles) as JSON payloads to VisionBridge.
- [ ] Create a new configuration flag to enable `ipc_control_out`.
- [ ] Initialize a connection to `/tmp/vlx_control.sock` (using standard Go `net.Dial("unix", ...)` or ZeroMQ REQ/DEALER socket, depending on the existing VisionBridge stack).
- [ ] Define a strict JSON struct for the payload:
  ```json
  {
    "event_id": "uuid",
    "timestamp": 1717254786,
    "action": "set_input_state",
    "target": "video_source_2",
    "payload": {
      "enabled": false,
      "text": "Optional text for drawtext"
    }
  }
- [ ] Hook into the Chat/Event routing logic: when an event occurs (e.g., a specific Twitch command or an alert), marshal the event into the JSON struct and send it over the control socket.
- [ ] Ensure the socket connection automatically reconnects if the pipe is broken (e.g., if VisionBridge restarts).

3. Cleanup & Documentation
- [ ] Ensure proper graceful shutdown for both sockets in the main application context.
- [ ] Document the exact paths (/tmp/vlx_audio.sock, /tmp/vlx_control.sock) and permissions required.
