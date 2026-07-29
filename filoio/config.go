package filoio

import (
	"context"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/crgimenes/filo"

	"github.com/crgimenes/linefire/config"
)

// The config document — the player's personal settings, a denshi-style Filo script at
// ~/.config/linefire/config.filo:
//
//	(config
//	  (volume 0.7)
//	  (muted)                       ; a presence flag
//	  (best-time "map0001" 3421)
//	  (best-time "map0002" 5102))
//
// Unlike an asset or a level, this document has no validation gate here: config.Load
// owns the "never stop the game from starting" rule (missing / unreadable / out-of-range
// all degrade to defaults). ParseConfig only turns text into the struct or an error.

// ExtConfig is the config file extension (a denshi-style user script).
const ExtConfig = ".filo"

// ConfigPath is the per-user settings file (~/.config/linefire/config.filo or the OS
// equivalent). An error means the platform has no user config dir; the caller skips
// persistence and plays with defaults.
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "linefire", "config"+ExtConfig), nil
}

// LoadConfig reads the settings, falling back to defaults when the file is missing,
// unreadable, unparseable or out of range — settings must never stop the game from
// starting. This is the one place the degradation policy lives.
func LoadConfig(path string) config.Config {
	c := config.Default()
	// #nosec G304 -- reading the user's own settings file is the purpose
	src, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	parsed, err := ParseConfig(string(src))
	if err != nil {
		return config.Default()
	}
	c = parsed
	if math.IsNaN(c.Volume) || c.Volume < 0 || c.Volume > 1 {
		c.Volume = config.Default().Volume
	}
	return c
}

// SaveConfig writes the settings, creating the directory on first use.
func SaveConfig(path string, c config.Config) error {
	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		return err
	}
	// #nosec G306 -- a personal settings file is fine world-readable
	return os.WriteFile(path, []byte(EmitConfig(c)), 0o600)
}

// SaveAudio persists only the audio settings, preserving every other field (so a volume
// change never wipes the best times). Read-modify-write.
func SaveAudio(path string, volume float64, muted bool) error {
	c := LoadConfig(path)
	c.Volume, c.Muted = volume, muted
	return SaveConfig(path, c)
}

// RecordHighScore stores score as the new HI score when it beats the stored one.
// Returns the high score now and whether this run set a record. Read-modify-write, so a
// score change never wipes the audio settings or best times.
func RecordHighScore(path string, score int) (best int, record bool, err error) {
	c := LoadConfig(path)
	if score <= c.HighScore {
		return c.HighScore, false, nil
	}
	c.HighScore = score
	return score, true, SaveConfig(path, c)
}

// RecordBestTime stores ticks as mapName's best clear time when it beats the stored one
// (or there is none). Times below config.MinBestTicks are rejected as implausible.
// Returns the best time now and whether this run set a record. Read-modify-write.
func RecordBestTime(path, mapName string, ticks int) (best int, record bool, err error) {
	c := LoadConfig(path)
	prev, ok := c.BestTimes[mapName]
	if ticks < config.MinBestTicks || (ok && ticks >= prev) {
		return prev, false, nil // implausibly fast, or not an improvement: not a record
	}
	if c.BestTimes == nil {
		c.BestTimes = map[string]int{}
	}
	c.BestTimes[mapName] = ticks
	return ticks, true, SaveConfig(path, c)
}

// configBuiltins are the forms of a config document.
func configBuiltins() map[string]filo.Builtin {
	return map[string]filo.Builtin{
		"config":     biTagged("config"),
		"volume":     biTagged("volume"),
		"muted":      biTagged("muted"),
		"best-time":  biTagged("best-time"),
		"high-score": biTagged("high-score"),
	}
}

// ParseConfig evaluates Filo source into a config. It does not apply defaults or clamp
// ranges — that is config.Load's job, so the degradation policy lives in one place.
func ParseConfig(src string) (config.Config, error) {
	var out config.Config
	var found bool
	var decodeErr error

	bi := configBuiltins()
	bi["config"] = func(_ context.Context, args []filo.Value) (filo.Value, error) {
		found = true
		out, decodeErr = decodeConfig(args)
		return filo.VNum(0), nil
	}

	f, err := newEngine(bi)
	if err != nil {
		return config.Config{}, err
	}
	defer f.Close()

	err = f.DoString(src)
	if err != nil {
		return config.Config{}, err
	}
	if decodeErr != nil {
		return config.Config{}, decodeErr
	}
	if !found {
		return config.Config{}, fmt.Errorf("no (config ...) form found")
	}
	return out, nil
}

// decodeConfig walks the tagged tree of a (config ...) form.
func decodeConfig(args []filo.Value) (config.Config, error) {
	var c config.Config
	for _, v := range args {
		tag, items, ok := tagOf(v)
		if !ok {
			return config.Config{}, fmt.Errorf("config: unexpected element")
		}
		err := applyConfigField(&c, tag, items)
		if err != nil {
			return config.Config{}, fmt.Errorf("config: %w", err)
		}
	}
	return c, nil
}

// applyConfigField sets one setting from its tagged form.
func applyConfigField(c *config.Config, tag string, items []filo.Value) error {
	switch tag {
	case "volume":
		v, err := one(tag, items)
		if err != nil {
			return err
		}
		c.Volume = v
	case "muted":
		if len(items) != 0 {
			return fmt.Errorf("muted: want (muted)")
		}
		c.Muted = true
	case "best-time":
		return applyBestTime(c, items)
	case "high-score":
		v, err := one(tag, items)
		if err != nil {
			return err
		}
		c.HighScore = int(v)
	default:
		return fmt.Errorf("unknown element %q", tag)
	}
	return nil
}

// applyBestTime reads one (best-time "map" ticks) record.
func applyBestTime(c *config.Config, items []filo.Value) error {
	if len(items) != 2 {
		return fmt.Errorf("best-time: want (best-time \"map\" ticks)")
	}
	name, err := str("best-time", items[0], "map")
	if err != nil {
		return err
	}
	ticks, err := items[1].AsNumber()
	if err != nil {
		return fmt.Errorf("best-time %q: %w", name, err)
	}
	if c.BestTimes == nil {
		c.BestTimes = map[string]int{}
	}
	c.BestTimes[name] = int(ticks)
	return nil
}

// EmitConfig renders a config as Filo text that ParseConfig reads back identically. The
// best times are written in sorted key order so a rewrite produces a stable diff.
func EmitConfig(c config.Config) string {
	var sb strings.Builder
	b := builder{&sb}
	b.printf("(config\n")
	b.printf("  (volume %s)", num(c.Volume))
	if c.Muted {
		b.printf("\n  (muted)")
	}
	if c.HighScore > 0 {
		b.printf("\n  (high-score %d)", c.HighScore)
	}
	for _, name := range slices.Sorted(maps.Keys(c.BestTimes)) {
		b.printf("\n  (best-time %q %d)", name, c.BestTimes[name])
	}
	b.printf(")\n")
	return sb.String()
}
