package sfx

import (
	"slices"
	"strings"

	"github.com/crgimenes/gion"
	"github.com/crgimenes/gion/music"
)

// IsMusicFile reports whether a theme names an audio file rather than a gion mood.
func IsMusicFile(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".mp3")
}

// KnownMood reports whether name is a gion music mood (cheap; Track without the
// render). Editors use it to flag a typo before playing silence.
func KnownMood(name string) bool {
	_, ok := music.Moods[name]
	return ok
}

// Moods lists the gion music mood names, sorted — the editor's pick list for
// theme sounds.
func Moods() []string {
	out := make([]string, 0, len(music.Moods))
	for name := range music.Moods {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// Track synthesizes one looping track for a gion mood name, or nil for a name the
// music engine does not know. The lead is muted: its high notes turn repetitive
// fast in a game loop (playtest + a known gion finding); bass and drums carry the
// mood.
func Track(mood string, seed int64) []byte {
	m, ok := music.Moods[mood]
	if !ok {
		return nil
	}
	p := music.New(m, seed)
	p.Mute = music.MuteLead | music.MuteEcho // the echo doubles the lead: mute both
	return Stereo16(p.Render(gion.DefaultRate))
}
