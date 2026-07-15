package sfx

import "github.com/crgimenes/gion"

// Continuous sounds (the thruster hum, the laser beam) loop while their state is
// active instead of playing once. gion's effect presets are one-shot envelopes, so
// loops come from local recipes tuned to be seamless: a constant envelope (no
// attack/decay) and a whole number of oscillator and vibrato cycles over the loop
// length, so the phase closes exactly where it started. The noise engine hides its
// loop point behind the low-pass; the beam closes phase by construction. These
// recipes can graduate into gion presets once they prove themselves.

// LoopSeconds is the length of one rendered loop. Long enough that the repetition
// is not read as rhythm, short enough to render instantly.
const LoopSeconds = 1.0

// loopRecipes are the continuous sound bases, by name (the `base` an asset's
// continuous sound declares). All frequencies are chosen so freq*LoopSeconds and
// vibratoHz*LoopSeconds are whole numbers (seamless loop).
var loopRecipes = map[string]func(seed int64) gion.Params{
	// engine is a low filtered rumble for the thruster.
	"engine": func(seed int64) gion.Params {
		return gion.Params{
			Wave:    gion.Noise,
			Freq:    420, // sample-and-hold rate: gritty, not hissy
			Sustain: LoopSeconds,
			LowPass: 260,
			Gain:    0.5,
			Seed:    seed,
		}
	},
	// beam is a steady low tone with a slow shimmer for the held laser. No low-pass:
	// the one-pole filter starts cold at the buffer head, and that warm-up transient
	// clicks at the loop splice. A triangle is already soft without it.
	"beam": func(seed int64) gion.Params {
		return gion.Params{
			Wave:      gion.Triangle,
			Freq:      120, // 120 whole cycles per loop: the phase closes exactly
			Sustain:   LoopSeconds,
			Vibrato:   6,
			VibratoHz: 5, // 5 whole shimmer cycles per loop
			Gain:      0.5,
			Seed:      seed,
		}
	},
}

// LoopSamples renders one seamless loop of a continuous base, or nil when the base
// is not a known loop recipe.
func LoopSamples(base string, seed int64) []int16 {
	fn, ok := loopRecipes[base]
	if !ok {
		return nil
	}
	return fn(seed).Render(gion.DefaultRate)
}
