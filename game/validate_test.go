package game

import (
	"strings"
	"testing"
)

// A well-formed program that leans on the whole contract surface must validate
// clean. (The skirmish repo validates the real reference program, hunter.filo,
// against this same function — it owns that file.)
func TestValidatePilotAcceptsAHealthyProgram(t *testing.T) {
	src := `
; a healthy program: instruments, memory, math, orders
(if first-tick (def waypoint (floor (/ self-x 500))))
(if (is-empty enemies)
    (seek 200 200)
    (let ((tgt (head enemies)))
      (do
        (face (bearing self-x self-y (nth tgt 1) (nth tgt 2)))
        (fire (nth tgt 1) (nth tgt 2)))))
`
	if issue := ValidatePilot(src); issue != nil {
		t.Fatalf("a healthy program failed validation: line %d: %s", issue.Line, issue.Msg)
	}
}

// A parse error points at ITS line — that is the whole reason the editor calls
// this instead of the player reading a byte offset out of a terminal.
func TestValidatePilotPointsAtTheBrokenLine(t *testing.T) {
	issue := ValidatePilot("(def a 1)\n(def b 2)\n(broken\n")
	if issue == nil {
		t.Fatal("an unterminated list validated clean")
	}
	if issue.Line != 3 {
		t.Errorf("the issue points at line %d, want 3 (msg: %s)", issue.Line, issue.Msg)
	}
}

// Filo only meets an undefined global at runtime; the dry tick is what drags
// that forward to the editor. No line to point at — runtime errors carry none.
func TestValidatePilotCatchesRuntimeErrorsOnTheDryTick(t *testing.T) {
	issue := ValidatePilot("(this-builtin-does-not-exist self-x)")
	if issue == nil {
		t.Fatal("an undefined global validated clean")
	}
	if !strings.Contains(issue.Msg, "first tick") {
		t.Errorf("the message should say the dry tick caught it: %s", issue.Msg)
	}
}

// The dry tick runs with populated instruments: a program that reads contacts
// and loot on tick one must pass, not trip over empty lists.
func TestValidatePilotPopulatesTheInstruments(t *testing.T) {
	src := `
(if first-tick (def target (head enemies)))
(seek (nth target 1) (nth target 2))
(fire (nth (head loot) 1) (nth (head loot) 2))
`
	if issue := ValidatePilot(src); issue != nil {
		t.Fatalf("a program using enemies and loot failed: line %d: %s", issue.Line, issue.Msg)
	}
}
