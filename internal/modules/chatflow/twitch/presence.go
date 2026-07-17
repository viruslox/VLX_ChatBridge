package twitch

import (
	"strings"
	"sync"
	"time"
)

// PresenceTracker records the most recent time each user was seen active in chat
// (Twitch IRC and/or YouTube live chat). It is the "is this user actually
// watching the live?" signal used by the lottery: a user counts as watching if
// they have produced chat activity within a configurable recency window.
//
// This is intentionally platform-agnostic. Both the Twitch ChatClient and the
// YouTube Client feed it via Touch(), so a single tracker instance can be
// shared across both ingestion paths.
type PresenceTracker struct {
	mu       sync.RWMutex
	lastSeen map[string]time.Time // normalized-username -> last activity timestamp
}

// NewPresenceTracker returns an initialized, empty tracker.
func NewPresenceTracker() *PresenceTracker {
	return &PresenceTracker{
		lastSeen: make(map[string]time.Time),
	}
}

// normalizeUser lowercases and trims a username so Twitch/YouTube display-name
// casing differences do not fragment presence records.
func normalizeUser(user string) string {
	return strings.ToLower(strings.TrimSpace(user))
}

// Touch records that the given user was active right now.
func (p *PresenceTracker) Touch(user string) {
	key := normalizeUser(user)
	if key == "" {
		return
	}
	p.mu.Lock()
	p.lastSeen[key] = time.Now()
	p.mu.Unlock()
}

// LastSeen returns the last activity time for a user and whether any record exists.
func (p *PresenceTracker) LastSeen(user string) (time.Time, bool) {
	key := normalizeUser(user)
	p.mu.RLock()
	t, ok := p.lastSeen[key]
	p.mu.RUnlock()
	return t, ok
}

// IsWatching reports whether the user has been active within the given window.
// A window <= 0 is treated as "presence not required" and always returns true.
func (p *PresenceTracker) IsWatching(user string, window time.Duration) bool {
	if window <= 0 {
		return true
	}
	t, ok := p.LastSeen(user)
	if !ok {
		return false
	}
	return time.Since(t) <= window
}

// Prune drops entries older than maxAge to keep the map from growing unbounded
// across long sessions. Safe to call periodically; a no-op if maxAge <= 0.
func (p *PresenceTracker) Prune(maxAge time.Duration) {
	if maxAge <= 0 {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	p.mu.Lock()
	for user, seen := range p.lastSeen {
		if seen.Before(cutoff) {
			delete(p.lastSeen, user)
		}
	}
	p.mu.Unlock()
}
