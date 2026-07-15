package editorkit

import (
	"bytes"
	"io"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
)

// Audio preview for the editors: play one sound or track at a time, by ear. The
// audio context is a process singleton created on first use, so editors that never
// touch sound never open the device.

var (
	previewCtx    *audio.Context
	previewPlayer *audio.Player
)

func previewContext(rate int) *audio.Context {
	if previewCtx == nil {
		previewCtx = audio.NewContext(rate)
	}
	return previewCtx
}

// stopPreview silences the running preview, if any.
func stopPreview() {
	if previewPlayer == nil {
		return
	}
	_ = previewPlayer.Close()
	previewPlayer = nil
}

// PlayPreview plays a PCM buffer (16-bit LE stereo at rate), replacing any running
// preview. Empty PCM just stops the current one.
func PlayPreview(rate int, pcm []byte, vol float64) {
	stopPreview()
	if len(pcm) == 0 {
		return
	}
	p := previewContext(rate).NewPlayerFromBytes(pcm)
	if vol > 0 && vol < 1 {
		p.SetVolume(vol)
	}
	p.Play()
	previewPlayer = p
}

// PlayPreviewStream plays a decoded audio stream (16-bit LE stereo at rate),
// replacing any running preview — used for long tracks (a stage song) that should
// stream rather than be buffered whole.
func PlayPreviewStream(rate int, src io.Reader, vol float64) {
	stopPreview()
	if src == nil {
		return
	}
	p, err := previewContext(rate).NewPlayer(src)
	if err != nil {
		return
	}
	if vol > 0 && vol < 1 {
		p.SetVolume(vol)
	}
	p.Play()
	previewPlayer = p
}

// PlayPreviewLoop plays a PCM buffer as a seamless infinite loop (a continuous
// sound: an engine, a beam), replacing any running preview. Stop with StopPreview.
func PlayPreviewLoop(rate int, pcm []byte, vol float64) {
	stopPreview()
	if len(pcm) == 0 {
		return
	}
	loop := audio.NewInfiniteLoop(bytes.NewReader(pcm), int64(len(pcm)))
	p, err := previewContext(rate).NewPlayer(loop)
	if err != nil {
		return
	}
	if vol > 0 && vol < 1 {
		p.SetVolume(vol)
	}
	p.Play()
	previewPlayer = p
}

// PlayPreviewMP3 decodes an MP3 file's bytes and streams it as the preview,
// replacing any running one. Undecodable data just stops the current preview.
func PlayPreviewMP3(rate int, data []byte, vol float64) {
	stream, err := mp3.DecodeWithSampleRate(rate, bytes.NewReader(data))
	if err != nil {
		stopPreview()
		return
	}
	PlayPreviewStream(rate, stream, vol)
}

// StopPreview silences the running preview (a Stop button).
func StopPreview() {
	stopPreview()
}
