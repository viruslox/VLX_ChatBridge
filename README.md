# VLX ChatBridge

VLX ChatBridge is a unified, self-hosted backend server designed for Linux environments that seamlessly integrates streaming platform events (Twitch, YouTube) with a bi-directional Discord audio gateway and overlay management system.

This project is a merge of **VLX_ChatFlow** (OBS Alert Overlay System) and **VLX_AudioBridge** (Discord-to-SRT Audio Gateway).

**NOTE:** The idea, logic, architecture, reviews, and validation were created by VirusLox, with code generation assisted by AI.

## Unified Architecture

ChatBridge operates as a single binary with six primary, independently configurable, hot-swappable modules sharing a unified core:

1.  **ChatFlow Module (Event Management & Overlays):**
    *   Ingests events via Twitch EventSub Webhooks and YouTube API Polling.
    *   Manages visual alerts, media commands via chat, and emote wall physics.
    *   Depends on Twitch, YouTube, and Overlay configurations.
2.  **AudioBridge Module (Audio Routing & Discord):**
    *   Connects to Discord voice channels.
    *   Captures incoming Discord audio, mixes it with internal ChatFlow audio, and pipes the resulting PCM stream.
3.  **Server Module (HTTP/WebSocket Webserver):**
    *   Serves low-latency WebSocket connections to OBS Browser Sources.
    *   Hosts the frontend HTML/JS overlays.
4.  **Streaming Module (SRT Egress):**
    *   Manages the egress of mixed audio to an SRT destination via FFmpeg.
5.  **AudioSource Module (Audio Feed Ingest):**
    *   Ingests external audio feeds via FFmpeg and pipes them directly into the internal audio mixer.
6.  **Connector Module (IPC Output):**
    *   High-performance local Inter-Process Communication (IPC) via Unix Domain Sockets for streaming raw PCM audio and JSON control events to `VLX_VisionBridge`.

### Hot-Swappable Modules
All six modules can be enabled or disabled on-the-fly via configuration (`modules` block), allowing the server to act solely as an alert system, an audio bridge, an SRT streamer, an IPC connector, or a combination of them simultaneously.

---

## Features

### ChatFlow Features
*   **Twitch Integration:** EventSub Webhooks (Follows, Subs, Raids) and IRC Bot with Role-Based Access Control (!commands).
*   **YouTube Integration:** Live polling for Super Chats, Stickers, and Memberships.
*   **Overlays:** Alerts overlay, Chat Media overlay, and Emote Wall.
*   **Smart Rate Limiting & Persistence:** Token buckets for API quotas and SQLite for state/token management.
*   **Dynamic Command Generation:** File-based chat commands generation by dropping files in corresponding permission folders (`everyone`, `subscribers`, `vips`). Use the `owner_` prefix to enforce strict broadcaster-only access control.
*   **Dynamic File-Based Routing (ZMQ & Webhooks):** As part of **"The Holy Trinity"** architecture, ChatBridge acts as the central nervous system. It receives chat commands and routes them instantly via IPC/ZMQ to `VLX_VisionBridge` (for zero-latency video mixing) and via HTTP Webhooks to `VLX_FrameFlow` (for IRL backpack control).

### Real-Time Telemetry (GPS)
ChatBridge natively receives GPS and Speed data directly from the FrameFlow backpack. It acts as a real-time telemetry receiver, ingesting JSON payload data and immediately broadcasting it to the frontend.
*   **Zero Latency:** The overlay runs at 60fps via WebSockets with zero latency, entirely eliminating disk I/O, database token checks, and HTTP polling.
*   **Setup Instructions:** To set this up in VisionBridge, add a Chromium web layer pointing directly to `http://127.0.0.1:<PORT>/gps_overlay.html`.

### AudioBridge Features
*   **Discord Ingress/Egress:** Joins voice channels, captures Opus packets (libdave/godave support), and injects internal audio.
*   **Soft-Clipping Mixer:** Real-time PCM mixing with volume normalization and clipping protection.
*   **SRT Streaming:** High-quality, low-latency audio transmission via FFmpeg.
*   **Direct Audio Injection:** Seamlessly routes `.mp3`/`.wav` media played via chat commands directly to Discord and the SRT stream.

---

## Project Structure

```bash
VLX_ChatBridge/
├── cmd/
│   └── chatbridge/
│       └── main.go              # Entry point. Initializes core and starts modules.
├── config.yml                   # Unified configuration file
├── internal/
│   ├── core/                    # Shared components (config, logger, db, audio, module manager)
│   └── modules/
│       ├── chatflow/            # Logic for Twitch, YouTube, WebSockets, Overlays
│       ├── audiobridge/         # Logic for Discord bot
│       ├── server/              # Logic for HTTP webserver and reverse proxy mapping
│       ├── streaming/           # Logic for SRT output mixing via FFmpeg
│       ├── audiosource/         # Logic for external audio ingest
│       └── connector/           # Logic for local IPC with VisionBridge
├── static/                      # Frontend folder (HTML/JS/CSS/Assets for OBS)
│   └── chat/                    # Audio/Video assets storage for commands
└── scripts/                     # Systemd service files
```

---

## System Requirements
*   **OS:** Linux (Tested on Debian)
*   **Dependencies:** Go 1.21+, SQLite, FFmpeg, libopus-dev, libopusfile-dev, pkg-config, cmake, clang, build-essential
*   *Note: PortAudio and Chromium dependencies previously required by AudioBridge have been removed in favor of direct internal audio decoding.*

## Installation & Build

### 1. Install libdave (Required for Discord Voice E2EE)
```bash
mkdir -p ~/Projects/ ; cd ~/Projects/
git clone https://github.com/disgoorg/godave.git
cd godave/scripts ; export CC=/usr/bin/clang CXX=/usr/bin/clang
export PKG_CONFIG_PATH="$HOME/.local/lib/pkgconfig:$PKG_CONFIG_PATH"
./libdave_install.sh v1.1.0
```

### 2. Clone and Build
```bash
git clone https://github.com/viruslox/VLX_ChatBridge
cd VLX_ChatBridge
go mod init
go mod tidy
./build.sh
```

### 3. Deploy
```bash
sudo ./VLX_ChatBridge install
```

---

## Configuration

Edit `/opt/VLX_ChatBridge/config.yml` to configure the system. You can use environment variables (e.g., `${ENV_VAR}`).

```yaml
modules:
  chatflow_enabled: yes
  audiobridge_enabled: yes
  server_enabled: yes
  streaming_enabled: yes
  audio_source_enabled: no
  connector_enabled: no

server:
  base_url: "https://your.ngrok.io"
  path_prefix: "/asortofkey"
  websocket_path: "/websocket"
  port: "8000"
  test_port: "8001"
  overlay_volume: 70

database:
  path: "/opt/VLX_ChatBridge/var/chatbridge.db"

twitch:
  # ... Twitch App IDs, Secrets, Tokens ...

youtube:
  # ... YouTube API Key, Channel ID ...

overlay:
  enable: yes
  emotes:
    html: yes
  alerts:
    html: yes
    discord: yes
    streaming: no
    volume: 75
  chat:
    html: yes
    discord: yes
    streaming: yes
    volume: 75

discord:
  token: "YOUR_DISCORD_BOT_TOKEN"
  prefix: "vlx."
  streaming: yes

streaming:
  enable: yes
  destination_url: "srt://127.0.0.1:8890?streamid=publish:vlx_audio&mode=caller&pkt_size=1316"
  bitrate: "128k"
  volume: 75

audio_source:
  enable: no
  discord: yes
  streaming: no
  volume: 80
  url: "srt://127.0.0.1:2020?..."

connector:
  ipc_audio_out: yes
  ipc_control_out: yes
  audio_socket: "/tmp/vlx_audio.sock"
  control_socket: "/tmp/vlx_control.sock"
```

### Reverse Proxy Configuration

The server supports a `path_prefix` configuration token that a reverse proxy can strip to enhance security.
Below is an example of an Apache reverse proxy configuration that properly routes requests, including upgrading WebSocket connections.

```apache
RewriteCond %{HTTP:Upgrade} websocket [NC]
RewriteCond %{HTTP:Connection} upgrade [NC]
RewriteRule "^/<path_prefix>/wwf$" "ws://localhost:8000/wwf" [P,L]
ProxyPass /<path_prefix>/ http://localhost:8000/
ProxyPassReverse /<path_prefix>/ http://localhost:8000/
```

---

### Twitch Webhook Configuration

Twitch EventSub requires a publicly accessible HTTPS endpoint to deliver events.
base_url: Must be your root public domain (e.g., https://yourdomain.com). Do not include any path prefixes or obfuscation tokens here, as Twitch will append /webhooks/twitch automatically.
Ingestion: ChatBridge listens for notifications at the /webhooks/twitch path. Ensure your reverse proxy (e.g., Apache/Nginx) is configured to forward requests from https://yourdomain.com/webhooks/twitch to the internal port defined in server.port.
Security: Ensure twitch.webhook_secret matches the secret provided in your Twitch Developer Console, as this is used to validate the HMAC signature of every incoming event.

## Usage

### Dynamic File-Based Routing

ChatBridge parses text files dropped into `static/chat/` to generate commands on the fly. By adding special blocks to these files, you can trigger routing to VisionBridge or FrameFlow.

**1. ZMQ Control Example (VisionBridge)**
Create a file at `static/chat/owner_cam1.txt` to trigger a scene change in VLX_VisionBridge via local IPC. The `owner_` prefix ensures only the broadcaster can run `!cam1`.
```ini
[ZMQ_CONTROL]
Target=stream
Enabled=true
```

**2. Webhook Control Example (FrameFlow)**
Create a file at `static/chat/owner_bitrate.txt` to send an HTTP POST request to VLX_FrameFlow running on the local backpack.
```ini
[WEBHOOK]
Method=POST
URL=http://127.0.0.1:8080/api/frameflow/bitrate
Body={"action": "increase"}
```

#### Stealth Mode (AutoDelete)
Both `[ZMQ_CONTROL]` and `[WEBHOOK]` files support an `AutoDelete=true` flag. This feature allows you to silently execute commands without cluttering the public chat. When enabled, it leverages dynamically refreshed DB tokens and the Twitch Helix API to instantly delete the invoking chat message.

Example with Stealth Mode:
```ini
[ZMQ_CONTROL]
Target=stream
Enabled=true
AutoDelete=true
```

### Running Manually
```bash
/opt/VLX_ChatBridge/VLX_ChatBridge -config /opt/VLX_ChatBridge/config.yml
```

### Running as a Service (Systemd)
```bash
mkdir -p ~/.config/systemd/user/
cp scripts/vlx_chatbridge.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable vlx_chatbridge.service
systemctl --user start vlx_chatbridge.service
```

### OBS Integration
Add Browser Sources in OBS pointing to the local server (e.g., `http://localhost:8000/static/alerts_overlay.html`).

### Discord Commands
*   `vlx.join` : Bot joins your voice channel and starts the SRT stream.
*   `vlx.leave`: Bot stops streaming and disconnects.

## License
This project is licensed under the GNU General Public License v3.0. See the LICENSE file for details.
