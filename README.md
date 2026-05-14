# VLX ChatBridge

VLX ChatBridge is a unified, self-hosted backend server designed for Linux environments that seamlessly integrates streaming platform events (Twitch, YouTube) with a bi-directional Discord audio gateway and overlay management system.

This project is a merge of **VLX_ChatFlow** (OBS Alert Overlay System) and **VLX_AudioBridge** (Discord-to-SRT Audio Gateway).

**NOTE:** The idea, logic, architecture, reviews, and validation were created by VirusLox, with code generation assisted by AI.

## Unified Architecture

ChatBridge operates as a single binary with two primary, hot-swappable modules sharing a unified core:

1.  **ChatFlow Module (Event Management & Overlays):**
    *   Ingests events via Twitch EventSub Webhooks and YouTube API Polling.
    *   Manages visual alerts, media commands via chat, and emote wall physics.
    *   Serves low-latency WebSocket connections to OBS Browser Sources.
2.  **AudioBridge Module (Audio Routing & Discord):**
    *   Connects to Discord voice channels.
    *   **Direct Audio Integration:** Audio triggered by ChatFlow alerts and commands is decoded internally and piped directly into the audio mixer, eliminating the need for headless browser audio capture.
    *   **Mixer & Egress:** Captures incoming Discord audio, mixes it with internal ChatFlow audio, and pipes the resulting PCM stream to FFmpeg for SRT transmission.

### Hot-Swappable Modules
Both the ChatFlow and AudioBridge modules can be enabled or disabled on-the-fly via configuration or runtime commands, allowing the server to act solely as an alert system, solely as an audio bridge, or both simultaneously.

---

## Features

### ChatFlow Features
*   **Twitch Integration:** EventSub Webhooks (Follows, Subs, Raids) and IRC Bot with Role-Based Access Control (!commands).
*   **YouTube Integration:** Live polling for Super Chats, Stickers, and Memberships.
*   **Overlays:** Alerts overlay, Chat Media overlay, and Emote Wall.
*   **Smart Rate Limiting & Persistence:** Token buckets for API quotas and PostgreSQL for state/token management.

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
│   ├── core/                    # Shared components (config, logger, db)
│   └── modules/
│       ├── chatflow/            # Logic for Twitch, YouTube, WebSockets, Overlays
│       └── audiobridge/         # Logic for Discord bot, Mixer, SRT FFmpeg wrapper
├── static/                      # Frontend folder (HTML/JS/CSS/Assets for OBS)
│   └── chat/                    # Audio/Video assets storage for commands
└── scripts/                     # Systemd service files
```

---

## System Requirements
*   **OS:** Linux (Tested on Debian)
*   **Dependencies:** Go 1.21+, PostgreSQL, FFmpeg
*   *Note: PortAudio and Chromium dependencies previously required by AudioBridge have been removed in favor of direct internal audio decoding.*

## Installation & Build

### 1. Install libdave (Required for Discord Voice E2EE)
```bash
mkdir -p ~/Projects/ ; cd ~/Projects/
git clone https://github.com/disgoorg/godave.git
cd godave/scripts ; export CC=/usr/bin/clang CXX=/usr/bin/clang
./libdave_install.sh v1.1.0
```

### 2. Clone and Build
```bash
git clone https://github.com/viruslox/VLX_ChatBridge
cd VLX_ChatBridge
go mod init
go mod tidy
go build -o VLX_ChatBridge ./cmd/chatbridge
```

### 3. Deploy
```bash
sudo mkdir -p /opt/VLX_ChatBridge
sudo chown -R $USER:$USER /opt/VLX_ChatBridge
cp VLX_ChatBridge /opt/VLX_ChatBridge/
cp config.yml /opt/VLX_ChatBridge/
```

---

## Configuration

Edit `/opt/VLX_ChatBridge/config.yml` to configure the system. You can use environment variables (e.g., `${ENV_VAR}`).

```yaml
modules:
  chatflow_enabled: true
  audiobridge_enabled: true

server:
  base_url: "https://your.ngrok.io"
  port: "8000"
  test_port: "8001"

database:
  host: "localhost"
  user: "postgres"
  password: "${DB_PASSWORD}"
  dbname: "chatbridge_db"

twitch:
  # ... Twitch App IDs, Secrets, Tokens ...

youtube:
  # ... YouTube API Key, Channel ID ...

discord:
  token: "YOUR_DISCORD_BOT_TOKEN"
  prefix: "vlx."
  admins: ["111111111111111111"]

streaming:
  destination_url: "srt://127.0.0.1:8890?streamid=publish:vlx_audio&mode=caller&pkt_size=1316"
  bitrate: "128k"

# Note: audio_source configuration now only accepts SRT inputs. HTML inputs are managed internally.
audio_source:
  source1:
    enable: yes
    type: SRT
    volume: 80
    url: "srt://127.0.0.1:2020?..."
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