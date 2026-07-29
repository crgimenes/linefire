package mapeditor

import (
	"fmt"
	"image"
	_ "image/gif"  // decode .gif reference images
	_ "image/jpeg" // decode .jpg/.jpeg reference images
	_ "image/png"  // decode .png reference images
	"math"
	"os"

	"github.com/crgimenes/native/filedialog"
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/render"
)

// Reference backdrop: a raster template (e.g. a screenshot of a classic arcade map) shown behind
// the canvas so a level can be traced over it. It is ONLY a drawing aid — kept in memory, never
// part of the level, so it never touches the .lfm. Same off-the-loop native-panel pattern as the
// SVG import (see importsvg.go / filepick.go).

// backdropDefaultAlpha is the starting opacity: faint enough that wall lines read clearly on top.
const backdropDefaultAlpha = 0.4

// openImageDialog shows the OS open panel filtered to raster images and returns the path, or "".
func openImageDialog(dir string) string {
	opts := filedialog.Options{Title: "Reference image", Directory: dir, Extensions: []string{"png", "jpg", "jpeg", "gif"}}
	return runDialog(func() string { return filedialog.Open(opts) })
}

// pickBackdrop launches the image open panel off the game loop; pollBackdrop applies the choice.
func (e *MapEditor) pickBackdrop() {
	if e.imgResult != nil {
		return
	}
	ch := make(chan string, 1)
	e.imgResult = ch
	dir := e.dialogDir()
	go func() { ch <- openImageDialog(dir) }()
	e.status = "reference image…"
}

// pollBackdrop applies a finished image choice on the game loop.
func (e *MapEditor) pollBackdrop() {
	if e.imgResult == nil {
		return
	}
	select {
	case p := <-e.imgResult:
		e.imgResult = nil
		if p == "" {
			e.status = "reference image cancelled"
			return
		}
		e.loadBackdrop(p)
	default:
	}
}

// loadBackdrop decodes the image and fits it over the map's Size box, visible at a low opacity.
func (e *MapEditor) loadBackdrop(path string) {
	f, err := os.Open(path) // #nosec G304 -- the user chose this file in the OS open panel
	if err != nil {
		e.status = "image open error: " + err.Error()
		return
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		e.status = "image decode error: " + err.Error()
		return
	}
	e.backdrop = ebiten.NewImageFromImage(img)
	e.backdropAlpha = backdropDefaultAlpha
	e.fitBackdrop()
	e.backdropVisible = true
	b := e.backdrop.Bounds()
	e.status = fmt.Sprintf("reference %dx%d (Shift+M hide · -/= scale · ,/. fade)", b.Dx(), b.Dy())
}

// fitBackdrop scales and centers the reference over the level's Size box, preserving aspect ratio.
func (e *MapEditor) fitBackdrop() {
	if e.backdrop == nil {
		return
	}
	b := e.backdrop.Bounds()
	iw, ih := float64(b.Dx()), float64(b.Dy())
	if iw <= 0 || ih <= 0 {
		return
	}
	w, h := e.level.Size.W, e.level.Size.H
	if w <= 0 || h <= 0 {
		w, h = 1000, 1000
	}
	s := math.Min(w/iw, h/ih) // world units per image pixel to fit inside the Size box
	e.backdropScale = s
	e.backdropPos = asset.Point{X: (w - iw*s) / 2, Y: (h - ih*s) / 2} // centered
}

// scaleBackdrop multiplies the reference's scale, keeping its center fixed so it grows/shrinks in
// place rather than drifting toward the origin.
func (e *MapEditor) scaleBackdrop(factor float64) {
	if e.backdrop == nil {
		return
	}
	b := e.backdrop.Bounds()
	iw, ih := float64(b.Dx()), float64(b.Dy())
	cx := e.backdropPos.X + iw*e.backdropScale/2
	cy := e.backdropPos.Y + ih*e.backdropScale/2
	e.backdropScale *= factor
	e.backdropPos = asset.Point{X: cx - iw*e.backdropScale/2, Y: cy - ih*e.backdropScale/2}
	e.status = fmt.Sprintf("reference scale %.2f", e.backdropScale)
}

// fadeBackdrop nudges the reference opacity, clamped so it never fully vanishes or blows out.
func (e *MapEditor) fadeBackdrop(d float64) {
	e.backdropAlpha = math.Min(1, math.Max(0.1, e.backdropAlpha+d))
	e.status = fmt.Sprintf("reference opacity %.0f%%", e.backdropAlpha*100)
}

// drawBackdrop paints the reference image behind the walls, in world space so it pans and zooms
// with the map. A no-op until one is loaded and shown.
func (e *MapEditor) drawBackdrop(canvas *ebiten.Image, view render.View) {
	if e.backdrop == nil || !e.backdropVisible {
		return
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(view.Scale*e.backdropScale, view.Scale*e.backdropScale)
	sx, sy := view.Project(e.backdropPos.X, e.backdropPos.Y)
	op.GeoM.Translate(float64(sx), float64(sy))
	op.ColorScale.ScaleAlpha(float32(e.backdropAlpha))
	canvas.DrawImage(e.backdrop, op)
}
