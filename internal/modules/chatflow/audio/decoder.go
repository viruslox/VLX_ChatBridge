package audio

import (
	"fmt"
	"io"
	"os"

	"VLX_ChatBridge/internal/core/audio"
	"github.com/hajimehoshi/go-mp3"
)

// DecodeMP3ToPCM reads an mp3 file, decodes it, and sends the raw PCM
// data to the core PCM channel in chunks.
func DecodeMP3ToPCM(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		return fmt.Errorf("failed to create mp3 decoder: %w", err)
	}

	buf := make([]byte, 8192) // Process in chunks of 8KB
	for {
		n, err := decoder.Read(buf)
		if n > 0 {
			// Copy the slice so we don't hold the buffer in the channel
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			audio.PCMChannel <- audio.StreamData{
				ID:   "chatflow_decoder",
				Data: chunk,
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading from decoder: %w", err)
		}
	}

	return nil
}
