package modules_test

import (
	"testing"
	"time"
	"os"

	"VLX_ChatBridge/internal/core/audio"
	chatflowaudio "VLX_ChatBridge/internal/modules/chatflow/audio"
)

func TestAudioIntegrationFlow(t *testing.T) {
	// Need to clear the PCMChannel if it has old data, but usually it's empty

	// Generate a dummy valid mp3 file
	dummyMP3Hex := "49443304000000000023545353450000000f0000034c61766636302e31362e3130300000000000000000000000fffb54000000000000000000000000000000000000000000000000000000000000000000496e666f0000000f0000002b000010e000111116161c1c22222227272d2d33333338383e3e44444449494f4f5555555b5b60606666666c6c71717777777d7d82828888888e8e93939999999f9fa4a4aaaaaab0b0b6b6bbbbbbc1c1c7c7ccccccd2d2d8d8dddddde3e3e9e9eeeeeef4f4fafaffff000000004c61766336302e33310000000000000000000000002403c000000000000010e0919f8e61fffb1464000ff00000690000000800000d20000001000001a400000020000034800000044c414d45332e31303055555555555555555555555555555555555555554c414d45332e31303055555555555555555555555555555555555555555555fffb14641e0ff00000690000000800000d20000001000001a4000000200000348000000455555555555555555555555555555555555555555555555555555555554c414d45332e31303055555555555555555555555555555555555555555555fffb14643c0ff00000690000000800000d20000001000001a4000000200000348000000455555555555555555555555555555555555555555555555555555555554c414d45332e31303055555555555555555555555555555555555555555555"
	dummyMP3Bytes := make([]byte, len(dummyMP3Hex)/2)
	for i := 0; i < len(dummyMP3Hex); i += 2 {
		val := 0
		for j := 0; j < 2; j++ {
			c := dummyMP3Hex[i+j]
			if c >= '0' && c <= '9' {
				val = val*16 + int(c-'0')
			} else if c >= 'a' && c <= 'f' {
				val = val*16 + int(c-'a'+10)
			}
		}
		dummyMP3Bytes[i/2] = byte(val)
	}

	tempFile, err := os.CreateTemp("", "test*.mp3")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(dummyMP3Bytes); err != nil {
		t.Fatalf("failed to write dummy mp3: %v", err)
	}
	tempFile.Close()

	errChan := make(chan error, 1)
	go func() {
		errChan <- chatflowaudio.DecodeMP3ToPCM(tempFile.Name())
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
		if err != nil {
			t.Fatalf("DecodeMP3ToPCM failed: %v", err)
		}
	}

	// Discard any remaining audio data to clean up the channel, or we just rely on tests exiting
}
