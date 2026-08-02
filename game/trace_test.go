package game

import (
	"bytes"
	"strings"
	"testing"

	"github.com/crgimenes/linefire/filoio"
)

const traceTestHunter = `
	(if first-tick (def sweep (* self-x 0.7)))
	(if (is-empty enemies)
	    (do
	      (def sweep (norm-angle (+ sweep 0.3)))
	      (move (cos sweep) (sin sweep)))
	    (let ((e (head enemies)))
	      (let ((tx (nth e 1)) (ty (nth e 2)))
	        (do
	          (face (bearing self-x self-y tx ty))
	          (move (- tx self-x) (- ty self-y))
	          (fire tx ty)))))
`

// The same seed must replay the same battle byte for byte: that is what makes
// a trace a verifiable record of a dispute rather than an anecdote.
func TestBattleIsDeterministic(t *testing.T) {
	run := func() ([]byte, BattleResult) {
		var buf bytes.Buffer
		res, err := RunBattle(filoio.OSFS(), "../gameassets", BattleOptions{
			Programs: []string{traceTestHunter},
			Ships:    4,
			Seed:     11,
			Trace:    &buf,
		})
		if err != nil {
			t.Fatalf("RunBattle: %v", err)
		}
		return buf.Bytes(), res
	}
	a, ra := run()
	b, rb := run()
	if ra.Winner != rb.Winner || ra.Ticks != rb.Ticks {
		t.Fatalf("same seed, different result: %+v vs %+v", ra, rb)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("same seed, different trace bytes")
	}
	if len(a) == 0 {
		t.Fatal("the trace is empty")
	}
}

// A real battle's trace must pass every structural invariant. Stall findings
// are allowed — they are honest observations, not corruption.
func TestBattleTracePassesTheChecker(t *testing.T) {
	var buf bytes.Buffer
	_, err := RunBattle(filoio.OSFS(), "../gameassets", BattleOptions{
		Programs: []string{traceTestHunter, traceTestHunter},
		Ships:    4,
		Seed:     3,
		Trace:    &buf,
	})
	if err != nil {
		t.Fatalf("RunBattle: %v", err)
	}
	finds, err := CheckTrace(&buf)
	if err != nil {
		t.Fatalf("CheckTrace: %v", err)
	}
	for _, f := range finds {
		if !strings.Contains(f, "stall") {
			t.Errorf("structural violation in a real trace: %s", f)
		}
	}
}

// The checker actually catches corruption: a planted hull raise, an unborn
// shooter and a post-mortem snapshot must each produce a finding.
func TestCheckTraceCatchesPlantedViolations(t *testing.T) {
	trace := strings.Join([]string{
		`{"ev":"battle","n":1,"seed":1,"w":1000,"h":1000,"factions":2,"ships":1}`,
		`{"t":10,"ev":"spawn","id":1,"f":1,"kind":"enemy","x":100,"y":100,"hp":3}`,
		`{"t":10,"ev":"spawn","id":2,"f":2,"kind":"enemy","x":900,"y":900,"hp":3}`,
		`{"t":20,"ev":"hit","id":1,"f":1,"by":99,"dmg":1,"x":100,"y":100,"hp":2}`, // by 99 never spawned
		`{"t":30,"ev":"hit","id":1,"f":1,"by":2,"dmg":1,"x":100,"y":100,"hp":9}`,  // hull ROSE 2 -> 9
		`{"t":40,"ev":"death","id":2,"f":2,"by":1,"x":900,"y":900}`,
		`{"t":60,"ev":"snap","id":2,"f":2,"x":900,"y":900,"hp":3,"mv":0}`,  // snapshot of the dead
		`{"t":70,"ev":"snap","id":1,"f":1,"x":2000,"y":100,"hp":9,"mv":0}`, // outside the arena
		`{"ev":"result","winner":1,"ticks":70,"alive":{"1":5}}`,            // 1 spawned, 0 died, claims 5
	}, "\n")

	finds, err := CheckTrace(strings.NewReader(trace))
	if err != nil {
		t.Fatalf("CheckTrace: %v", err)
	}
	wants := []string{"never spawned", "RAISED", "which is dead", "outside", "survivors"}
	for _, want := range wants {
		found := false
		for _, f := range finds {
			if strings.Contains(f, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("planted violation %q was not caught; findings: %v", want, finds)
		}
	}
}

// A repair is a legitimate way for a hull to rise, and a shield absorbing a
// hit is a legitimate way for one not to move. The checker has to tell those
// from corruption, or salvage would make every real trace look broken.
func TestCheckTraceAcceptsRepairsAndShields(t *testing.T) {
	trace := strings.Join([]string{
		`{"ev":"battle","n":1,"seed":1,"w":1000,"h":1000,"factions":2,"ships":1}`,
		`{"t":10,"ev":"spawn","id":1,"f":1,"kind":"enemy","x":100,"y":100,"hp":4}`,
		`{"t":10,"ev":"spawn","id":2,"f":2,"kind":"enemy","x":900,"y":900,"hp":4}`,
		`{"t":20,"ev":"hit","id":1,"f":1,"by":2,"dmg":2,"x":100,"y":100,"hp":2}`,
		`{"t":30,"ev":"salvage","id":1,"f":1,"kind":"heal","x":100,"y":100,"hp":4}`, // repaired
		`{"t":60,"ev":"snap","id":1,"f":1,"x":140,"y":100,"hp":4,"mv":40}`,          // the rise is explained
		`{"t":70,"ev":"hit","id":1,"f":1,"by":2,"dmg":0,"x":140,"y":100,"hp":4}`,    // a shield held
		`{"ev":"result","winner":0,"ticks":70,"alive":{"1":1,"2":1}}`,
	}, "\n")

	finds, err := CheckTrace(strings.NewReader(trace))
	if err != nil {
		t.Fatalf("CheckTrace: %v", err)
	}
	if len(finds) != 0 {
		t.Fatalf("a repaired and shielded ship is not corruption: %v", finds)
	}

	// The same rise with nothing salvaged IS corruption.
	broken := strings.ReplaceAll(trace, `{"t":30,"ev":"salvage","id":1,"f":1,"kind":"heal","x":100,"y":100,"hp":4}`+"\n", "")
	finds, err = CheckTrace(strings.NewReader(broken))
	if err != nil {
		t.Fatalf("CheckTrace: %v", err)
	}
	found := false
	for _, f := range finds {
		if strings.Contains(f, "nothing salvaged") {
			found = true
		}
	}
	if !found {
		t.Fatalf("an unexplained hull rise should still be caught; findings: %v", finds)
	}
}
