//go:build js

package game

import (
	"sync"
	"syscall/js"
)

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

var (
	webAudioOnce sync.Once
	webAudioEl   js.Value // THE audio element — one per process (see ensureWebAudioEl)
	webSwallow   js.Func  // shared no-op rejection handler: an autoplay-gate refusal is expected, not an error

	// webTrackURLs caches one Blob object URL per theme path, so replaying a
	// theme never re-copies the MP3 bytes into JS.
	webTrackURLs = map[string]js.Value{}

	// liveWebTrack is the track that SHOULD be on the air — the target the
	// gesture listeners retry. Package level: the bank swaps tracks, the
	// listeners are registered once.
	liveWebTrack *webTrack
)

// ensureWebAudioEl builds the single persistent <audio> element and hooks the
// page's user gestures. iOS only honors a programmatic play() on an element that
// was unlocked INSIDE a real user-gesture call stack — a fresh element per track
// (the first cut) stayed locked forever when no gesture followed the track
// switch, which is why the iPad attract sometimes had effects but no music. One
// persistent element keeps its unlock across every track switch, and every
// pointer/touch/key gesture retries the pending track right there, inside the
// sanctioned call stack.
func ensureWebAudioEl() js.Value {
	webAudioOnce.Do(func() {
		webAudioEl = js.Global().Get("Audio").New()
		webSwallow = js.FuncOf(func(js.Value, []js.Value) any { return nil })
		retry := js.FuncOf(func(js.Value, []js.Value) any {
			t := liveWebTrack
			if t != nil && t.el.Get("paused").Bool() && !t.el.Get("ended").Bool() {
				webUnlockPending = true // this gesture woke the audio; the game gives it no second job
				t.el.Call("play").Call("catch", webSwallow)
			}
			return nil
		})
		doc := js.Global().Get("document")
		opts := map[string]any{"passive": true}
		doc.Call("addEventListener", "pointerdown", retry, opts)
		doc.Call("addEventListener", "touchend", retry, opts)
		doc.Call("addEventListener", "keydown", retry)
	})
	return webAudioEl
}

// webMusicBlocked reports a track is pending but the browser's autoplay gate is
// still closed (no user gesture since the page loaded — e.g. iOS reloaded the
// tab under the idle attract). The title shows a "tap for sound" hint on it:
// audio before a gesture is a browser impossibility, so the honest move is to
// ask for the tap.
func webMusicBlocked() bool {
	t := liveWebTrack
	return t != nil && t.el.Get("paused").Bool() && !t.el.Get("ended").Bool()
}

// webUnlockPending latches when a user gesture just woke a blocked track, so the
// game can give that gesture no second meaning (a "tap for sound" must not also
// start a run or leave the credits).
var webUnlockPending bool

// consumeWebUnlock reports (and clears) the latch.
func consumeWebUnlock() bool {
	v := webUnlockPending
	webUnlockPending = false
	return v
}

// webTrack is one theme riding the shared element.
type webTrack struct {
	el   js.Value
	tick int // poke cadence counter for the autoplay-gate retry
}

func newWebTrack(key string, data []byte, loop bool, vol float64) *webTrack {
	el := ensureWebAudioEl()
	url, ok := webTrackURLs[key]
	if !ok {
		buf := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(buf, data)
		blob := js.Global().Get("Blob").New([]any{buf}, map[string]any{"type": "audio/mpeg"})
		url = js.Global().Get("URL").Call("createObjectURL", blob)
		webTrackURLs[key] = url
	}
	el.Set("loop", loop)
	el.Set("src", url)
	t := &webTrack{el: el}
	t.setVolume(vol)
	liveWebTrack = t
	t.tryPlay()
	return t
}

// tryPlay starts playback, swallowing the promise rejection the browser throws
// while its autoplay gate is still closed; the gesture listeners and poke retry.
func (t *webTrack) tryPlay() {
	t.el.Call("play").Call("catch", webSwallow)
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

// stop halts playback. The element and its unlock are kept (they are shared);
// only the claim to the air is dropped.
func (t *webTrack) stop() {
	if t == nil {
		return
	}
	t.el.Call("pause")
	if liveWebTrack == t {
		liveWebTrack = nil
	}
}

// poke retries a play blocked by the autoplay gate, about once a second — the
// fallback for desktop policies where any prior gesture unlocks playback without
// a listener firing. Called every frame from soundBank.update.
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
