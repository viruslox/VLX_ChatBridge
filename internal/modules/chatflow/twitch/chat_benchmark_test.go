package twitch

import (
	"github.com/gempir/go-twitch-irc/v4"
	"testing"
)

func BenchmarkEmoteWallURLGeneration_Unoptimized(b *testing.B) {
	message := twitch.PrivateMessage{
		Emotes: []*twitch.Emote{
			{ID: "1", Count: 5},
			{ID: "2", Count: 10},
			{ID: "3", Count: 3},
			{ID: "4", Count: 15},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var emoteURLs []string
		for _, emote := range message.Emotes {
			for j := 0; j < emote.Count; j++ {
				url := "https://static-cdn.jtvnw.net/emoticons/v2/" + emote.ID + "/default/dark/3.0"
				emoteURLs = append(emoteURLs, url)
			}
		}
		_ = emoteURLs
	}
}

func BenchmarkEmoteWallURLGeneration_Optimized(b *testing.B) {
	message := twitch.PrivateMessage{
		Emotes: []*twitch.Emote{
			{ID: "1", Count: 5},
			{ID: "2", Count: 10},
			{ID: "3", Count: 3},
			{ID: "4", Count: 15},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		totalCount := 0
		for _, emote := range message.Emotes {
			totalCount += emote.Count
		}

		emoteURLs := make([]string, 0, totalCount)
		for _, emote := range message.Emotes {
			url := "https://static-cdn.jtvnw.net/emoticons/v2/" + emote.ID + "/default/dark/3.0"
			for j := 0; j < emote.Count; j++ {
				emoteURLs = append(emoteURLs, url)
			}
		}
		_ = emoteURLs
	}
}

func BenchmarkFormatCommandList(b *testing.B) {
	commands := make(AudioCommandsMap)
	commands["foo"] = CommandData{Permission: PermissionEveryone}
	commands["bar"] = CommandData{Permission: PermissionSubscriber}
	commands["baz"] = CommandData{Permission: PermissionVIP}
	commands["qux"] = CommandData{Permission: PermissionEveryone}

	client := &ChatClient{
		commands: commands,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.formatCommandList()
	}
}

func BenchmarkCachedCommandList(b *testing.B) {
	commands := make(AudioCommandsMap)
	commands["foo"] = CommandData{Permission: PermissionEveryone}
	commands["bar"] = CommandData{Permission: PermissionSubscriber}
	commands["baz"] = CommandData{Permission: PermissionVIP}
	commands["qux"] = CommandData{Permission: PermissionEveryone}

	client := &ChatClient{
		commands: commands,
	}
	// Simulate caching
	cachedCmdList := client.formatCommandList()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cachedCmdList
	}
}
