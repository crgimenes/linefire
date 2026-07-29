// Package level defines the Linefire stage (phase) format. A level is a 2D vector world: glowing walls plus gameplay
// markers (player start, enemy and power-up spawns, trigger zones). At runtime
// the ship stays centered and the world rotates/translates around it; that is a
// camera concern, so the level is authored top-down in plain world coordinates.
//
// Levels reuse the asset package's Layer/Path/Point types so walls render with
// the same vector + glow pipeline as ships.
package level

import "github.com/crgimenes/linefire/asset"

// CurrentVersion is the schema version written by this build. v2 unified the
// separate enemies/power_ups lists into a single spawns list, each spawn carrying
// a Kind (category) — see Load for the v1->v2 migration.
const CurrentVersion = 2

// Zone shape kinds.
const (
	ZoneRect   = "rect"   // Points[0], Points[1] = opposite corners
	ZoneCircle = "circle" // Points[0] = center, Radius set
)

// Start is where the player ship begins, with a facing angle in degrees.
type Start struct {
	X     float64
	Y     float64
	Angle float64
}

// Entry is a named arrival point in the map, used as a destination by portals so
// the player lands at a chosen spot (and facing) instead of the default
// PlayerStart. The default PlayerStart is the unnamed entry used on fresh play.
type Entry struct {
	Name  string
	X     float64
	Y     float64
	Angle float64
}

// Spawn places an instance of a saved asset (referenced by file path) in the
// world, with a position and facing angle. Kind is the category, taken from the
// asset: "enemy", "heal", "shield", "score" (and future "ally"/"neutral"); it
// tells the game what to make of the spawn.
type Spawn struct {
	Name   string
	Asset  string // path to the asset JSON
	Kind   string // category (from the asset): enemy / heal / shield / score / portal / ...
	Target string // portal only: "map" or "map:label" (label = a named entry; same map = teleport in place)
	Boss   bool   // enemy only: shielded (invulnerable) until every other enemy on the map is dead, then vulnerable
	X      float64
	Y      float64
	Angle  float64
}

// Zone is a rectangular or circular trigger region.
type Zone struct {
	Name    string
	Kind    string
	Points  []asset.Point
	Radius  float64
	Trigger string // free-form event id
}

// Resolution condition and routine names.
const (
	// Conditions (Resolution.On).
	OnCleared = "cleared" // every enemy the map spawned is destroyed

	// Routines (Resolution.Do).
	DoExit   = "exit"   // leave to Target: "map" or "map:label" (like a portal)
	DoReturn = "return" // go back to the map the player came from
	DoSpawn  = "spawn"  // place the Target asset (a power-up, loot...) at (X, Y)
	DoWin    = "win"    // the campaign is complete: clearing this map wins the game
	DoPortal = "portal" // open a portal at (X, Y) leading to Target — the way onward, revealed on clear
)

// Resolution ties a stage condition to a routine the runtime executes once when
// it is met — e.g. destroying every enemy opens the way out or drops a reward.
// This is what turns "level complete" from informative into an actual outcome.
type Resolution struct {
	On     string  // condition: OnCleared
	Do     string  // routine: DoExit / DoReturn / DoSpawn
	Target string  // exit: "map[:label]"; spawn: asset path
	X      float64 // spawn position
	Y      float64
}

// Horde configures a time-based enemy spawner: it drips enemies of the given kinds
// in around the player, holding at MaxAlive live at once, and ramps the pressure
// over time (interval shrinks, MaxAlive grows). Empty/absent = no spawner. It is
// deterministic from Seed, so wave counts are testable without a window. Zero
// fields fall back to the runtime defaults.
type Horde struct {
	Types    []string // enemy kinds: enemy/turret/rusher/sniper/tank
	Interval int      // starting frames between spawns
	MaxAlive int      // density: hold spawning at this many alive
	Ramp     int      // frames per difficulty step (0 = steady)
	Seed     int64
}

// Level is the top-level stage document; filoio reads and writes it as Filo.
type Level struct {
	Version int
	Name    string
	// Title is the stage's human-facing name — free to be a nostalgic homage ("River
	// Raid 2600", "Pac-Man Maze") where Name is the plain file stem portals use. Optional;
	// the HUD and victory summary fall back to Name when it is empty.
	Title       string
	Size        asset.Size // nominal world extent (editor framing)
	PlayerStart Start
	Walls       []asset.Layer
	Spawns      []Spawn
	Entries     []Entry // named portal arrival points
	Zones       []Zone
	Resolutions []Resolution // condition -> routine, fired once each

	// Music is the stage's ambient theme: either a path to an audio file (ends in
	// ".mp3" — e.g. an AI-generated song) played in a loop, or a gion music mood
	// name ("upbeat", "heroic", "dark", "chill", "battle", "boss") the runtime
	// synthesizes; the seed picks the gion composition. Empty = silence. Enemies
	// declare combat themes on their assets (a "theme" sound), which take over
	// while they are engaged. The doc references audio, never embeds it, and an
	// unknown name simply plays nothing.
	Music     string
	MusicSeed int64

	Horde *Horde // optional time-based enemy spawner

	Editor asset.EditorSettings
	Tags   []string
}

// New returns an empty level with sensible defaults: a 512x512 world, the player
// starting at the center, one empty wall layer and the grid enabled.
func New() *Level {
	return &Level{
		Version:     CurrentVersion,
		Name:        "untitled",
		Size:        asset.Size{W: 512, H: 512},
		PlayerStart: Start{X: 256, Y: 256, Angle: -90},
		Walls: []asset.Layer{{
			Name:        "walls",
			Stroke:      "#80ffff",
			StrokeWidth: 2.0,
			Fill:        "transparent",
			Glow:        0.8,
			Paths:       []asset.Path{},
		}},
		Spawns: []Spawn{},
		Zones:  []Zone{},
		Editor: asset.EditorSettings{
			SnapEnabled:  true,
			SnapRadiusPx: 8,
			GridEnabled:  true,
			GridSize:     16,
			SnapToGrid:   true,
		},
		Tags: []string{},
	}
}

// Clone returns a deep copy of the level, used for editor undo/redo.
func (l *Level) Clone() *Level {
	c := *l

	c.Walls = make([]asset.Layer, len(l.Walls))
	for i, layer := range l.Walls {
		c.Walls[i] = layer
		c.Walls[i].Paths = make([]asset.Path, len(layer.Paths))
		for pi, p := range layer.Paths {
			c.Walls[i].Paths[pi] = p.Clone()
		}
	}

	c.Spawns = make([]Spawn, len(l.Spawns))
	copy(c.Spawns, l.Spawns)

	c.Entries = make([]Entry, len(l.Entries))
	copy(c.Entries, l.Entries)

	c.Zones = make([]Zone, len(l.Zones))
	for i, z := range l.Zones {
		c.Zones[i] = z
		c.Zones[i].Points = make([]asset.Point, len(z.Points))
		copy(c.Zones[i].Points, z.Points)
	}

	c.Resolutions = make([]Resolution, len(l.Resolutions))
	copy(c.Resolutions, l.Resolutions) // Resolution is flat (no nested slices)

	if l.Horde != nil {
		h := *l.Horde
		h.Types = append([]string(nil), l.Horde.Types...)
		c.Horde = &h
	}

	c.Tags = make([]string, len(l.Tags))
	copy(c.Tags, l.Tags)

	return &c
}

// EntryByName returns the named arrival point, if the map defines one.
func (l *Level) EntryByName(name string) (Entry, bool) {
	for _, e := range l.Entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}
