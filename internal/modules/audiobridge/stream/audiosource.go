package stream

import (
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"VLX_ChatBridge/internal/core/audio"
	"VLX_ChatBridge/internal/core/config"
)

type AudioSourceManager struct {
	cfg      *config.Config
	cmd      *exec.Cmd
	stopChan chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
}

func NewAudioSourceManager(cfg *config.Config) *AudioSourceManager {
	return &AudioSourceManager{
		cfg:      cfg,
		stopChan: make(chan struct{}),
	}
}

func (a *AudioSourceManager) Start() error {
	log.Println("[AudioBridge] Audio Source manager starting...")

	if !a.cfg.AudioSource.Enable {
		log.Println("[AudioBridge] Audio Source module is disabled. Audio Source manager will not start.")
		return nil
	}

	if a.cfg.AudioSource.URL == "" {
		log.Println("[AudioBridge] Audio Source URL is empty. Audio Source manager will not start.")
		return nil
	}

	go a.runLoop()

	return nil
}

func (a *AudioSourceManager) runLoop() {
	for {
		select {
		case <-a.stopChan:
			return
		default:
		}

		err := a.run()
		if err != nil {
			log.Printf("[AudioBridge] Audio Source FFmpeg error: %v. Restarting in 5s...", err)
			time.Sleep(5 * time.Second)
		} else {
			// Normal exit (e.g. stopped)
			return
		}
	}
}

func (a *AudioSourceManager) run() error {
	// Setup FFmpeg command with options to ingest audio and decode to raw PCM
	// Audio Source output is routed to audio.PCMChannel

	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", a.cfg.AudioSource.URL,
		"-f", "s16le",     // raw 16-bit little-endian PCM
		"-ar", "48000",    // 48kHz sample rate
		"-ac", "2",        // 2 channels (stereo)
		"pipe:1",          // Write to stdout
	)

	a.mu.Lock()
	a.cmd = cmd
	a.mu.Unlock()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[AudioBridge] Failed to create stdout pipe for ffmpeg: %v", err)
		return err
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("[AudioBridge] Failed to start ffmpeg: %v", err)
		return err
	}

	// Start a goroutine to wait for the command to finish so we don't create zombies
	go func() {
		cmd.Wait()
	}()

	log.Printf("[AudioBridge] Ingesting Audio Source from %s", a.cfg.AudioSource.URL)

	buf := make([]byte, 3840) // 20ms of 48kHz stereo 16-bit PCM (48000 * 2 * 2 * 0.02)
	for {
		// Stop reading if stopChan is closed
		select {
		case <-a.stopChan:
			if cmd != nil && cmd.Process != nil {
				cmd.Process.Kill()
			}
			return nil
		default:
		}

		n, err := stdout.Read(buf)
		if n > 0 {
			// Copy the slice so we don't hold the buffer in the channel
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			// We always decode to the mixer if either streaming or discord requires it
			// However the instructions say:
			// audio_source (enable: yes, streaming: yes) -> decoded to PCM -> audio.PCMChannel (mixer)
			// Actually if Discord needs it too, it routes through the mixer
			if a.cfg.AudioSource.Streaming || a.cfg.AudioSource.Discord {
				audio.PCMChannel <- audio.StreamData{
					ID:   "audio_source",
					Data: chunk,
				}
			}
		}

		if err != nil {
			return err
		}
	}
}

func (a *AudioSourceManager) Stop() error {
	log.Println("[AudioBridge] Audio Source manager stopping...")
	a.stopOnce.Do(func() {
		close(a.stopChan)
		a.mu.Lock()
		if a.cmd != nil && a.cmd.Process != nil {
			a.cmd.Process.Kill()
		}
		a.mu.Unlock()
	})
	return nil
}
