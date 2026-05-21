package stream

import (
	"bytes"
	"encoding/binary"
	"log"
	"math"
	"sync"
	"time"

	"VLX_ChatBridge/internal/core/audio"
	"VLX_ChatBridge/internal/core/config"
)

const (
	sampleRate     = 48000
	channels       = 2
	bytesPerSample = 2 // 16-bit PCM
	tickRateMs     = 20
	chunkSize      = (sampleRate * channels * bytesPerSample * tickRateMs) / 1000 // 3840 bytes for 20ms

	attackTimeMs  = 5.0
	releaseTimeMs = 100.0
)

type Mixer struct {
	cfg        *config.Config
	outChan    chan<- []byte
	buffers    map[string]*bytes.Buffer
	mu         sync.Mutex
	stopChan   chan struct{}
	stopOnce   sync.Once
	envelope   float64
	gateGain   float64
}

func NewMixer(cfg *config.Config, outChan chan<- []byte) *Mixer {
	return &Mixer{
		cfg:      cfg,
		outChan:  outChan,
		buffers:  make(map[string]*bytes.Buffer),
		stopChan: make(chan struct{}),
	}
}

func (m *Mixer) Start() error {
	log.Println("[AudioBridge] Mixer starting...")

	if !m.cfg.Streaming.Enable {
		log.Println("[AudioBridge] Streaming is disabled. Mixer will only drain audio data.")
	}

	go m.readLoop()
	if m.cfg.Streaming.Enable {
		go m.mixLoop()
	}

	return nil
}

func (m *Mixer) readLoop() {
	for {
		select {
		case streamData := <-audio.PCMChannel:
			if !m.cfg.Streaming.Enable {
				// Discard data when streaming is disabled
				continue
			}

			m.mu.Lock()
			if _, exists := m.buffers[streamData.ID]; !exists {
				m.buffers[streamData.ID] = new(bytes.Buffer)
			}
			m.buffers[streamData.ID].Write(streamData.Data)
			m.mu.Unlock()
		case <-m.stopChan:
			log.Println("[AudioBridge] Mixer stopped reading PCM data.")
			return
		}
	}
}

func (m *Mixer) mixLoop() {
	ticker := time.NewTicker(time.Millisecond * tickRateMs)
	defer ticker.Stop()

	volumeConfig := m.cfg.Streaming.Volume
	if volumeConfig == 0 {
		volumeConfig = 75 // fallback
	}
	baseVolumeMultiplier := float64(volumeConfig) / 100.0

	// Calculate filter coefficients for envelope tracking
	attackCoef := math.Exp(-1.0 / (sampleRate * (attackTimeMs / 1000.0)))
	releaseCoef := math.Exp(-1.0 / (sampleRate * (releaseTimeMs / 1000.0)))

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
				}

				// Send a chunk of silence to keep the SRT stream active
				select {
				case m.outChan <- silentChunk:
				default:
					// Silently drop if blocked to avoid log spam in the fast loop
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

				mixedSample *= m.gateGain

				// Hard clip
				if mixedSample > 32767.0 {
					mixedSample = 32767.0
				} else if mixedSample < -32768.0 {
					mixedSample = -32768.0
				}

				finalSample := int16(mixedSample)
				binary.LittleEndian.PutUint16(mixedChunk[i*2:i*2+2], uint16(finalSample))
			}

			select {
			case m.outChan <- mixedChunk:
			default:
				log.Println("[AudioBridge] Warning: Mixer output channel blocked, dropping chunk")
			}

		case <-m.stopChan:
			log.Println("[AudioBridge] Mixer stopped mixing loop.")
			return
		}
	}
}

func (m *Mixer) Stop() error {
	log.Println("[AudioBridge] Mixer stopping...")
	m.stopOnce.Do(func() {
		close(m.stopChan)
	})
	return nil
}
