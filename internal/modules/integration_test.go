package modules_test

import (
	"path/filepath"
	"testing"
	"time"
	"os"

	"VLX_ChatBridge/internal/core/audio"
	chatflowaudio "VLX_ChatBridge/internal/modules/chatflow/audio"
)

func TestAudioIntegrationFlow(t *testing.T) {
	// Need to clear the PCMChannel if it has old data, but usually it's empty

	// Start decoding test.mp3
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	// Go tests run from the package directory, so we need to go up two directories
	testFilePath := filepath.Join(wd, "..", "..", "static", "chat", "test.mp3")

	errChan := make(chan error, 1)
	go func() {
		errChan <- chatflowaudio.DecodeMP3ToPCM(testFilePath)
	}()

	// Wait for PCM data to arrive on the channel
	select {
	case data := <-audio.PCMChannel:
		if len(data.Data) == 0 {
			t.Errorf("received empty chunk on PCM channel")
		} else {
			t.Logf("Received %d bytes of PCM data successfully", len(data.Data))
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timed out waiting for PCM data")
	case err := <-errChan:
		t.Fatalf("DecodeMP3ToPCM failed: %v", err)
	}

	// Discard any remaining audio data to clean up the channel, or we just rely on tests exiting
}
