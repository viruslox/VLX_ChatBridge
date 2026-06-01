package audio

// StreamData represents raw PCM audio data along with an identifier for its source.
type StreamData struct {
	ID           string
	Data         []byte
	RouteSRT     bool
	RouteDiscord bool
	RouteConnector bool
}

// PCMChannel is the shared channel for passing raw PCM audio data
// from various sources (ChatFlow, etc.) to the AudioBridge Mixer.
var PCMChannel = make(chan StreamData, 1024)

var SRTChannel = make(chan StreamData, 1024)
var DiscordChannel = make(chan StreamData, 1024)
var ConnectorChannel = make(chan StreamData, 1024)

func init() {
	go func() {
		for {
			data := <-PCMChannel
			if data.RouteSRT {
				select {
				case SRTChannel <- data:
				default:
				}
			}
			if data.RouteDiscord {
				select {
				case DiscordChannel <- data:
				default:
				}
			}
			if data.RouteConnector {
				select {
				case ConnectorChannel <- data:
				default:
				}
			}
		}
	}()
}
