package streaming_test

import (
	"testing"
	"time"

	"VLX_ChatBridge/internal/core/audio"
	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/modules/streaming"
)

func TestSRTEgressWithMixedAudio(t *testing.T) {
	// Initialize and start components
	cfg := &config.Config{}
	outChan := make(chan []byte, 10)
	inChan := make(chan audio.StreamData, 10)
	mixer := audio.NewMixer("TestMixer", 100, true, inChan, outChan)
	srtManager := streaming.NewSRTManager(cfg, outChan)

	if err := mixer.Start(); err != nil {
		t.Fatalf("Failed to start mixer: %v", err)
	}
	defer mixer.Stop()

	if err := srtManager.Start(); err != nil {
		t.Fatalf("Failed to start SRT manager: %v", err)
	}
	defer srtManager.Stop()

	// Simulate sending audio from ChatFlow/Mixer into the audio pipeline for SRT egress
	testChunk := []byte{0x00, 0x01, 0x02, 0x03}
	streamData := audio.StreamData{
		ID:   "test_stream",
		Data: testChunk,
	}

	select {
	case inChan <- streamData:
		t.Log("Successfully sent test audio chunk to PCM pipeline")
	case <-time.After(1 * time.Second):
		t.Fatalf("Timed out sending test audio chunk")
	}

	// Give the mixer and SRT manager some time to process
	time.Sleep(100 * time.Millisecond)

	// Note: Currently SRT and Mixer just log output since the actual FFmpeg/SRT implementation
	// is mocked/stubbed out. This test ensures the pipeline doesn't panic when audio
	// is routed towards the egress, and that start/stop lifecycle works.
}
