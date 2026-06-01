# VLX ChatBridge Architecture

## Overview
VLX ChatBridge is a unified, self-hosted Go application designed to bridge streaming platform events (Twitch, YouTube) with Discord audio and video/audio overlays. It is structured around a centralized core that orchestrates five hot-swappable modules.

## Modules

The system is composed of six primary, independently configurable modules. These are managed by a central `ModuleManager` (located in `internal/core/module`) that provides a `module.Controller` interface to handle asynchronous lifecycle events and hot-swapping without circular dependencies.

1.  **ChatFlow Module**
    *   **Purpose:** Manages event ingestion, logic, and visual overlay coordination.
    *   **Components:**
        *   Twitch integration (EventSub webhooks for Follows/Subs/Raids and IRC client via `go-twitch-irc` for chat commands).
        *   YouTube integration (Live polling for Super Chats, Stickers, and Memberships).
        *   Overlay management (Alerts overlay, Chat Media overlay, and Emote Wall).
        *   WebSocket Hub (`*websocket.Hub`) for real-time OBS Browser Source communication.
        *   State management via SQLite (`*database.DB`).
2.  **AudioBridge Module**
    *   **Purpose:** Handles Discord integration and audio routing to/from Discord.
    *   **Components:**
        *   Discord Bot integration using the `disgo` library.
        *   Supports Discord's End-to-End Encryption (E2EE/DAVE protocol) using `godave` (`libdave`).
        *   Voice audio ingestion via custom Opus receiver (using `gopkg.in/hraban/opus.v2`).
        *   Voice audio egress to Discord using a `DiscordPCMSender` implementing `voice.OpusFrameProvider`.
3.  **Server Module**
    *   **Purpose:** Provides the HTTP webserver interface.
    *   **Components:**
        *   Handles HTTP routing using a shared `http.ServeMux`.
        *   Serves static frontend files (`/static/`).
        *   Parses HTML overlay templates to inject dynamic configuration variables.
        *   Supports reverse proxy setups via a `path_prefix` configuration token.
4.  **Streaming Module**
    *   **Purpose:** Manages SRT (Secure Reliable Transport) streaming egress.
    *   **Components:**
        *   `SRTManager` pipes mixed PCM audio (from `SRTChannel`) via `stdin` to an `ffmpeg` child process using the `fifo` muxer for infinite network reconnects.
5.  **AudioSource Module**
    *   **Purpose:** Handles external audio feed ingest.
    *   **Components:**
        *   Ingests external audio feeds (e.g., internet radio) via an `ffmpeg` child process.
        *   Decodes the audio to raw PCM (s16le, 48000Hz, stereo) and pipes it directly to the internal `audio.PCMChannel`.
6.  **Connector Module**
    *   **Purpose:** Local IPC integration with `VLX_VisionBridge`.
    *   **Components:**
        *   Unix Domain Socket for audio (`/tmp/vlx_audio.sock`), taking raw PCM from the internal `audio.ConnectorChannel`.
        *   Unix Domain Socket for JSON control events (`/tmp/vlx_control.sock`), receiving events mapped from `events.ControlBroadcastChan`.

## Audio Architecture

The audio system replaces traditional headless browser capture with direct internal audio decoding and mixing.

*   **Decoding:** Media files triggered by alerts or chat commands are decoded by FFmpeg (via `os/exec` in `audio.DecodeMediaToPCM`) into 48kHz stereo 16-bit PCM.
*   **Routing:** Audio is initially sent to a shared singleton `PCMChannel` (`chan StreamData`). A central router (`internal/core/audio/pipe.go`) fans out this data to specific channels (`SRTChannel`, `DiscordChannel`, and `ConnectorChannel`) based on configuration flags (`RouteSRT`, `RouteDiscord`, and `RouteConnector`).
*   **Mixing:** Independent `audio.Mixer` instances handle mixing for different outputs (e.g., SRT, Discord, Connector). This separation prevents issues like echoing a Discord participant's audio back to them. The mixer tracks multiple streams by ID, applying dynamic equal-power volume balancing and envelope-based noise gating.

## Database

The primary database schema relies on an SQLite file (`chatbridge.db` located in `$chatbridge_DIR/var/`). It is used for persistence, managing tables such as:
*   `twitch_credentials`
*   `twitch_subscriptions`
*   `youtube_state`

The application interacts with SQLite using `database/sql` and the `github.com/mattn/go-sqlite3` driver.

## Dependency Management

*   **Discord Integration:** `github.com/disgoorg/disgo` (requires `bot.WithVoiceManagerConfigOpts(voice.WithDaveSessionCreateFunc(golibdave.NewSession))` during initialization).
*   **E2EE Voice:** `github.com/disgoorg/godave/golibdave` (requires local compilation of `libdave` v1.1.0).
*   **Twitch IRC:** `github.com/gempir/go-twitch-irc/v4`.
*   **Opus Decoding:** `gopkg.in/hraban/opus.v2`.
