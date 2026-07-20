//go:build !js

package game

// Native profile: MP3 music plays through oto like everything else. The web
// build routes it to an HTMLAudioElement instead — see musicweb_js.go for why.
const webAudioMusic = false

// webTrack is never instantiated natively; the nil-safe methods keep the music
// code free of build tags.
type webTrack struct{}

func newWebTrack(string, []byte, bool, float64) *webTrack { return nil }

func (t *webTrack) done() bool        { return false }
func (t *webTrack) setVolume(float64) {}
func (t *webTrack) stop()             {}
func (t *webTrack) poke()             {}
