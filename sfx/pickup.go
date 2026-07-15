package sfx

import "github.com/crgimenes/gion"

// pickupRecipes are local one-shot sounds for collecting an item and for something
// arriving on the map. The stock powerup/pickup presets run bright, and two of them
// landing together — a reward dropping as the enemy that triggered it dies — read as
// one shrill note that buries the explosion (crg's playtest). Low base pitch plus a
// low-pass keeps these warm, and collect vs arrival stay distinct from each other and
// from the graver explosion. Variation pitch is pinned (variationParams), so repeats
// keep this low character.
var pickupRecipes = map[string]func(seed int64) gion.Params{
	// collecting a pickup: a warm rising "whoop".
	"collect_soft": func(seed int64) gion.Params {
		return gion.Params{
			Wave: gion.Triangle, Freq: 340, FreqSlide: 600,
			Sustain: 0.05, Punch: 0.2, Decay: 0.16, LowPass: 2600,
			Gain: 0.5, Seed: seed,
		}
	},
	// something new appearing on the map: lower, softer and longer than a pickup, so
	// it does not fight the explosion of the enemy whose death spawned it.
	"arrival_low": func(seed int64) gion.Params {
		return gion.Params{
			Wave: gion.Triangle, Freq: 260, FreqSlide: 300,
			Sustain: 0.06, Decay: 0.22, LowPass: 2000,
			Gain: 0.45, Seed: seed,
		}
	},
	// clearing a stage / the finale: a warm, resolved rising tone. Replaces the stock
	// "powerup" preset, which ran shrill (crg's rule: few or no sounds pitched too high).
	"clear_warm": func(seed int64) gion.Params {
		return gion.Params{
			Wave: gion.Triangle, Freq: 300, FreqSlide: 180,
			Sustain: 0.09, Decay: 0.30, LowPass: 1700,
			Gain: 0.5, Seed: seed,
		}
	},
}
