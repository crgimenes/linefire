package game

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/crgimenes/filo"
)

// Validation for fleet programs, made for editors: the garage's editor calls
// this on demand and marks the failing line, instead of the player finding a
// broken program only when a battle quietly falls back to the house brain.

// PilotIssue is one problem in a fleet program. Line is 1-based, or 0 when the
// failure has no usable position (runtime errors do not carry one).
type PilotIssue struct {
	Line int
	Msg  string
}

// ValidatePilot checks a Filo fleet program the way the battle will actually
// treat it, in two stages. First it compiles with the REAL pilot engine — the
// same builtins the battle registers, so nothing passes here that would fail
// there. Then it flies ONE dry tick on a synthetic ship with live instruments,
// which is what catches the errors Filo only raises at runtime: an undefined
// global, a wrong arity, a step-limit blowup. A dynamic language cannot promise
// more than the paths it ran — a branch the dry tick never took stays
// unchecked — but the first tick is the one every program runs, and it is
// where the overwhelming majority of mistakes surface.
//
// nil means the program compiles and survives its first tick.
func ValidatePilot(src string) *PilotIssue {
	eng := newPilotEngine()
	prog, err := eng.Compile(src)
	if err != nil {
		return &PilotIssue{Line: lineOfParseError(src, err), Msg: err.Error()}
	}

	// The synthetic battlefield: one ship of faction 1, one visible foe, one
	// pickup, so the instrument lists are populated and a program that indexes
	// into them exercises its real first-tick path. Built through the same
	// fillInstruments the battle uses — when the contract grows, the dry tick
	// grows with it instead of drifting.
	g := &Game{}
	g.skirmishMode = true
	g.simTick = 1
	g.bounds = bounds{minX: 0, minY: 0, maxX: 1000, maxY: 1000}
	g.match, _ = newMatch(MatchLastFleet, 2, 0, 2)
	g.entities = []entity{
		{kind: kindEnemy, id: 1, x: 400, y: 500, radius: 12, hp: 3, hpMax: 3, faction: 1, radar: 400, fireEvery: 60},
		{kind: kindEnemy, id: 2, x: 600, y: 500, radius: 12, hp: 3, hpMax: 3, faction: 2, radar: 400, fireEvery: 60},
		{kind: kindPowerUp, power: powerHeal, x: 500, y: 400, radius: 8, hp: pickupHP},
	}
	e := &g.entities[0]

	mem := map[string]filo.Value{}
	g.fillInstruments(e, mem)
	mem["first-tick"] = filo.VBool(true)

	orders := &pilotOrders{e: e}
	ctx := context.WithValue(context.Background(), pilotCtxKey{}, orders)
	_, _, err = prog.Execute(ctx, mem, filo.EvalConfig{
		StepLimit: pilotStepLimit,
		Timeout:   pilotTimeout,
	})
	if err != nil {
		return &PilotIssue{Msg: "first tick: " + err.Error()}
	}
	return nil
}

// lineOfParseError turns filo's "parse error at position N" into a 1-based
// line. The position is a byte offset into the source; when the message shape
// changes or carries none, 0 says "no line to point at" and an editor shows
// the message alone.
var parsePosRe = regexp.MustCompile(`position (\d+)`)

func lineOfParseError(src string, err error) int {
	m := parsePosRe.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	pos, aerr := strconv.Atoi(m[1])
	if aerr != nil || pos < 0 {
		return 0
	}
	if pos > len(src) {
		pos = len(src)
	}
	return strings.Count(src[:pos], "\n") + 1
}
