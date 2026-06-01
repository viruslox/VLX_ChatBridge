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

## Usage

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