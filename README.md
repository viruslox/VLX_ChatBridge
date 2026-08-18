# VLX ChatBridge

VLX ChatBridge is a unified, self-hosted backend server designed for Linux environments that seamlessly integrates streaming platform events (Twitch, YouTube) with a bi-directional Discord audio gateway and overlay management system.

This project is a merge of **VLX_ChatFlow** (OBS Alert Overlay System) and **VLX_AudioBridge** (Discord-to-SRT Audio Gateway).

**NOTE:** The idea, logic, architecture, reviews, and validation were created by VirusLox, with code generation assisted by AI.

## Unified Architecture

ChatBridge operates as a single binary with six primary, independently configurable, hot-swappable modules sharing a unified core:

1.  **ChatFlow Module (Event Management & Overlays):**
    *   Ingests events via Twitch EventSub Webhooks and YouTube API Polling.
    *   Manages visual alerts, media commands via chat, and emote wall physics.
    *   Cross-platform Discord Go-Live Announcer utilizing Discord webhooks for live/end notifications.
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
    *   High-performance local Inter-Process Communication (IPC) via Unix Domain Sockets for streaming JSON control events to `VLX_VisionBridge`.

### Hot-Swappable Modules
All six modules can be enabled or disabled on-the-fly via configuration (`modules` block), allowing the server to act solely as an alert system, an audio bridge, an SRT streamer, an IPC connector, or a combination of them simultaneously.

---

## Features

### ChatFlow Features
*   **Twitch Integration:** EventSub Webhooks (Follows, Subs, Raids) and IRC Bot with Role-Based Access Control (!commands).
*   **First-Chatter Float:** Floats the username across the screen the first time each user chats in the current live session, colored to match their Twitch chat name.
*   **YouTube Integration:** Live polling for Super Chats, Stickers, and Memberships.
*   **Overlays:** Alerts overlay, Chat Media overlay, Emote Wall, and Scenes overlay.
*   **Smart Rate Limiting & Persistence:** Token buckets for API quotas and SQLite for state/token management.
*   **Dynamic Command Generation:** File-based chat commands generation by dropping files in corresponding permission folders (`everyone`, `subscribers`, `vips`). Use the `owner_` prefix to enforce strict broadcaster-only access control.
*   **Dynamic File-Based Routing (ZMQ & Webhooks):** As part of **VLX Stream Flow** architecture, ChatBridge acts as the central nervous system. It receives chat commands and routes them instantly via IPC/ZMQ to `VLX_VisionBridge` (for zero-latency video mixing) and via HTTP Webhooks to `VLX_FrameFlow` (for IRL backpack control).

### Advanced Audio Engine
Features direct internal audio decoding, dynamic equal-power volume balancing, envelope-based noise gating, a zero-latency feed-forward compressor, and soft-clip peak limiting for distortion-free audio mixing.

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
*   **Dependencies:** Go 1.24+, SQLite, FFmpeg, libopus-dev, libopusfile-dev, pkg-config, cmake, clang, build-essential
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
# VLX ChatBridge User Profile
chatbridge_USER: "chatbridge"
chatbridge_DIR: "/opt/VLX_ChatBridge"
database:
  path: "/opt/VLX_ChatBridge/var/chatbridge.db"

modules:
  chatflow_enabled: no
  audiobridge_enabled: no
  server_enabled: no
  streaming_enabled: no
  audio_source_enabled: no
  connector_enabled: no

# Modules dependency:
# - discord -> audiobridge
# - twitch -> chatflow
# - youtube -> chatflow
# - overlay -> chatflow

server:
  # base_url: The public root URL of your server. Example: "https://yourdomain.com"
  # NOTE: Do not include path_prefix here. Twitch will call: <base_url>/webhooks/twitch
  base_url: "https://your.ngrok.io"
  # path_prefix: Internal security token for overlays and API telemetry.
  path_prefix: "/asortofkey"
  websocket_path: "/websocket"
  allowed_origins:
    - "https://net.example.com"
    - "http://localhost:8000"
    - "http://127.0.0.1:8000"
  # This port is used for scenes/emotes overlays, and ALSO acts as the
  # ingestion endpoint for FrameFlow's telemetry (e.g., http://10.1.10.1:<PORT>/api/gps)
  port: "8000"
  test_port: "8001"

# Recommended to use 2 different accounts, or at least 2 difrerent tokens
# You can create token with https://twitchtokengenerator.com/
twitch:
  client_id: "YOUR_TWITCH_APP_CLIENT_ID"
  client_secret: "YOUR_TWITCH_APP_CLIENT_SECRET"
  webhook_secret: "YOUR_CUSTOM_WEBHOOK_SECRET"
  channel_name: "viruslox"
  chat:
    bot_username: "<twitch account acting as bot>"
    bot_id: "YOUR_BOT_NUMERIC_ID"
    channel_to_join: "<channel name>"
    command_cooldown: 15  # General cooldown in seconds (default 15)

# YouTube APIs
youtube: # Leave empty to disable YouTube module
  api_key: ""
  channel_id: ""
  polling_interval: 5  # re-read chat every x seconds; the default is 5

# Enables Overlays creations and reproduction routes: # html -> audio+video overlay via "server" module
# discord -> audio sent via discord module (to voice channel) # streaming -> audio sent via streaming  module (SRT)
overlay:
  enable: yes
  gps:    # gps_overlay.html
    html: yes
    event_type: "gps"  # Configurable listener target for telemetry payloads
  emotes: # emotes_overlay.html
    html: yes
  alerts: # alerts_overlay.html
    html: yes
    discord: yes
    streaming: no
    volume: 75
  chat: # chat_overlay.html
    html: yes
    discord: yes
    streaming: yes
    volume: 75
  scenes: # scenes_overlay.html ("owner chat commands")
    html: yes
    discord: yes
    streaming: yes
    volume: 75

discord:
  token: ""
  prefix: "vlx."       # Commands prefix (es. !join, !leave)
  admins: []           # List of Discord User IDs for bot admins
  guild_id: "<discord server id>" # Optional, but suggested: Restrict to specific Guild ID
  # enable or disable the capture of discord voice channel audio via streaming module (SRT)
  streaming: yes
  # List of Discord User IDs of voice channels partecipante to mute on streaming module
  excluded_users:
    - "<discord user id>"
    - "<discord user id>"

streaming:
  enable: yes
  # SRT Destination (e.g., MediaMTX) # mode=caller to enable "us" as media sender
  destination_url: "srt://127.0.0.1:8890?streamid=publish:ChatBridge/<channel>:<srt user>:<srt pass>&mode=caller&pkt_size=1316"
  bitrate: "128k" # Audio output Bitrate for FFmpeg
  volume: 75    # 0-100, in percentage

audio_source:
  # SRTs source to load and inject into Discord or sent via streaming module.
  enable: yes
  discord: yes
  streaming: no
  url: "srt://some.online.radio"

# VLX Connector configuration (IPC locally for VisionBridge)
# Enables IPC JSON command routing to VLX_VisionBridge for JSON control commands.
connector:
  # Requires connector_enabled: yes in modules block
  ipc_control_out: yes
  control_socket: "/tmp/vlx_control.sock"

# Discord go-live / stream-end announcement (webhook-based, fire-and-forget).
# Go-live fires when Twitch and/or YouTube confirm live. If both go live within
# combine_window seconds, they are merged into a single message.
# Stream-end fires immediately per-platform (no coalescing).
announce:
  enable: no
  webhook_url: "https://discord.com/api/webhooks/<id>/<token>"
  username: "VLX Live"      # webhook display-name override (optional)
  avatar_url: ""            # webhook avatar override; empty = webhook default
  combine_window: 45        # seconds to wait for a 2nd platform before sending
  # Go-live placeholders: {platforms} {title} {url}
  message_template: "🔴 Live now on {platforms}: {title}\n{url}"
  end_enable: no
  # End placeholders: {platform} {url}
  end_message_template: "⚫ {platform} stream has ended."
  # Rich embeds instead of plain text (coloured side-bar, title, fields, footer).
  embed_enable: no
  twitch:
    enable: yes
  youtube:
    enable: yes

control_api:
  enable: yes
  bind_address: "127.0.0.1"
  port: "8760"
  user: "chatbridge"
  pass: "changeme"
  log_unit: "vlx_chatbridge"
```

### Configuration Options Explained

*   **modules**: Hot-swappable toggle for the 6 primary modules (`chatflow`, `audiobridge`, `server`, `streaming`, `audio_source`, `connector`).

*   **server**: Defines bind ports, routing prefixes, allowed WebSocket origins, and the public `base_url` for Twitch Webhooks.

*   **twitch & youtube**: Credentials, polling intervals, and target channels for chat ingestion and EventSub subscriptions.

*   **overlay**: Master switch and individual targets for Emotes, Alerts, Chat, GPS, and Scenes. Can enable HTML web broadcast and audio routing (Discord/Streaming).

*   **discord**: Bot tokens, admin lists, guild restrictions, and excluded users from stream captures.

*   **streaming**: Configures the SRT egress URL, audio bitrate, and baseline volume.

*   **audio_source**: Allows external SRT audio ingestion to pipe directly into Discord or Streaming modules.

*   **connector**: Setup for local IPC UNIX sockets to communicate JSON actions directly with VLX_VisionBridge.

*   **announce**: Cross-platform Discord go-live / stream-end webhook announcer. Supports rich embeds, customizable text, and combines platforms if they go live closely together.

*   **control_api**: Always-on management backend that provides Basic Auth REST endpoints to toggle features and a WebSockets console to stream journalctl logs.


### Reverse Proxy Configuration

The server supports a `path_prefix` configuration token that a reverse proxy can strip to enhance security.
Below is an example of an Apache reverse proxy configuration that properly routes requests, including upgrading WebSocket connections.

**Direct Local Access:** If you access the server directly (e.g., via OBS Browser Source on localhost), the configured `path_prefix` is automatically bypassed. The server dynamically clears this prefix when standard reverse proxy headers (`X-Forwarded-For`, `X-Real-IP`, `X-Forwarded-Host`) are missing, ensuring static assets and WebSockets resolve correctly regardless of the environment.

```apache
RewriteCond %{HTTP:Upgrade} websocket [NC]
RewriteCond %{HTTP:Connection} upgrade [NC]
RewriteRule "^/<path_prefix>/websocket$" "ws://localhost:8000/websocket" [P,L]
ProxyPass /<path_prefix>/ http://localhost:8000/
ProxyPassReverse /<path_prefix>/ http://localhost:8000/
```

### Security Considerations
The WebSocket upgrade path performs strict, exact matching against the configured `websocket_path` in `chatbridge.settings.template` to prevent unauthorized WebSocket connections and aligns with Go 1.24+ `http.ServeMux` standards.


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

Multi-action `.json` files also support the stealth mode feature by structuring the file to include an `auto_delete` boolean flag, an optional `description`, and an `actions` array:
```json
{
  "auto_delete": true,
  "description": "Optional description for Discord /commands list",
  "actions": [
    {
      "type": "ipc_control",
      ...
    }
  ]
}
```

Example with Stealth Mode and Description:
```ini
[ZMQ_CONTROL]
Target=stream
Enabled=true
AutoDelete=true
Description=Optional description for Discord /commands list
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

### Built-in Chat Commands
ChatBridge natively supports several built-in commands for stream interaction.
*   `!followage`: Displays how long a user has been following the channel. Requires the `moderator:read:followers` scope on the broadcaster token.
*   `!lottery`: A complete watch-checked lottery system.
    *   **Broadcaster/Mod Commands:** `!lottery start [time]` (e.g. `!lottery start 10m` to require the winner to be actively watching for the last 10 minutes), `!lottery draw`, `!lottery end`.
    *   **User Commands:** `!lottery join`.
    *   The watch-check evaluates chat activity across both Twitch and YouTube seamlessly using a shared presence tracker.

### Discord Commands
*   `vlx.join` : Bot joins your voice channel and starts the SRT stream.
*   `vlx.leave`: Bot stops streaming and disconnects.
*   `/commands` / `/comandi`: List owner-only available commands (native and reserved Twitch/YouTube chat commands).
*   `/run <command>`: Executes a reserved multi_action command directly from Discord.

## Twitch OAuth2 Credentials and Token Configuration

VLX_ChatBridge utilizes the modern Twitch API (Helix) and IRC interfaces. To ensure security and stability, the system uses an **automatic token renewal** mechanism backed by a local database. This means you will only need to generate your tokens manually **once**, and the system will keep them valid indefinitely.

You will need two sets of credentials: the Application credentials and the User Tokens (one for your main broadcaster channel and one for the Bot).

### Step 1: Create the Twitch Application (Client ID & Secret)
These credentials identify your software on Twitch's servers.
1. Go to the [Twitch Developer Console](https://dev.twitch.tv/console) and log in.
2. Click on **"Register Your Application"**.
3. Choose a name for your app (e.g., `VLX_ChatBridge_App`).
4. In **OAuth Redirect URLs**, enter: `http://localhost`
5. In **Category**, select `Chat Bot`.
6. Click "Create". Once created, click "Manage" to get your **Client ID** and generate a **Client Secret**.
7. Insert these two values into the `vlx_chatbridge.conf` file under the `twitch` section.

### Step 2: Generate Tokens (Access & Refresh)
ChatBridge requires explicit authorization to act as both a Bot and the channel owner (e.g., to delete messages for the Auto-Delete feature).

The fastest way to generate the initial tokens is by using a secure site like [Twitch Token Generator](https://twitchtokengenerator.com/):
1. Go to the site and scroll down to the **"Custom Scope Token"** section.
2. You will need to generate **TWO** distinct tokens. Log in to Twitch first with your **Bot** account, and then repeat the process with your **Broadcaster** account (your main channel).

#### Required Permissions (Scopes)
Select the following boxes with extreme care. It is highly recommended to include all these scopes to ensure compatibility with moderation features and future expansions of the bridge:

**For the BOT account (e.g., VirusRoboLox):**
Check these boxes, then click "Generate Token" while logged in with the bot profile:
* `chat:read` (Required to read commands)
* `chat:edit` (Required to reply in chat)
* `channel:moderate` (Required for basic moderation actions)
* `whispers:read` and `whispers:edit` (If the bot uses whispers)

**For the BROADCASTER account (Your main channel):**
Return to the site, check these boxes, and log in with your main account:
* `moderator:manage:chat_messages` (**REQUIRED** for the Auto-Delete command feature to work)
* `moderator:read:followers` (**REQUIRED** for the !followage command)
* `channel:read:redemptions` (To intercept Channel Points)
* `channel:manage:broadcast` (To update stream info via commands)
* `channel:read:subscriptions` (To intercept subs via API)
* `moderation:read` (To read chat state)
* `chat:read` and `chat:edit` (For security fallback)

*Note: Once generated, the site will provide two long strings for each account: an `ACCESS TOKEN` and a `REFRESH TOKEN`. Copy them and keep them safe.*

### Step 3: Database Insertion (Automatic Renewal)
To enable automatic renewal, you must insert these initial tokens into the ChatBridge SQLite database (`chatbridge.db` or your configured database).
Open the database (via terminal with `sqlite3` or using a GUI tool like DB Browser for SQLite) and execute this query using your data:

```sql
-- Insert Broadcaster (Owner) Token
INSERT OR REPLACE INTO twitch_credentials (user_id, access_token, refresh_token, expires_at) 
VALUES ('BROADCASTER_NUMERIC_ID', 'BROADCASTER_ACCESS_TOKEN', 'BROADCASTER_REFRESH_TOKEN', '2030-01-01 00:00:00');

-- Insert Bot Token
INSERT OR REPLACE INTO twitch_credentials (user_id, access_token, refresh_token, expires_at) 
VALUES ('BOT_NUMERIC_ID', 'BOT_ACCESS_TOKEN', 'BOT_REFRESH_TOKEN', '2030-01-01 00:00:00');
```

(Tip: You can find Twitch account numeric IDs using free sites like Twitch Insights or StreamWeasels).

Once inserted, make sure your vlx_chatbridge.conf file has the bot_id: "BOT_NUMERIC_ID" entry configured. From this moment on, ChatBridge will manage the tokens completely autonomously, silently renewing them before they expire. You will never have to repeat this procedure!

## License
This project is licensed under the GNU General Public License v3.0. See the LICENSE file for details.
