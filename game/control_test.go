package game

import (
	"bytes"
	"strings"
	"testing"

	"github.com/crgimenes/linefire/filoio"
)

// The control plane end to end, in process: the game announces its staged
// battle on the event stream, a posted command restages it with a new mode,
// and the announcement follows.
func TestControlPlaneStagesAndReconfigures(t *testing.T) {
	var events bytes.Buffer
	g, err := NewSkirmish(filoio.OSFS(), "../gameassets", SkirmishOptions{
		Mode:   MatchLastFleet,
		Ships:  2,
		Events: &events,
	})
	if err != nil {
		t.Fatalf("NewSkirmish: %v", err)
	}
	if !strings.Contains(events.String(), `"mode":"lastfleet"`) {
		t.Fatalf("the staged battle was not announced: %s", events.String())
	}

	g.PostCommand(MatchCommand{Op: "match", Mode: MatchTimed, Ships: 3, Duration: 60})
	g.stepSkirmishMeta() // the game loop consumes commands between ticks
	if g.match.Mode != MatchTimed || g.match.Ships != 3 || g.match.Duration != 3600 {
		t.Fatalf("the match command did not take: %+v", g.match)
	}
	if !strings.Contains(events.String(), `"mode":"timed"`) {
		t.Fatalf("the restaged battle was not announced: %s", events.String())
	}

	// An invalid command is reported and ignored: the show goes on.
	before := g.match
	g.PostCommand(MatchCommand{Op: "match", Mode: "brawl"})
	g.stepSkirmishMeta()
	if g.match != before {
		t.Fatal("an invalid mode replaced the running match")
	}

	// restart deals a fresh battle of the same shape.
	g.PostCommand(MatchCommand{Op: "restart"})
	g.stepSkirmishMeta()
	if g.match == before {
		t.Fatal("restart should stage a fresh match object")
	}
	if g.match.Mode != MatchTimed {
		t.Fatalf("restart changed the mode to %q", g.match.Mode)
	}
}

// A decided battle goes out on the event stream with its scoreboard.
func TestControlPlaneReportsTheResult(t *testing.T) {
	var events bytes.Buffer
	g, err := NewSkirmish(filoio.OSFS(), "../gameassets", SkirmishOptions{
		Mode:   MatchLastFleet,
		Ships:  1,
		Events: &events,
	})
	if err != nil {
		t.Fatalf("NewSkirmish: %v", err)
	}
	// Decide it by hand: kill everything, then let the meta loop notice.
	g.arrivals = nil
	g.entities = []entity{{kind: kindEnemy, id: 1, hp: 3, faction: 1}}
	g.match.recordKill(1, 2)
	g.stepSkirmishMeta()
	out := events.String()
	if !strings.Contains(out, `"ev":"result"`) || !strings.Contains(out, `"winner":1`) {
		t.Fatalf("the decided battle was not reported: %s", out)
	}
	if !strings.Contains(out, `"Kills":1`) {
		t.Fatalf("the result carries no scoreboard: %s", out)
	}
}
