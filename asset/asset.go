// Package asset defines the Linefire vector asset model and its JSON
// representation. An asset describes the vector geometry and gameplay metadata
// (origin, hardpoints, collision) for a single object such as a ship, enemy,
// projectile or power-up.
package asset

// DefaultFileName is where the asset editor saves when it was started without a path.
const DefaultFileName = "linefire_asset.lfa"

// CurrentVersion is the schema version written by this build. Older versions
// are migrated on load (see load.go); newer ones are refused.
const CurrentVersion = 2

// Path command operations.
const (
	OpMoveTo  = "M" // start a new sub-path at (X, Y)
	OpLineTo  = "L" // straight line to (X, Y)
	OpQuadTo  = "Q" // quadratic Bézier to (X, Y) with one control point (Ctrl[0])
	OpCubicTo = "C" // cubic Bézier to (X, Y) with two control points (Ctrl[0..1])
	OpClose   = "Z" // close the current sub-path
)

// flattenTol is the default chord tolerance, in asset-space units, used when
// subdividing Bézier curves into line segments. Small enough to read as smooth at
// normal zoom while keeping the segment count bounded.
const flattenTol = 0.3

// Hardpoint kinds.
const (
	KindWeapon   = "weapon"
	KindThruster = "thruster"
)

// Collision shape kinds.
const (
	CollisionCircle   = "circle"   // Points[0] = center, Radius set
	CollisionRect     = "rect"     // Points[0], Points[1] = opposite corners
	CollisionTriangle = "triangle" // Points[0..2] = vertices
)

// Point is a 2D coordinate in asset space (pixels, origin at top-left).
type Point struct {
	X float64
	Y float64
}

// Size is the nominal bounding box of the asset in asset-space pixels.
type Size struct {
	W float64
	H float64
}

// Command is a single drawing instruction inside a Path. (X, Y) is the endpoint;
// Ctrl holds the Bézier control points (one for OpQuadTo, two for OpCubicTo) and
// is empty otherwise. For OpClose the coordinates are unused.
type Command struct {
	Op   string
	X    float64
	Y    float64
	Ctrl []Point // curve control points (Q: 1, C: 2)
}

// Path is an ordered list of drawing commands forming one open or closed shape.
type Path struct {
	Commands []Command
}

// Clone returns a deep copy of the path, including each command's curve control
// points, so editors can snapshot it for undo without sharing mutable state.
func (p Path) Clone() Path {
	cmds := make([]Command, len(p.Commands))
	for i, c := range p.Commands {
		cmds[i] = c
		if len(c.Ctrl) > 0 {
			cmds[i].Ctrl = append([]Point(nil), c.Ctrl...)
		}
	}
	return Path{Commands: cmds}
}

// Closed reports whether the path is explicitly closed (contains a close
// command). Open paths are stroked as polylines and are never filled, so simple
// lines render as lines rather than filled regions.
func (p Path) Closed() bool {
	for _, c := range p.Commands {
		if c.Op == OpClose {
			return true
		}
	}
	return false
}

// Layer groups paths that share the same stroke and fill style. Layers are
// drawn in slice order; later layers paint on top.
type Layer struct {
	Name        string
	Stroke      string  // hex color, e.g. "#80ffff"
	StrokeWidth float64 // in asset-space pixels
	Fill        string  // hex color or "transparent"
	Glow        float64 // reserved for the future game renderer
	Hidden      bool
	Paths       []Path
}

// Hardpoint marks a gameplay attachment point such as a gun muzzle or thruster.
type Hardpoint struct {
	Name  string
	Kind  string // KindWeapon or KindThruster
	X     float64
	Y     float64
	Angle float64 // degrees; 0 points right, -90 points up
}

// CollisionShape is one hitbox primitive. The meaning of Points and Radius
// depends on Kind (see the collision kind constants).
type CollisionShape struct {
	Kind   string
	Points []Point
	Radius float64
}

// Sound is one base sound effect declared on an asset. The runtime synthesizes it
// (and several variations of it) at load with the gion library, so a repeated event
// does not sound identical, at zero asset cost. The asset stores only the recipe,
// never audio samples.
//
// Event is the trigger name, by object type: a weapon's "fire", a power-up's
// "pickup", a ship's "destroy"/"thruster"/"hit", a special effect's "impact". Base
// names the base sound: a gion preset family ("laser", "explosion", "hit",
// "pickup", "powerup", "jump", "blip") or, in a future revision, an entry in an
// authored .gion document. Seed picks the base variation (0 = the preset default).
// Volume overrides the gain (0 = the preset's own). Continuous loops the sound
// while its state is active (an engine, a held laser) instead of playing once.
// Muted switches the sound off without deleting the entry (its tuning survives);
// a muted event is fully silent — it does not fall back to a default sound.
type Sound struct {
	Event      string
	Base       string
	Seed       int64
	Volume     float64
	Continuous bool
	Muted      bool
}

// EditorSettings persists editor preferences inside the asset file so a project
// reopens with the same snapping and grid behaviour.
type EditorSettings struct {
	SnapEnabled  bool
	SnapRadiusPx float64
	GridEnabled  bool
	GridSize     float64
	SnapToGrid   bool
}

// Asset is the top-level document; filoio reads and writes it as Filo.
type Asset struct {
	Version    int
	Name       string
	Kind       string // spawn category: "enemy", "heal", "shield", "score", "ally", "neutral"
	Size       Size
	Origin     Point
	Layers     []Layer
	Hardpoints []Hardpoint
	Collisions []CollisionShape
	Sounds     []Sound // base sounds synthesized at load (gion)
	Editor     EditorSettings
	Tags       []string
}

// New returns an empty asset with sensible defaults: a 64x64 box, a centered
// origin and a single empty "main" layer using the default neon style.
func New() *Asset {
	return &Asset{
		Version: CurrentVersion,
		Name:    "untitled",
		Size:    Size{W: 64, H: 64},
		Origin:  Point{X: 32, Y: 32},
		Layers: []Layer{{
			Name:        "main",
			Stroke:      "#80ffff",
			StrokeWidth: 2.0,
			Fill:        "transparent",
			Glow:        0.8,
			Paths:       []Path{},
		}},
		Hardpoints: []Hardpoint{},
		Editor: EditorSettings{
			SnapEnabled:  true,
			SnapRadiusPx: 8,
			GridEnabled:  true,
			GridSize:     4,
			SnapToGrid:   false,
		},
		Tags: []string{},
	}
}

// Clone returns a deep copy of the asset, independent of the original. It is
// used for editor history (undo/redo) and is safe to mutate.
func (a *Asset) Clone() *Asset {
	c := *a

	c.Layers = make([]Layer, len(a.Layers))
	for i, l := range a.Layers {
		c.Layers[i] = l
		c.Layers[i].Paths = make([]Path, len(l.Paths))
		for pi, p := range l.Paths {
			c.Layers[i].Paths[pi] = p.Clone()
		}
	}

	c.Hardpoints = make([]Hardpoint, len(a.Hardpoints))
	copy(c.Hardpoints, a.Hardpoints)

	c.Collisions = make([]CollisionShape, len(a.Collisions))
	for i, s := range a.Collisions {
		c.Collisions[i] = s
		c.Collisions[i].Points = make([]Point, len(s.Points))
		copy(c.Collisions[i].Points, s.Points)
	}

	c.Sounds = make([]Sound, len(a.Sounds))
	copy(c.Sounds, a.Sounds) // Sound is flat (no nested slices/pointers)

	c.Tags = make([]string, len(a.Tags))
	copy(c.Tags, a.Tags)

	return &c
}
