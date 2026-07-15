package asset

import (
	"fmt"
	"math"
)

// finite reports whether x is a usable coordinate (neither NaN nor Inf).
func finite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

// Validate checks structural and numeric invariants of an asset. It returns a
// descriptive error on the first problem found so callers can report it on the
// terminal instead of silently producing a broken file.
func Validate(a *Asset) error {
	if a == nil {
		return fmt.Errorf("asset is nil")
	}
	if a.Version != CurrentVersion {
		return fmt.Errorf("unsupported version %d (expected %d)", a.Version, CurrentVersion)
	}

	err := validateGeometry(a)
	if err != nil {
		return err
	}
	err = validateLayers(a)
	if err != nil {
		return err
	}
	err = validateHardpoints(a)
	if err != nil {
		return err
	}
	err = validateCollision(a)
	if err != nil {
		return err
	}
	err = validateSounds(a)
	if err != nil {
		return err
	}
	return validateEditor(a)
}

// validateSounds checks the base-sound declarations structurally. The base/preset
// name is NOT resolved here — that is the runtime's job (it owns the gion library),
// so the asset package stays free of an audio dependency.
func validateSounds(a *Asset) error {
	seen := make(map[string]bool, len(a.Sounds))
	for i, s := range a.Sounds {
		if s.Event == "" {
			return fmt.Errorf("sound %d: empty event", i)
		}
		if seen[s.Event] {
			return fmt.Errorf("sound %d: duplicate event %q", i, s.Event)
		}
		seen[s.Event] = true
		if s.Base == "" {
			return fmt.Errorf("sound %d (%q): empty base sound", i, s.Event)
		}
		if !finite(s.Volume) || s.Volume < 0 || s.Volume > 1 {
			return fmt.Errorf("sound %d (%q): volume %g out of [0,1]", i, s.Event, s.Volume)
		}
	}
	return nil
}

// validateGeometry checks the size box and origin.
func validateGeometry(a *Asset) error {
	if !finite(a.Size.W) || !finite(a.Size.H) {
		return fmt.Errorf("size has non-finite values")
	}
	if a.Size.W <= 0 || a.Size.H <= 0 {
		return fmt.Errorf("size must be positive, got %gx%g", a.Size.W, a.Size.H)
	}
	if !finite(a.Origin.X) || !finite(a.Origin.Y) {
		return fmt.Errorf("origin has non-finite values")
	}
	return nil
}

// validateLayers checks every layer's style and paths.
func validateLayers(a *Asset) error {
	for li, layer := range a.Layers {
		if !finite(layer.StrokeWidth) || layer.StrokeWidth < 0 {
			return fmt.Errorf("layer %d (%q): invalid stroke_width %g", li, layer.Name, layer.StrokeWidth)
		}
		if !finite(layer.Glow) {
			return fmt.Errorf("layer %d (%q): non-finite glow", li, layer.Name)
		}
		for pi, p := range layer.Paths {
			err := ValidatePath(p)
			if err != nil {
				return fmt.Errorf("layer %d (%q), path %d: %w", li, layer.Name, pi, err)
			}
		}
	}
	return nil
}

// validateHardpoints checks hardpoint kinds and coordinates.
func validateHardpoints(a *Asset) error {
	for hi, h := range a.Hardpoints {
		if h.Kind != KindWeapon && h.Kind != KindThruster {
			return fmt.Errorf("hardpoint %d (%q): unknown kind %q", hi, h.Name, h.Kind)
		}
		if !finite(h.X) || !finite(h.Y) || !finite(h.Angle) {
			return fmt.Errorf("hardpoint %d (%q): non-finite values", hi, h.Name)
		}
	}
	return nil
}

// validateCollision checks every collision shape by kind.
func validateCollision(a *Asset) error {
	for i, s := range a.Collisions {
		for _, p := range s.Points {
			if !finite(p.X) || !finite(p.Y) {
				return fmt.Errorf("collision %d: non-finite point", i)
			}
		}
		switch s.Kind {
		case CollisionCircle:
			if len(s.Points) != 1 {
				return fmt.Errorf("collision %d: circle needs 1 point, got %d", i, len(s.Points))
			}
			if !finite(s.Radius) || s.Radius < 0 {
				return fmt.Errorf("collision %d: invalid radius %g", i, s.Radius)
			}
		case CollisionRect:
			if len(s.Points) != 2 {
				return fmt.Errorf("collision %d: rect needs 2 points, got %d", i, len(s.Points))
			}
		case CollisionTriangle:
			if len(s.Points) != 3 {
				return fmt.Errorf("collision %d: triangle needs 3 points, got %d", i, len(s.Points))
			}
		default:
			return fmt.Errorf("collision %d: unsupported kind %q", i, s.Kind)
		}
	}
	return nil
}

// validateEditor checks the persisted editor preferences.
func validateEditor(a *Asset) error {
	if !finite(a.Editor.SnapRadiusPx) || a.Editor.SnapRadiusPx <= 0 {
		return fmt.Errorf("editor: invalid snap_radius_px %g", a.Editor.SnapRadiusPx)
	}
	if !finite(a.Editor.GridSize) || a.Editor.GridSize < 0 {
		return fmt.Errorf("editor: invalid grid_size %g", a.Editor.GridSize)
	}
	return nil
}

// ValidatePath verifies a single path (shared by the asset and level checks): it
// must start with a move command and every command must use a known op with
// finite coordinates and, for curves, the right number of finite control points.
func ValidatePath(p Path) error {
	if len(p.Commands) == 0 {
		return fmt.Errorf("path has no commands")
	}
	if p.Commands[0].Op != OpMoveTo {
		return fmt.Errorf("path must start with %q, starts with %q", OpMoveTo, p.Commands[0].Op)
	}
	for ci, c := range p.Commands {
		switch c.Op {
		case OpMoveTo, OpLineTo:
			if !finite(c.X) || !finite(c.Y) {
				return fmt.Errorf("command %d (%s): non-finite coordinate", ci, c.Op)
			}
		case OpQuadTo, OpCubicTo:
			err := validateCurve(c, ci)
			if err != nil {
				return err
			}
		case OpClose:
			// no coordinates to check
		default:
			return fmt.Errorf("command %d: unknown op %q", ci, c.Op)
		}
	}
	return nil
}

// validateCurve checks a Bézier command's endpoint and control points (Q needs
// one, C needs two), all finite.
func validateCurve(c Command, ci int) error {
	if !finite(c.X) || !finite(c.Y) {
		return fmt.Errorf("command %d (%s): non-finite coordinate", ci, c.Op)
	}
	want := 1
	if c.Op == OpCubicTo {
		want = 2
	}
	if len(c.Ctrl) != want {
		return fmt.Errorf("command %d (%s): expected %d control point(s), got %d", ci, c.Op, want, len(c.Ctrl))
	}
	for _, p := range c.Ctrl {
		if !finite(p.X) || !finite(p.Y) {
			return fmt.Errorf("command %d (%s): non-finite control point", ci, c.Op)
		}
	}
	return nil
}
