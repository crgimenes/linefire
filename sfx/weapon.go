package sfx

import "github.com/crgimenes/gion"

// shotRecipes are local one-shot weapon sounds, tuned LOW and dark — the stock gion
// presets run bright and piercing, and with a full screen of ships and shots that
// treble stacks up and masks the music (crg's playtest). Low base frequencies plus a
// tight low-pass keep these in the bass/low-mid, out of the melody's way, punchy but
// round. The front gun and turret fire in bursts, so they are the darkest and
// quietest. They resolve by name like a preset, so a weapon (or the editor) just
// names one; variation pitch is pinned (variationParams), so repeats stay this low.
var shotRecipes = map[string]func(seed int64) gion.Params{
	// front gun: a short, soft, dark "pu" (fired constantly — kept unobtrusive).
	"shot_soft": func(seed int64) gion.Params {
		return gion.Params{
			Wave: gion.Triangle, Freq: 300, FreqSlide: -500,
			Sustain: 0.02, Punch: 0.25, Decay: 0.06, LowPass: 1500,
			Gain: 0.45, Seed: seed,
		}
	},
	// turret: a beefier, lower "dow" (also bursty — darker than a bright square).
	"shot_zap": func(seed int64) gion.Params {
		return gion.Params{
			Wave: gion.Square, Freq: 220, FreqSlide: -300, Duty: 0.4,
			Sustain: 0.03, Punch: 0.2, Decay: 0.09, LowPass: 1300,
			Gain: 0.45, Seed: seed,
		}
	},
	// missile launch: a low rising whoosh (infrequent — a moment, not spam).
	"shot_launch": func(seed int64) gion.Params {
		return gion.Params{
			Wave: gion.Saw, Freq: 140, FreqSlide: 220,
			Sustain: 0.12, Decay: 0.12, LowPass: 1300,
			Gain: 0.55, Seed: seed,
		}
	},
	// mine deploy: a dull low "thunk".
	"shot_thunk": func(seed int64) gion.Params {
		return gion.Params{
			Wave: gion.Square, Freq: 170, FreqSlide: -110,
			Sustain: 0.02, Decay: 0.06, LowPass: 1100,
			Gain: 0.5, Seed: seed,
		}
	},
	// the beam cutting out when it overheats: a long falling whistle, "fiuuuu". It has
	// to SAY "it died" without a word on screen, so it is the one weapon sound that
	// lingers — but it still starts low and rounds off, like the rest.
	"laser_cutout": func(seed int64) gion.Params {
		return gion.Params{
			Wave: gion.Triangle, Freq: 460, FreqSlide: -560,
			Sustain: 0.04, Punch: 0.1, Decay: 0.30, LowPass: 2400,
			Gain: 0.5, Seed: seed,
		}
	},
}

// recipeMaps groups Linefire's local one-shot recipes; oneShot and Bases scan them
// in order before falling back to gion's stock presets, so a tuned local name wins.
var recipeMaps = []map[string]func(seed int64) gion.Params{shotRecipes, pickupRecipes}

// oneShot resolves a base name to a one-shot gion params generator: a local recipe
// first (so a tuned name wins), else a stock gion preset. Editors and the game share
// it, so a preview is exactly what plays.
func oneShot(base string) (func(seed int64) gion.Params, bool) {
	for _, m := range recipeMaps {
		if fn, ok := m[base]; ok {
			return fn, true
		}
	}
	fn, ok := gion.Presets[base]
	return fn, ok
}
