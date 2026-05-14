# VLX ChatBridge - Merge Plan

This document outlines the detailed architectural and implementation plan for merging `VLX_ChatFlow` and `VLX_AudioBridge` into a single, unified application: **VLX_ChatBridge**.

## 1. Architectural Overview & Objectives

*   **Unified Binary:** Create a single executable (`VLX_ChatBridge`) that contains the functionality of both ChatFlow and AudioBridge.
*   **Modular Architecture (On-the-fly Toggling):**
    *   The system must have a shared "core" (configuration, logging, database, HTTP server base).
    *   The **ChatFlow module** (Twitch/YouTube integration, overlays, webhooks) and the **AudioBridge module** (Discord bot, SRT streaming, internal audio routing) should be activable and deactivable independently via configuration and/or runtime commands without restarting the whole process.
*   **Direct Audio Integration:**
    *   *Current State:* AudioBridge uses a headless Chromium browser to load the ChatFlow overlay, captures the audio via Pipewire/PulseAudio, and sends it to Discord.
    *   *New State:* The HTML audio capture component in AudioBridge will be **removed**. The ChatFlow module will directly pass PCM/Opus audio to the AudioBridge mixing pipeline internally.
*   **Keep SRT Egress:** The Egress component (Discord -> mixed audio -> FFmpeg -> SRT) remains intact. Audio from ChatFlow (alerts/commands) will be mixed with Discord audio and sent out via SRT.

## 2. Directory Structure Restructuring

A proposed directory structure for the merged project:

```text
VLX_ChatBridge/
├── cmd/
│   └── chatbridge/
│       └── main.go              # Entry point. Initializes core and starts modules.
├── config.yml                   # Unified configuration file.
├── internal/
│   ├── core/                    # Shared components (config, logger, db, system checks)
│   ├── modules/
│   │   ├── chatflow/            # ChatFlow specific logic
│   │   │   ├── server/
│   │   │   ├── twitch/
│   │   │   ├── youtube/
│   │   │   └── websocket/
│   │   └── audiobridge/         # AudioBridge specific logic
│   │       ├── bot/             # Discord Bot
│   │       ├── stream/          # Mixer, SRT egress
│   │       └── internal_audio/  # Internal audio piping from ChatFlow
│   └── system/                  # Pipewire/PulseAudio setup (if still needed for SRT out)
├── static/                      # Frontend overlays (HTML/JS/CSS/Assets)
├── go.mod
└── go.sum
```

## 3. Configuration Merge

Create a unified `config.yml` that encompasses all settings.

*   Add a `modules` section to control what is active.
*   Merge `ServerConfig`, `DatabaseConfig`, `TwitchConfig`, `YouTubeConfig`, `OverlayConfig` from ChatFlow.
*   Merge `DiscordConfig`, `StreamingConfig`, `AudioSource` from AudioBridge.

**Example snippet:**

```yaml
modules:
  chatflow_enabled: true
  audiobridge_enabled: true

# ... (ChatFlow config blocks) ...
server: ...
database: ...
twitch: ...
youtube: ...
overlay: ...

# ... (AudioBridge config blocks) ...
discord: ...
streaming: ...
audio_source: ... # Only SRT sources remain, HTML sources are removed.
```

## 4. Module Toggling Implementation

*   [x] **Interface Definition:** Create a standard interface for modules (e.g., `Start() error`, `Stop() error`).
*   [x] **Core Controller:** The `main.go` or a `ModuleManager` will read the configuration.
    *   If `chatflow_enabled`, initialize and `Start()` the ChatFlow components.
    *   If `audiobridge_enabled`, initialize and `Start()` the Discord bot and Mixer.
*   [x] **Hot-swapping:** Implement an API endpoint (in ChatFlow) or Discord command (in AudioBridge) to trigger `Stop()` or `Start()` on the other module dynamically, updating the running state.

## 5. Removing Headless Browser (Direct Audio Integration)

This is the most critical technical change.

1.  **Remove Dependencies:** Remove `browser_manager.go`, the Chromium dependencies, and the `pactl` Virtual Sink creation specifically for capturing HTML overlays.
2.  **Internal Audio Sink:** In ChatFlow, when an audio command or alert is triggered (e.g., playing an `.mp3` from `static/chat/`), the backend currently just tells the frontend via WebSocket to play it.
3.  **New Flow:**
    *   ChatFlow receives an event (Twitch command, YouTube SuperChat).
    *   ChatFlow decodes the target audio file (`.mp3`/`.wav`) on the backend (using a library like `go-mp3` or piping through a lightweight ffmpeg decode).
    *   ChatFlow passes the raw PCM data (48kHz, Stereo) directly to the AudioBridge `Mixer`.
    *   The `Mixer` (which already handles mixing Discord users) treats the ChatFlow audio as another "user" source.
4.  **AudioBridge Mixer Updates:** Ensure the `Mixer.AddFrame` can accept the internal feed concurrently with Discord packets.

## 6. Updating Audio Sources (SRT only)

*   Modify `audio_source` parsing in the config.
*   Drop support for `type: HTML`.
*   Retain `srt_manager.go` logic to pull external SRT feeds and route them into the mixer (likely requiring a backend decoder to PCM, as it currently uses `ffmpeg -f pulse`). If ffmpeg is still used to decode SRT, it should output raw PCM to a pipe read by the Go application, rather than relying on PulseAudio virtual sinks, to keep the architecture clean and self-contained.

## 7. Execution Steps

1.  **Phase 1: Project Setup & Core Merge**
    - [x] Initialize `VLX_ChatBridge` repo.
    - [x] Merge `go.mod` dependencies.
    - [x] Create unified `config/config.go` handling the combined YAML.
    - [x] Setup the `cmd/chatbridge/main.go` skeleton.
    - [x] Create Module Interface and Manager.
2.  **Phase 2: Porting ChatFlow**
    - [x] Move ChatFlow code into `internal/modules/chatflow`.
    - [x] Ensure the HTTP server, WebSockets, Twitch, and YouTube modules work independently. (HTTP Server base done, skeletons created and initialized)
3.  **Phase 3: Porting AudioBridge & Refactoring**
    - [x] Move AudioBridge code into `internal/modules/audiobridge`.
    - [x] Strip out the `overlay` package (browser manager, PortAudio capture). (Removed and initialization logic replaced with new architecture)
    - [ ] Refactor `Mixer` to accept a direct internal channel for PCM data.
4.  **Phase 4: The Bridge**
    *   Implement an audio decoding utility in ChatFlow (to decode `.mp3` files).
    *   Connect the ChatFlow event handlers (when an alert fires) to send decoded PCM chunks to the AudioBridge Mixer channel.
5.  **Phase 5: Refine Module Toggling**
    - [x] Implement logic to start/stop the Discord Bot and ChatFlow HTTP server without exiting the main process.
6.  **Phase 6: Testing & Cleanup**
    *   Write integration tests verifying audio flows from ChatFlow to the Mixer.
    *   Test SRT egress with mixed audio.
    *   Test runtime toggling.

## 8. Dependencies to Add/Remove

*   **Remove:** Chromium, PortAudio (`portaudio19-dev`), `github.com/jfreymuth/pulse` (if fully moving away from PulseAudio virtual sinks).
*   **Add:** A Go-native audio decoder (e.g., `github.com/hajimehoshi/go-mp3` or `github.com/gordonklaus/portaudio` if keeping ALSA/Pulse access for other reasons, though pure PCM passing is better) to decode alert sounds in the backend.