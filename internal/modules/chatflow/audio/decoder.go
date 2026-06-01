package audio

import (
	"fmt"
	"io"
	"os/exec"
	"os"

	"VLX_ChatBridge/internal/core/audio"
)

// DecodeMediaToPCM uses FFmpeg to decode any media file to 48kHz stereo 16-bit PCM
// and pushes it to the core PCM channel with the given routing flags.
func DecodeMediaToPCM(id string, filePath string, routeSRT bool, routeDiscord bool, routeConnector bool) error {
	// Setup FFmpeg command with options to ingest audio and decode to raw PCM
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", filePath,
		"-f", "s16le",     // raw 16-bit little-endian PCM
		"-ar", "48000",    // 48kHz sample rate
		"-ac", "2",        // 2 channels (stereo)
		"pipe:1",          // Write to stdout
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe for ffmpeg: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	go func() {
		defer cmd.Wait()

		buf := make([]byte, 3840) // 20ms of 48kHz stereo 16-bit PCM
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				audio.PCMChannel <- audio.StreamData{
					ID:           id,
					Data:         chunk,
					RouteSRT:     routeSRT,
					RouteDiscord: routeDiscord,
					RouteConnector: routeConnector,
				}
			}

			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Printf("[ChatFlow/Audio] Error reading from ffmpeg stdout: %v\n", err)
				break
			}
		}
	}()

	return nil
}

// DecodeMP3ToPCM is kept for backward compatibility and testing.
func DecodeMP3ToPCM(filePath string) error {
	return DecodeMediaToPCM("chatflow_decoder", filePath, true, false, false)
}

// PlayAlert is a helper to decode a specific file with routing flags.
func PlayAlert(id string, filePath string, routeSRT bool, routeDiscord bool, routeConnector bool) {
	if filePath == "" {
		return
	}
	err := DecodeMediaToPCM(id, filePath, routeSRT, routeDiscord, routeConnector)
	if err != nil {
		fmt.Printf("[ChatFlow/Audio] Failed to play alert %s: %v\n", filePath, err)
	}
}
