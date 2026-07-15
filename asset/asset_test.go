package asset

import (
	"math"
	"testing"
)

func TestPathClosed(t *testing.T) {
	open := Path{Commands: []Command{{Op: OpMoveTo}, {Op: OpLineTo, X: 1}}}
	if open.Closed() {
		t.Error("open polyline reported as closed")
	}
	closed := Path{Commands: []Command{{Op: OpMoveTo}, {Op: OpLineTo, X: 1}, {Op: OpClose}}}
	if !closed.Closed() {
		t.Error("closed path reported as open")
	}
}

func TestCloneIsDeep(t *testing.T) {
	a := New()
	a.Layers[0].Paths = []Path{{Commands: []Command{{Op: OpMoveTo, X: 1, Y: 2}}}}
	a.Hardpoints = []Hardpoint{{Name: "g", Kind: KindWeapon, X: 3, Y: 4}}
	a.Collisions = []CollisionShape{{Kind: CollisionCircle, Points: []Point{{X: 5, Y: 6}}, Radius: 7}}
	a.Tags = []string{"player"}

	c := a.Clone()
	c.Layers[0].Paths[0].Commands[0].X = 99
	c.Hardpoints[0].X = 99
	c.Collisions[0].Points[0].X = 99
	c.Collisions[0].Radius = 99
	c.Tags[0] = "enemy"

	if a.Layers[0].Paths[0].Commands[0].X != 1 {
		t.Error("path command shared with clone")
	}
	if a.Hardpoints[0].X != 3 {
		t.Error("hardpoint shared with clone")
	}
	if a.Collisions[0].Points[0].X != 5 || a.Collisions[0].Radius != 7 {
		t.Error("collision shared with clone")
	}
	if a.Tags[0] != "player" {
		t.Error("tags shared with clone")
	}
}

func TestNewIsValid(t *testing.T) {
	err := Validate(New())
	if err != nil {
		t.Fatalf("default asset should be valid: %v", err)
	}
}

func TestSoundsRoundTripAndValidate(t *testing.T) {
	a := New()
	a.Sounds = []Sound{
		{Event: "fire", Base: "laser", Seed: 7, Volume: 0.4},
		{Event: "thruster", Base: "jump", Continuous: true, Muted: true},
	}

	// The file round trip lives in filoio (it owns serialization now); here we only
	// check the model: a deep copy keeps the sounds and still validates.
	b := *a.Clone()
	err := Validate(&b)
	if err != nil {
		t.Fatalf("cloned asset invalid: %v", err)
	}
	if len(b.Sounds) != 2 {
		t.Fatalf("expected 2 sounds, got %d", len(b.Sounds))
	}
	if b.Sounds[0] != (Sound{Event: "fire", Base: "laser", Seed: 7, Volume: 0.4}) {
		t.Fatalf("fire sound not preserved: %+v", b.Sounds[0])
	}
	if !b.Sounds[1].Continuous || !b.Sounds[1].Muted {
		t.Fatal("thruster sound should keep continuous and muted")
	}

	// Clone must deep-copy the sounds (independent backing array).
	c := a.Clone()
	c.Sounds[0].Base = "explosion"
	if a.Sounds[0].Base != "laser" {
		t.Fatal("Clone shares the sounds slice with the original")
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Asset){
		"bad version":   func(a *Asset) { a.Version = 99 },
		"zero size":     func(a *Asset) { a.Size.W = 0 },
		"nan origin":    func(a *Asset) { a.Origin.X = math.NaN() },
		"inf size":      func(a *Asset) { a.Size.H = math.Inf(1) },
		"bad hardpoint": func(a *Asset) { a.Hardpoints = []Hardpoint{{Kind: "laser"}} },
		"bad path start": func(a *Asset) {
			a.Layers[0].Paths = []Path{{Commands: []Command{{Op: OpLineTo, X: 1, Y: 1}}}}
		},
		"unknown op": func(a *Asset) {
			a.Layers[0].Paths = []Path{{Commands: []Command{{Op: "Q", X: 1, Y: 1}}}}
		},
		"negative radius": func(a *Asset) {
			a.Collisions = []CollisionShape{{Kind: CollisionCircle, Points: []Point{{}}, Radius: -1}}
		},
		"circle wrong points": func(a *Asset) {
			a.Collisions = []CollisionShape{{Kind: CollisionCircle, Points: []Point{{}, {}}, Radius: 1}}
		},
		"triangle short": func(a *Asset) {
			a.Collisions = []CollisionShape{{Kind: CollisionTriangle, Points: []Point{{}, {}}}}
		},
		"bad collision kind": func(a *Asset) {
			a.Collisions = []CollisionShape{{Kind: "blob", Points: []Point{{}}}}
		},
		"sound empty event": func(a *Asset) {
			a.Sounds = []Sound{{Base: "laser"}}
		},
		"sound empty base": func(a *Asset) {
			a.Sounds = []Sound{{Event: "fire"}}
		},
		"sound duplicate event": func(a *Asset) {
			a.Sounds = []Sound{{Event: "fire", Base: "laser"}, {Event: "fire", Base: "hit"}}
		},
		"sound volume over one": func(a *Asset) {
			a.Sounds = []Sound{{Event: "fire", Base: "laser", Volume: 1.5}}
		},
	}
	for name, mutate := range cases {
		a := New()
		mutate(a)
		err := Validate(a)
		if err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}
