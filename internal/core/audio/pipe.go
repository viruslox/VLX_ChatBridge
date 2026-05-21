package audio

// StreamData represents raw PCM audio data along with an identifier for its source.
type StreamData struct {
	ID           string
	Data         []byte
	RouteSRT     bool
	RouteDiscord bool
}

// PCMChannel is the shared channel for passing raw PCM audio data
// from various sources (ChatFlow, etc.) to the AudioBridge Mixer.
var PCMChannel = make(chan StreamData, 1024)
