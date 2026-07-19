//go:build js

package game

import (
	"strings"
	"syscall/js"
)

// Web A/B switches, read once from the page URL (play.html?debug&noaudio). They exist
// to diagnose device-side performance WITHOUT rebuilding: the iPhone playtests showed
// main-thread saturation that only starts when iOS unlocks audio on the first tap, and
// flags let the player isolate audio, music and telemetry right on the device.
//
//	?debug    always show the debug overlay (FPS/TPS, heap/sys), attract included
//	?noaudio  no audio context at all: zero audio cost on the main thread
//	?nomusic  keep the SFX but never start a music track (no streaming mp3 decode)
var webFlags = parseWebFlags(js.Global().Get("location").Get("search").String())

// parseWebFlags splits "?a&b=1" into a lookup set. Pure, testable shape.
func parseWebFlags(search string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(strings.TrimPrefix(search, "?"), "&") {
		name, _, _ := strings.Cut(part, "=")
		if name != "" {
			out[name] = true
		}
	}
	return out
}

// webFlag reports a play.html?flag toggle.
func webFlag(name string) bool { return webFlags[name] }
