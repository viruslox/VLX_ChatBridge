# VLX ChatBridge API & Command Reference

> **Part of the VLX Stream Flow ecosystem — Control & Engagement tier.**

VLX ChatBridge translates platform events (Twitch, YouTube chat/events, Discord slash commands) into real-time actions. It allows you to dynamically build commands by placing `.json` files in `static/chat/`. This document explains how to construct these commands and interact with the ecosystem APIs.

---

## 1. File-based Command System

To add a command, drop a `.json` file inside `static/chat/`. The base name of the file becomes the command name (e.g., `camera1.json` becomes `!camera1`).

### Permissions by Prefix
- `owner_<name>.json` — Broadcaster-only command (executable via chat or Discord `/run`).
- `sub_<name>.json` — Subscriber-only command.
- `vip_<name>.json` — VIP-only command.
- `<name>.json` (no prefix) — Available to everyone.

> **Discord Execution:** All `owner_` prefixed commands are natively executable by the server owner via the `/run <command>` slash command in Discord.

### Command Structure (`multi_action`)
A standard API command uses `multi_action` structure to issue one or more actions sequentially or in parallel.

```json
{
  "description": "Switch to Camera 1 via VisionBridge and Start FrameFlow transmission",
  "auto_delete": true,
  "actions": [
    { ... action 1 ... },
    { ... action 2 ... }
  ]
}
```

- `description`: (Optional) Appears in the `!commands` and `/commands` lists.
- `auto_delete`: (Optional) If `true`, ChatBridge will automatically delete the user's triggering chat message using the Twitch API (stealth execution).
- `actions`: An array of action objects. The key field in an action is `"transport"`, which can be `"ipc"` (VisionBridge) or `"webhook"` (FrameFlow/External APIs).

---

## 2. Controlling VisionBridge (IPC Transport)

ChatBridge controls the video compositor (VisionBridge) over a fast, zero-latency Unix socket. Actions using `"transport": "ipc"` are sent directly to VisionBridge.

### Switching Cameras / Scenes
To change the active layout (e.g., switching from Camera 1 to Camera 2), use the `apply_template` action. This instructs VisionBridge to hot-reload the specified Z-order layout file (which must exist in VisionBridge's configuration folder).

```json
{
  "transport": "ipc",
  "action": "apply_template",
  "payload": {
    "text": "camera1_layout.yaml"
  }
}
```

### Enabling / Disabling the Stream Output
To completely kill or start the broadcast out of VisionBridge (terminates/starts FFmpeg).

```json
{
  "transport": "ipc",
  "action": "set_input_state",
  "target": "stream",
  "payload": {
    "enabled": true
  }
}
```

### Toggling Overlays
You can enable or disable a specific Z-layer overlay (where `layer2` corresponds to `z2` in VisionBridge's configuration). Optionally set a new path/URL.

```json
{
  "transport": "ipc",
  "action": "set_input_state",
  "target": "overlay@layer2",
  "payload": {
    "enabled": true,
    "text": "http://127.0.0.1:8000/gps_overlay.html"
  }
}
```

### Changing Volume
Adjust the volume of a specific layer in real time (0-100).

```json
{
  "transport": "ipc",
  "action": "set_input_state",
  "target": "volume@layer3",
  "payload": {
    "text": "75"
  }
}
```

---

## 3. Controlling FrameFlow (Webhook Transport)

To control the remote field unit (SBC backpacks) running VLX FrameFlow, you use HTTP Webhooks. FrameFlow runs a relay server on the VPS (default `127.0.0.1:9090`) that forwards requests safely through the MLVPN tunnel to the SBC.

Use `"transport": "webhook"`.

### Starting the Field Camera
```json
{
  "transport": "webhook",
  "method": "POST",
  "url": "http://127.0.0.1:9090/api/v1/relay/cameraman/start",
  "payload": {
    "device": "V0A1"
  }
}
```

### General Webhook Action Format
```json
{
  "transport": "webhook",
  "method": "POST",
  "url": "http://127.0.0.1:9090/api/v1/relay/<module>/<action>",
  "headers": {
    "Optional-Header": "Value"
  },
  "payload": {
    "key": "value"
  }
}
```

*(For a full list of FrameFlow endpoints, refer to the FrameFlow API documentation).*

---

## 4. Media Commands (Audio / Video)

If you just want an alert (sound or video) to play on stream and in Discord when someone types a command, you don't need `multi_action`. Just place the media file (`.mp3`, `.wav`, `.mp4`, `.webm`) in `static/chat/`. 

To configure permissions, prefix it: `owner_fart.mp3`. When the owner types `!fart` (or runs `/run fart` in Discord), the media will play. No `.json` file is required for simple media alerts.
