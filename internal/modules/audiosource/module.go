package audiosource

import (
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"VLX_ChatBridge/internal/core/audio"
	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
)

type Module struct {
	config     *config.Config
	controller module.Controller
	cmd        *exec.Cmd
	stopChan   chan struct{}
	stopOnce   sync.Once
	mu         sync.Mutex
}

func NewModule(cfg *config.Config, ctrl module.Controller) *Module {
	return &Module{
		config:     cfg,
		controller: ctrl,
		stopChan:   make(chan struct{}),
	}
}

func (m *Module) Start() error {
	log.Println("[AudioSource] Starting module...")

	if !m.config.AudioSource.Enable {
		log.Println("[AudioSource] Audio Source is disabled in config. Module will not start ingestion.")
		return nil
	}

	if m.config.AudioSource.URL == "" {
		log.Println("[AudioSource] Audio Source URL is empty. Module will not start ingestion.")
		return nil
	}

	go m.runLoop()

	log.Println("[AudioSource] Started successfully.")
	return nil
}

func (m *Module) Name() string {
	return "AudioSource"
}

func (m *Module) runLoop() {
	for {
		select {
		case <-m.stopChan:
			return
		default:
		}

		err := m.run()
		if err != nil {
			log.Printf("[AudioSource] FFmpeg error: %v. Restarting in 5s...", err)
			time.Sleep(5 * time.Second)
		} else {
			// Normal exit (e.g. stopped)
			return
		}
	}
}

func (m *Module) run() error {
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", m.config.AudioSource.URL,
		"-f", "s16le",     // raw 16-bit little-endian PCM
		"-ar", "48000",    // 48kHz sample rate
		"-ac", "2",        // 2 channels (stereo)
		"pipe:1",          // Write to stdout
	)

	m.mu.Lock()
	m.cmd = cmd
	m.mu.Unlock()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[AudioSource] Failed to create stdout pipe for ffmpeg: %v", err)
		return err
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("[AudioSource] Failed to start ffmpeg: %v", err)
		return err
	}

	// Start a goroutine to wait for the command to finish so we don't create zombies
	go func() {
		cmd.Wait()
	}()

	log.Printf("[AudioSource] Ingesting from %s", m.config.AudioSource.URL)

	buf := make([]byte, 3840) // 20ms of 48kHz stereo 16-bit PCM (48000 * 2 * 2 * 0.02)
	for {
		// Stop reading if stopChan is closed
		select {
		case <-m.stopChan:
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

			if m.config.AudioSource.Streaming || m.config.AudioSource.Discord {
				audio.PCMChannel <- audio.StreamData{
					ID:           "audio_source",
					Data:         chunk,
					RouteSRT:     bool(m.config.AudioSource.Streaming),
					RouteDiscord: bool(m.config.AudioSource.Discord),
				}
			}
		}

		if err != nil {
			return err
		}
	}
}

func (m *Module) Stop() error {
	log.Println("[AudioSource] Stopping module...")
	m.stopOnce.Do(func() {
		close(m.stopChan)
		m.mu.Lock()
		if m.cmd != nil && m.cmd.Process != nil {
			m.cmd.Process.Kill()
		}
		m.mu.Unlock()
	})
	log.Println("[AudioSource] Stopped successfully.")
	return nil
}
