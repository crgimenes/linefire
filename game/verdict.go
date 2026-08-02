package game

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

// The end of a real match, on screen.
//
// Skirmish strips the campaign's presentation — no HUD, no border, no keys —
// because the overlay lives on somebody's desktop. The verdict is the one
// exception, and it earns it: a battle that is decided and shows nothing is a
// battle whose ending the spectator misses entirely. It used to deal itself a
// rematch after five seconds, which is exactly how the ending got missed.
//
// It is drawn in the campaign's own title style (titleText: the CRT-cyan scaled
// glyph with a dark outline, the same one GAME OVER and the victory screens
// use), because a player who knows linefire must recognise this — the standing
// rule that the campaign is the source of look and behaviour.
//
// The winner is named by FACTION, and colour says faction here as everywhere
// else in skirmish, so the fleet's own hue carries the announcement.
func (g *Game) drawVerdict(dst *ebiten.Image) {
	if !g.skirmishMode || g.match == nil {
		return
	}
	winner, over := g.match.Over()
	if !over {
		return
	}
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cx, cy := float64(w)/2, float64(h)*0.42

	if winner == 0 {
		g.titleText(dst, "DRAW", cx, cy, 3)
		return
	}
	g.titleFactionText(dst, fmt.Sprintf("FACTION %d", winner), cx, cy, 3, winner)
	g.titleText(dst, "PREVAILS", cx, cy+float64(h)*0.09, 2)
}
