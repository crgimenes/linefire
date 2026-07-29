package game

import (
	"fmt"

	"github.com/crgimenes/linefire/level"
)

// objectiveKind classifies a level objective.
type objectiveKind int

const (
	objEliminate objectiveKind = iota // clear every enemy in the map
	objReach                          // enter a named goal zone
)

// objective is one condition the player must satisfy to complete the level. done
// latches: once met it stays met even if the player later leaves a goal zone.
type objective struct {
	kind objectiveKind
	zone string // objReach: the zone to enter
	done bool
}

// label describes the objective for the HUD.
func (o objective) label() string {
	switch o.kind {
	case objReach:
		return "Reach " + o.zone
	default:
		return "Clear all enemies"
	}
}

// deriveObjectives builds the level's objectives from its content. Today the only
// derived objective is "clear all enemies", and only on maps whose enemies are FINITE:
// an endless horde can never be cleared, so a horde map gets no clear objective — the
// player escapes through the exit instead.
//
// Reaching the exit is deliberately NOT an objective: the exit always leads to another
// screen, so a checked "reach exit" over there would describe the map you just left, not
// the one you are on. The objReach kind stays in the model for FUTURE goal zones that do
// belong to the current screen (pass a checkpoint, grab an item) — those just need their
// own trigger, not the exit's.
func deriveObjectives(lvl *level.Level, entities []entity) []objective {
	var objs []objective
	if isHordeMap(lvl) {
		return objs // survival map: no "clear all", just find the way out
	}
	for i := range entities {
		if entities[i].kind == kindEnemy {
			objs = append(objs, objective{kind: objEliminate})
			break
		}
	}
	return objs
}

// isHordeMap reports whether the level runs an endless enemy spawner.
func isHordeMap(lvl *level.Level) bool {
	return lvl != nil && lvl.Horde != nil && len(lvl.Horde.Types) > 0
}

// zoneContains reports whether the world point (x,y) is inside the zone.
func zoneContains(z level.Zone, x, y float64) bool {
	switch z.Kind {
	case level.ZoneRect:
		if len(z.Points) < 2 {
			return false
		}
		x0, x1 := sortPair(z.Points[0].X, z.Points[1].X)
		y0, y1 := sortPair(z.Points[0].Y, z.Points[1].Y)
		return x >= x0 && x <= x1 && y >= y0 && y <= y1
	case level.ZoneCircle:
		if len(z.Points) < 1 {
			return false
		}
		dx, dy := x-z.Points[0].X, y-z.Points[0].Y
		return dx*dx+dy*dy <= z.Radius*z.Radius
	default:
		return false
	}
}

func sortPair(a, b float64) (float64, float64) {
	if a <= b {
		return a, b
	}
	return b, a
}

// updateObjectives fires zone-enter triggers and refreshes objective completion.
// Called each playing frame after the world has advanced.
func (g *Game) updateObjectives() {
	for _, z := range g.level.Zones {
		inside := zoneContains(z, g.x, g.y)
		if inside && !g.inZones[z.Name] && z.Trigger != "" {
			if z.Trigger == triggerCheckpoint {
				g.saveCheckpoint(z) // re-saved on each entry, so it holds the best state
			} else {
				g.fireTrigger(z.Trigger)
			}
		}
		g.inZones[z.Name] = inside
	}

	enemies := g.enemiesLeft()
	for i := range g.objectives {
		o := &g.objectives[i]
		if o.done {
			continue
		}
		switch o.kind {
		case objEliminate:
			o.done = enemies == 0
		case objReach:
			if g.inZones[o.zone] {
				o.done = true
				g.curMapState().reached[o.zone] = true // persists across revisits
			}
		}
	}
}

// fireTrigger records a zone trigger the first time the player enters it. Triggers are
// recorded for future actions (doors, waves, ...); none drives an objective today (the
// exit zone is just a portal-adjacent marker now, not a goal).
func (g *Game) fireTrigger(name string) {
	if g.firedTriggers[name] {
		return
	}
	g.firedTriggers[name] = true
}

// zoneByName returns the named zone in the current level.
func (g *Game) zoneByName(name string) (level.Zone, bool) {
	for _, z := range g.level.Zones {
		if z.Name == name {
			return z, true
		}
	}
	return level.Zone{}, false
}

// zoneCenter returns the world-space center of a zone.
func zoneCenter(z level.Zone) (float64, float64) {
	switch z.Kind {
	case level.ZoneCircle:
		if len(z.Points) >= 1 {
			return z.Points[0].X, z.Points[0].Y
		}
	case level.ZoneRect:
		if len(z.Points) >= 2 {
			return (z.Points[0].X + z.Points[1].X) / 2, (z.Points[0].Y + z.Points[1].Y) / 2
		}
	}
	return 0, 0
}

// objectivesComplete reports whether the level has objectives and all are done.
func (g *Game) objectivesComplete() bool {
	if len(g.objectives) == 0 {
		return false
	}
	for i := range g.objectives {
		if !g.objectives[i].done {
			return false
		}
	}
	return true
}

// objectiveLines is the HUD checklist: a header, one line per objective with a
// done mark (the eliminate line shows the remaining count), and a completion note.
func (g *Game) objectiveLines() []string {
	if len(g.objectives) == 0 {
		return nil
	}
	lines := []string{"OBJECTIVES"}
	for i := range g.objectives {
		o := g.objectives[i]
		mark := "[ ]"
		if o.done {
			mark = "[x]"
		}
		label := o.label()
		if o.kind == objEliminate && !o.done {
			label = fmt.Sprintf("%s (%d left)", label, g.enemiesLeft())
		}
		lines = append(lines, "  "+mark+" "+label)
	}
	if g.objectivesComplete() {
		lines = append(lines, "  LEVEL COMPLETE")
	}
	return lines
}
