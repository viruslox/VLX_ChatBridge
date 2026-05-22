package streaming

import (
	"log"
	"os"
	"os/exec"
	"sync"

	"VLX_ChatBridge/internal/core/config"
)

type SRTManager struct {
	cfg      *config.Config
	inChan   <-chan []byte
	cmd      *exec.Cmd
	stopChan chan struct{}
	stopOnce sync.Once
}

func NewSRTManager(cfg *config.Config, inChan <-chan []byte) *SRTManager {
	return &SRTManager{
		cfg:      cfg,
		inChan:   inChan,
		stopChan: make(chan struct{}),
	}
}

func (s *SRTManager) Start() error {
	log.Println("[Streaming] SRT manager starting...")

	if !s.cfg.Streaming.Enable {
		log.Println("[Streaming] Streaming module is disabled. SRT manager will not start.")
		return nil
	}

	bitrate := s.cfg.Streaming.Bitrate
	if bitrate == "" {
		bitrate = "128k"
	}

	// Setup FFmpeg command with options to keep connection alive
	s.cmd = exec.Command("ffmpeg",
		"-f", "s16le", // raw 16-bit little-endian PCM
		"-ar", "48000", // 48kHz sample rate
		"-ac", "2", // 2 channels (stereo)
		"-i", "pipe:0", // Read from stdin
		"-map", "0:a", // explicitly map audio
		"-c:a", "libopus", // Encode to Opus
		"-b:a", bitrate,
		// Apply fifo to abstract network output, adding reliability, dropped packets rather than blocking, and infinite reconnects
		"-f", "fifo",
		"-fifo_format", "mpegts",
		"-drop_pkts_on_overflow", "1",
		"-attempt_recovery", "1",
		"-recovery_wait_time", "1",
		s.cfg.Streaming.DestinationURL,
	)

	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		log.Printf("[Streaming] Failed to create stdin pipe for ffmpeg: %v", err)
		return err
	}

	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr

	if err := s.cmd.Start(); err != nil {
		log.Printf("[Streaming] Failed to start ffmpeg: %v", err)
		return err
	}

	// Goroutine to stream data to ffmpeg
	go func() {
		defer stdin.Close()
		for {
			select {
			case chunk, ok := <-s.inChan:
				if !ok {
					return
				}
				_, err := stdin.Write(chunk)
				if err != nil {
					log.Printf("[Streaming] Error writing to ffmpeg stdin: %v", err)
					return
				}
			case <-s.stopChan:
				return
			}
		}
	}()

	return nil
}

func (s *SRTManager) Stop() error {
	log.Println("[Streaming] SRT manager stopping...")
	s.stopOnce.Do(func() {
		close(s.stopChan)
		if s.cmd != nil && s.cmd.Process != nil {
			s.cmd.Process.Signal(os.Interrupt)
			s.cmd.Wait()
		}
	})
	return nil
}
