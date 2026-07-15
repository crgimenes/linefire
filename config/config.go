// Package config is the model for the player's personal settings — today the audio
// volume and mute plus speedrun best times — kept separate from assets, levels and
// saves. It is a pure leaf: the struct, its defaults, and the one policy constant.
// Reading and writing the file (a denshi-style Filo script) lives in filoio, like every
// other Linefire document, so this package imports nothing and no cycle forms.
package config

// Config is the persisted settings document.
type Config struct {
	Volume    float64        // master audio volume, 0..1
	Muted     bool           //
	BestTimes map[string]int // map name -> best clear time in ticks (speedrun records)
	HighScore int            // best score across all runs (the arcade HI score)
}

// Default is the configuration of a fresh install.
func Default() Config {
	return Config{Volume: 0.7}
}

// MinBestTicks is the smallest clear time accepted as a record. A real clear cannot be
// near-instant, so this rejects glitch/edge-case ~0 times (e.g. re-entering an
// already-cleared map) from ever being stored — a last-line guard for whatever the store
// is (local Filo config today, an online leaderboard later). ~1s at 60 TPS.
const MinBestTicks = 60
