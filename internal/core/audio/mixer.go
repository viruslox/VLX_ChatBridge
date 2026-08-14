package audio

import (
	"bytes"
	"encoding/binary"
	"log"
	"math"
	"sync"
	"time"
)

const (
	sampleRate     = 48000
	channels       = 2
	bytesPerSample = 2 // 16-bit PCM
	tickRateMs     = 20
	chunkSize      = (sampleRate * channels * bytesPerSample * tickRateMs) / 1000 // 3840 bytes for 20ms

	attackTimeMs  = 5.0
	releaseTimeMs = 100.0

	// Feed-forward compressor: reacts only to the causal envelope built from
	// past samples (the same envelope the noise gate already uses), so it
	// needs no lookahead buffer and adds no latency vs. the video pipeline.
	compressorThreshold = 22000.0 // ~ -3.4 dBFS
	compressorRatio     = 3.0     // 3:1 gain reduction above threshold
	makeupGainDb        = 3.0
)

type Mixer struct {
	name       string
	inChan     <-chan StreamData
	outChan    chan<- []byte
	buffers    map[string]*bytes.Buffer
	mu         sync.Mutex
	stopChan   chan struct{}
	stopOnce   sync.Once
	envelope   float64
	gateGain   float64
	compGain   float64
	volume     int
	continuous bool
}

func NewMixer(name string, volume int, continuous bool, inChan <-chan StreamData, outChan chan<- []byte) *Mixer {
	return &Mixer{
		name:       name,
		inChan:     inChan,
		outChan:    outChan,
		buffers:    make(map[string]*bytes.Buffer),
		stopChan:   make(chan struct{}),
		volume:     volume,
		continuous: continuous,
		compGain:   1.0, // start at unity so the first chunk isn't ramped up from silence
	}
}

func (m *Mixer) Start() error {
	log.Printf("[Audio] Mixer (%s) starting...", m.name)

	go m.readLoop()
	go m.mixLoop()

	return nil
}

func (m *Mixer) readLoop() {
	for {
		select {
		case streamData := <-m.inChan:
			m.mu.Lock()
			if _, exists := m.buffers[streamData.ID]; !exists {
				m.buffers[streamData.ID] = new(bytes.Buffer)
			}
			m.buffers[streamData.ID].Write(streamData.Data)
			m.mu.Unlock()
		case <-m.stopChan:
			log.Printf("[Audio] Mixer (%s) stopped reading PCM data.", m.name)
			return
		}
	}
}

func (m *Mixer) mixLoop() {
	ticker := time.NewTicker(time.Millisecond * tickRateMs)
	defer ticker.Stop()

	volumeConfig := m.volume
	if volumeConfig == 0 {
		volumeConfig = 75 // fallback
	}
	baseVolumeMultiplier := float64(volumeConfig) / 100.0

	// Calculate filter coefficients for envelope tracking
	attackCoef := math.Exp(-1.0 / (sampleRate * (attackTimeMs / 1000.0)))
	releaseCoef := math.Exp(-1.0 / (sampleRate * (releaseTimeMs / 1000.0)))

	// Constant compressor makeup gain, computed once rather than per-sample.
	makeupGainLinear := math.Pow(10, makeupGainDb/20.0)

	// Reusable zeroed chunk for sending silence
	silentChunk := make([]byte, chunkSize)

	for {
		select {
		case <-ticker.C:
			m.mu.Lock()

			activeStreams := make([]*bytes.Buffer, 0, len(m.buffers))
			for id, buf := range m.buffers {
				if buf.Len() >= chunkSize {
					activeStreams = append(activeStreams, buf)
				} else if buf.Len() == 0 {
					delete(m.buffers, id)
				}
			}

			if len(activeStreams) == 0 {
				m.mu.Unlock()
				// Smoothly decay envelope and gain when no input
				for i := 0; i < chunkSize/2; i++ {
					m.envelope = releaseCoef * m.envelope
					if m.envelope < 50.0 {
						m.gateGain = releaseCoef * m.gateGain
					}
					// Relax the compressor back to unity gain during silence so
					// the next transient isn't hit with stale gain reduction.
					if m.compGain < 1.0 {
						m.compGain = releaseCoef*m.compGain + (1.0-releaseCoef)*1.0
					}
				}

				if m.continuous {
					// Send a chunk of silence to keep the stream active (e.g., SRT)
					select {
					case m.outChan <- silentChunk:
					default:
						// Silently drop if blocked to avoid log spam in the fast loop
					}
				}
				continue
			}

			chunks := make([][]byte, len(activeStreams))
			for i, buf := range activeStreams {
				chunk := make([]byte, chunkSize)
				buf.Read(chunk)
				chunks[i] = chunk
			}
			m.mu.Unlock()

			mixedChunk := make([]byte, chunkSize)
			samplesPerChunk := chunkSize / 2

			streamCountMultiplier := 1.0
			if len(activeStreams) > 1 {
				streamCountMultiplier = 1.0 / math.Sqrt(float64(len(activeStreams)))
			}

			for i := 0; i < samplesPerChunk; i++ {
				var mixedSample float64

				for j := 0; j < len(chunks); j++ {
					sample := int16(binary.LittleEndian.Uint16(chunks[j][i*2 : i*2+2]))
					mixedSample += float64(sample) * baseVolumeMultiplier * streamCountMultiplier
				}

				// Envelope tracking
				absSample := math.Abs(mixedSample)
				if absSample > m.envelope {
					m.envelope = attackCoef*m.envelope + (1.0-attackCoef)*absSample
				} else {
					m.envelope = releaseCoef*m.envelope + (1.0-releaseCoef)*absSample
				}

				// Noise gate logic based on envelope
				if m.envelope < 50.0 { // Threshold
					m.gateGain = releaseCoef * m.gateGain
				} else {
					m.gateGain = attackCoef*m.gateGain + (1.0-attackCoef)*1.0
				}

				// Feed-forward compressor gain, derived from the same causal
				// envelope used by the gate above -- no additional buffering
				// or lookahead required, so latency vs. the video pipeline is
				// unchanged.
				targetCompGain := 1.0
				if m.envelope > compressorThreshold {
					compressedLevel := compressorThreshold + (m.envelope-compressorThreshold)/compressorRatio
					targetCompGain = compressedLevel / m.envelope
				}
				if targetCompGain < m.compGain {
					m.compGain = attackCoef*m.compGain + (1.0-attackCoef)*targetCompGain
				} else {
					m.compGain = releaseCoef*m.compGain + (1.0-releaseCoef)*targetCompGain
				}

				mixedSample *= m.gateGain * m.compGain * makeupGainLinear

				// Soft clip -- still a memoryless, sample-by-sample function
				// like the previous hard clamp, so it adds no latency; it
				// rounds off peaks instead of chopping them flat.
				mixedSample = 32767.0 * math.Tanh(mixedSample/32767.0)

				finalSample := int16(mixedSample)
				binary.LittleEndian.PutUint16(mixedChunk[i*2:i*2+2], uint16(finalSample))
			}

			select {
			case m.outChan <- mixedChunk:
			default:
				log.Printf("[Audio] Warning: Mixer (%s) output channel blocked, dropping chunk", m.name)
			}

		case <-m.stopChan:
			log.Printf("[Audio] Mixer (%s) stopped mixing loop.", m.name)
			return
		}
	}
}

func (m *Mixer) Stop() error {
	log.Printf("[Audio] Mixer (%s) stopping...", m.name)
	m.stopOnce.Do(func() {
		close(m.stopChan)
	})
	return nil
}
