package bot

import (
	"log"

	"VLX_ChatBridge/internal/core/audio"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"gopkg.in/hraban/opus.v2"
	"encoding/binary"
	"bytes"
)

type DiscordOpusReceiver struct {
	decoder *opus.Decoder
	configDiscordOut bool
	excludedUsers map[string]struct{}
}

func NewDiscordOpusReceiver(discordOutEnabled bool, excludedUsersList []string) *DiscordOpusReceiver {
	// Discord sends 48kHz, 2 channels
	decoder, err := opus.NewDecoder(48000, 2)
	excludedMap := make(map[string]struct{})
	for _, id := range excludedUsersList {
		excludedMap[id] = struct{}{}
	}

	if err != nil {
		log.Printf("[AudioBridge] Failed to create Opus decoder: %v", err)
		return &DiscordOpusReceiver{configDiscordOut: discordOutEnabled, excludedUsers: excludedMap}
	}
	return &DiscordOpusReceiver{
		decoder: decoder,
		configDiscordOut: discordOutEnabled,
		excludedUsers: excludedMap,
	}
}

func (r *DiscordOpusReceiver) ReceiveOpusFrame(userID snowflake.ID, packet *voice.Packet) error {
	if !r.configDiscordOut {
		return nil
	}

	if _, excluded := r.excludedUsers[userID.String()]; excluded {
		return nil
	}

	if r.decoder == nil {
		return nil
	}

	// Opus packets from Discord are typically 20ms at 48kHz stereo = 960 samples per channel = 1920 samples total.
	// 1920 int16 samples * 2 bytes/sample = 3840 bytes.
	// Allocate a slice large enough.
	pcm := make([]int16, 1920)

	n, err := r.decoder.Decode(packet.Opus, pcm)
	if err != nil {
		return err
	}

	// n is the number of samples per channel. For stereo, total samples = n * 2
	totalSamples := n * 2

	// Convert int16 PCM to byte slice (little endian)
	buf := new(bytes.Buffer)
	// We can use binary.Write
	err = binary.Write(buf, binary.LittleEndian, pcm[:totalSamples])
	if err != nil {
		return err
	}

	audio.PCMChannel <- audio.StreamData{
		ID:   "discord_" + userID.String(),
		Data: buf.Bytes(),
	}

	return nil
}

func (r *DiscordOpusReceiver) CleanupUser(userID snowflake.ID) {
	// No user specific state right now.
}

func (r *DiscordOpusReceiver) Close() {
	// Cleanup if necessary
}
