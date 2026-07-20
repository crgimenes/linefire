//go:build js

package game

import "syscall/js"

// Web profile: MP3 music plays through an HTMLAudioElement, so the BROWSER's
// media pipeline fetches, decodes and mixes it off the wasm main thread. oto's
// js driver refills its worklet from the MAIN thread ~43×/s, and for MP3 music
// that refill decoded go-mp3 frames synchronously inside the game loop — a
// measured slice of the "FPS 7 = TPS 7" iPhone profile. SFX and pre-rendered
// gion tracks stay on oto: they are raw PCM copies, no decode.
//
// Known trade-off: iOS ignores HTMLMediaElement.volume (hardware volume rules),
// so the volume slider cannot re-level the music there; muted still works and
// setVolume mirrors volume==0 into it.
const webAudioMusic = true

// webTrackURLs caches one Blob object URL per theme path, so replaying a theme
// never re-copies the MP3 bytes into JS.
var webTrackURLs = map[string]js.Value{}

// webTrack is one playing HTMLAudioElement.
type webTrack struct {
	el   js.Value
	tick int // poke cadence counter for the autoplay-gate retry
}

func newWebTrack(key string, data []byte, loop bool, vol float64) *webTrack {
	url, ok := webTrackURLs[key]
	if !ok {
		buf := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(buf, data)
		blob := js.Global().Get("Blob").New([]any{buf}, map[string]any{"type": "audio/mpeg"})
		url = js.Global().Get("URL").Call("createObjectURL", blob)
		webTrackURLs[key] = url
	}
	el := js.Global().Get("Audio").New(url)
	el.Set("loop", loop)
	t := &webTrack{el: el}
	t.setVolume(vol)
	t.tryPlay()
	return t
}

// tryPlay starts playback, swallowing the promise rejection the browser throws
// while its autoplay gate is still closed (before the first user gesture); poke
// retries until it opens.
func (t *webTrack) tryPlay() {
	p := t.el.Call("play")
	var swallow js.Func
	swallow = js.FuncOf(func(js.Value, []js.Value) any {
		swallow.Release()
		return nil
	})
	p.Call("catch", swallow)
}

// done reports a one-shot track has played through (loops never end).
func (t *webTrack) done() bool {
	return t != nil && t.el.Get("ended").Bool()
}

func (t *webTrack) setVolume(v float64) {
	if t == nil {
		return
	}
	t.el.Set("volume", v)
	t.el.Set("muted", v <= 0)
}

// stop halts playback and detaches the media resource so the browser can free
// its decoder promptly (the object URL stays cached for the next play).
func (t *webTrack) stop() {
	if t == nil {
		return
	}
	t.el.Call("pause")
	t.el.Set("src", "")
	t.el.Call("load")
}

// poke retries a play blocked by the autoplay gate, about once a second. Called
// every frame from soundBank.update; a playing or finished track is a no-op.
func (t *webTrack) poke() {
	if t == nil {
		return
	}
	t.tick++
	if t.tick%60 != 0 {
		return
	}
	if t.el.Get("paused").Bool() && !t.el.Get("ended").Bool() {
		t.tryPlay()
	}
}
