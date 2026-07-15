// Package sfx holds Linefire's pure sound-synthesis helpers over gion, shared by
// the game runtime and the editors so a previewed sound is exactly the played one.
// Everything here is deterministic and device-free: functions return PCM (16-bit
// little-endian stereo at gion.DefaultRate); playing it is the caller's business.
package sfx

import (
	"slices"

	"github.com/crgimenes/gion"
)

// VariationCount is how many siblings of a base sound the game cycles through, so
// repeated shots or explosions do not sound identical.
const VariationCount = 4

// variationParams derives variation k of a base sound: the base itself for k=0, a
// gion.Mutate sibling otherwise. On TONAL waves the pitch is pinned back to the
// base — a ±10% frequency jump between repeats reads as a wrong note on a high
// tone (playtest), while envelope/duty/slide drift still breaks the repetition.
// Noise keeps the full drift (its frequency is texture, not pitch).
func variationParams(p0 gion.Params, k int) gion.Params {
	if k == 0 {
		return p0
	}
	p := gion.Mutate(p0, int64(k))
	if p0.Wave != gion.Noise {
		p.Freq = p0.Freq
		p.FreqSlide = p0.FreqSlide
	}
	return p
}

// Variations resolves a one-shot base (weapon recipe or gion preset) and renders
// the base plus its variations to stereo PCM. Unknown base -> nil.
func Variations(base string, seed int64) [][]byte {
	fn, ok := oneShot(base)
	if !ok {
		return nil
	}
	p0 := fn(seed)
	out := make([][]byte, VariationCount)
	for k := range VariationCount {
		out[k] = Stereo16(variationParams(p0, k).Render(gion.DefaultRate))
	}
	return out
}

// Render synthesizes one base sound — a one-shot (weapon recipe or gion preset), or
// a single pass of a continuous loop — to stereo PCM. It is the editor's preview:
// what this returns is what the game plays. Unknown base -> nil.
func Render(base string, seed int64) []byte {
	if fn, ok := oneShot(base); ok {
		return Stereo16(fn(seed).Render(gion.DefaultRate))
	}
	samples := LoopSamples(base, seed)
	if samples == nil {
		return nil
	}
	return Stereo16(samples)
}

// Known reports whether base names a renderable sound — a one-shot (weapon recipe
// or gion preset) or a continuous loop recipe. Editors use it to flag a typo.
func Known(base string) bool {
	if _, ok := oneShot(base); ok {
		return true
	}
	_, loop := loopRecipes[base]
	return loop
}

// Bases lists every renderable base sound name (weapon recipes + gion presets +
// loop recipes), sorted — the editor's pick list.
func Bases() []string {
	out := make([]string, 0, len(gion.Presets)+len(loopRecipes))
	for _, m := range recipeMaps {
		for name := range m {
			out = append(out, name)
		}
	}
	for name := range gion.Presets {
		out = append(out, name)
	}
	for name := range loopRecipes {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// Stereo16 packs mono samples into the 16-bit little-endian stereo stream the
// audio context expects, duplicating each sample into both channels.
func Stereo16(samples []int16) []byte {
	b := make([]byte, len(samples)*4)
	for i, v := range samples {
		u := uint16(v) // #nosec G115 -- reinterpreting the sample's bits is the encoding
		lo, hi := byte(u&0xff), byte(u>>8)
		b[i*4], b[i*4+1], b[i*4+2], b[i*4+3] = lo, hi, lo, hi
	}
	return b
}
