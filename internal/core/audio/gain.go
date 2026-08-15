package audio

import "encoding/binary"

// ApplyGain scales a 16-bit little-endian PCM buffer in-place by volumePercent.
// volumePercent <= 0 or == 100 is treated as unity gain (no-op), so an unset
// config value never silently mutes a source.
func ApplyGain(buf []byte, volumePercent int) {
	if volumePercent <= 0 || volumePercent == 100 {
		return
	}
	mult := float64(volumePercent) / 100.0
	for i := 0; i+1 < len(buf); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(buf[i : i+2]))
		scaled := float64(sample) * mult
		if scaled > 32767 {
			scaled = 32767
		} else if scaled < -32768 {
			scaled = -32768
		}
		binary.LittleEndian.PutUint16(buf[i:i+2], uint16(int16(scaled)))
	}
}
