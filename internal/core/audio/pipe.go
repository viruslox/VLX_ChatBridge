package audio

// PCMChannel is the shared channel for passing raw PCM audio data
// from ChatFlow to the AudioBridge Mixer.
var PCMChannel = make(chan []byte, 1024)
