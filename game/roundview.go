package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var (
	roundMaskColor   = color.RGBA{0x00, 0x00, 0x00, 0xff} // fully black outside the circle
	roundBorderColor = color.RGBA{0x80, 0xff, 0xff, 0xff} // cyan, matching the wall/HUD borders
)

// drawRoundMask masks the view to a centered circle ("porthole"): opaque black
// outside, transparent inside, with a cyan ring at the edge — so the rectangular
// frame reads as a round screen.
func (g *Game) drawRoundMask(screen *ebiten.Image) {
	cx, cy := float64(g.sw)/2, float64(g.sh)/2
	bw := 3 * g.deviceScale()
	r := math.Min(float64(g.sw), float64(g.sh))/2 - 2*bw
	if r <= 0 {
		return
	}

	// Opaque black with the circle punched transparent.
	mask := g.ensureFogMask()
	mask.Fill(roundMaskColor)
	var circle vector.Path
	circle.Arc(float32(cx), float32(cy), float32(r), 0, 2*math.Pi, vector.Clockwise)
	circle.Close()
	vector.FillPath(mask, &circle, &vector.FillOptions{}, &vector.DrawPathOptions{
		Blend:     ebiten.BlendDestinationOut,
		AntiAlias: true,
	})
	screen.DrawImage(mask, nil)

	vector.StrokeCircle(screen, float32(cx), float32(cy), float32(r), float32(bw), roundBorderColor, true)
}
