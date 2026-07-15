package level

import (
	"fmt"
	"math"

	"linefire/asset"
)

func finite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

// Validate checks structural and numeric invariants of a level, returning a
// descriptive error on the first problem found.
func Validate(l *Level) error {
	if l == nil {
		return fmt.Errorf("level is nil")
	}
	if l.Version != CurrentVersion {
		return fmt.Errorf("unsupported version %d (expected %d)", l.Version, CurrentVersion)
	}
	if !finite(l.Size.W) || !finite(l.Size.H) || l.Size.W <= 0 || l.Size.H <= 0 {
		return fmt.Errorf("size must be positive and finite, got %gx%g", l.Size.W, l.Size.H)
	}
	if !finite(l.PlayerStart.X) || !finite(l.PlayerStart.Y) || !finite(l.PlayerStart.Angle) {
		return fmt.Errorf("player_start has non-finite values")
	}

	err := validateWalls(l)
	if err != nil {
		return err
	}
	err = validateSpawns("spawns", l.Spawns)
	if err != nil {
		return err
	}
	err = validateEntries(l.Entries)
	if err != nil {
		return err
	}
	err = validateZones(l)
	if err != nil {
		return err
	}
	err = validateResolutions(l.Resolutions)
	if err != nil {
		return err
	}
	err = validateHorde(l.Horde)
	if err != nil {
		return err
	}

	if !finite(l.Editor.SnapRadiusPx) || l.Editor.SnapRadiusPx <= 0 {
		return fmt.Errorf("editor: invalid snap_radius_px %g", l.Editor.SnapRadiusPx)
	}
	if !finite(l.Editor.GridSize) || l.Editor.GridSize < 0 {
		return fmt.Errorf("editor: invalid grid_size %g", l.Editor.GridSize)
	}
	return nil
}

func validateWalls(l *Level) error {
	for li, layer := range l.Walls {
		if !finite(layer.StrokeWidth) || layer.StrokeWidth < 0 {
			return fmt.Errorf("wall layer %d (%q): invalid stroke_width %g", li, layer.Name, layer.StrokeWidth)
		}
		if !finite(layer.Glow) {
			return fmt.Errorf("wall layer %d (%q): non-finite glow", li, layer.Name)
		}
		for pi, p := range layer.Paths {
			err := asset.ValidatePath(p)
			if err != nil {
				return fmt.Errorf("wall layer %d (%q), path %d: %w", li, layer.Name, pi, err)
			}
		}
	}
	return nil
}

func validateSpawns(kind string, spawns []Spawn) error {
	for i, s := range spawns {
		if s.Asset == "" {
			return fmt.Errorf("%s %d (%q): empty asset reference", kind, i, s.Name)
		}
		if !finite(s.X) || !finite(s.Y) || !finite(s.Angle) {
			return fmt.Errorf("%s %d (%q): non-finite values", kind, i, s.Name)
		}
	}
	return nil
}

func validateEntries(entries []Entry) error {
	for i, e := range entries {
		if !finite(e.X) || !finite(e.Y) || !finite(e.Angle) {
			return fmt.Errorf("entry %d (%q): non-finite values", i, e.Name)
		}
	}
	return nil
}

func validateZones(l *Level) error {
	for i, z := range l.Zones {
		for _, p := range z.Points {
			if !finite(p.X) || !finite(p.Y) {
				return fmt.Errorf("zone %d (%q): non-finite point", i, z.Name)
			}
		}
		switch z.Kind {
		case ZoneRect:
			if len(z.Points) != 2 {
				return fmt.Errorf("zone %d (%q): rect needs 2 points, got %d", i, z.Name, len(z.Points))
			}
		case ZoneCircle:
			if len(z.Points) != 1 {
				return fmt.Errorf("zone %d (%q): circle needs 1 point, got %d", i, z.Name, len(z.Points))
			}
			if !finite(z.Radius) || z.Radius < 0 {
				return fmt.Errorf("zone %d (%q): invalid radius %g", i, z.Name, z.Radius)
			}
		default:
			return fmt.Errorf("zone %d (%q): unsupported kind %q", i, z.Name, z.Kind)
		}
	}
	return nil
}

// validateHorde checks the spawner config: non-negative counts and no empty type
// names. The type NAMES are resolved by the runtime (it owns the archetypes), so
// the level package does not couple to them.
func validateHorde(h *Horde) error {
	if h == nil {
		return nil
	}
	if h.Interval < 0 || h.MaxAlive < 0 || h.Ramp < 0 {
		return fmt.Errorf("horde: interval/max_alive/ramp must be non-negative")
	}
	for i, t := range h.Types {
		if t == "" {
			return fmt.Errorf("horde: type %d is empty", i)
		}
	}
	return nil
}

// validateResolutions checks each condition->routine pair: known names, a target
// where the routine needs one, finite coordinates.
func validateResolutions(rs []Resolution) error {
	for i, r := range rs {
		if r.On != OnCleared {
			return fmt.Errorf("resolution %d: unknown condition %q", i, r.On)
		}
		switch r.Do {
		case DoExit, DoSpawn, DoPortal:
			if r.Target == "" {
				return fmt.Errorf("resolution %d: %s needs a target", i, r.Do)
			}
		case DoReturn, DoWin:
			// no target: return goes back to the origin map; win ends the campaign
		default:
			return fmt.Errorf("resolution %d: unknown routine %q", i, r.Do)
		}
		if !finite(r.X) || !finite(r.Y) {
			return fmt.Errorf("resolution %d: non-finite position", i)
		}
	}
	return nil
}
