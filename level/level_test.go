package level

import (
	"testing"

	"linefire/asset"
)

func TestNewIsValid(t *testing.T) {
	err := Validate(New())
	if err != nil {
		t.Fatalf("default level should be valid: %v", err)
	}
}

func TestCloneIsDeep(t *testing.T) {
	l := New()
	l.Walls[0].Paths = []asset.Path{{Commands: []asset.Command{{Op: asset.OpMoveTo, X: 1, Y: 2}}}}
	l.Spawns = []Spawn{{Name: "e", Asset: "enemy.json", Kind: "enemy", X: 3, Y: 4}}
	l.Zones = []Zone{{Name: "z", Kind: ZoneCircle, Points: []asset.Point{{X: 5, Y: 6}}, Radius: 7}}

	c := l.Clone()
	c.Walls[0].Paths[0].Commands[0].X = 99
	c.Spawns[0].X = 99
	c.Zones[0].Points[0].X = 99

	if l.Walls[0].Paths[0].Commands[0].X != 1 {
		t.Error("wall path shared with clone")
	}
	if l.Spawns[0].X != 3 {
		t.Error("spawn shared with clone")
	}
	if l.Zones[0].Points[0].X != 5 {
		t.Error("zone points shared with clone")
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Level){
		"bad version":   func(l *Level) { l.Version = 99 },
		"zero size":     func(l *Level) { l.Size.W = 0 },
		"empty asset":   func(l *Level) { l.Spawns = []Spawn{{Name: "e"}} },
		"bad zone kind": func(l *Level) { l.Zones = []Zone{{Kind: "blob", Points: []asset.Point{{}}}} },
		"rect one point": func(l *Level) {
			l.Zones = []Zone{{Kind: ZoneRect, Points: []asset.Point{{}}}}
		},
	}
	for name, mutate := range cases {
		l := New()
		mutate(l)
		err := Validate(l)
		if err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}
