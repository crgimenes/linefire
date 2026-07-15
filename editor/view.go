package editor

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"linefire/asset"
)

// handleView processes zooming (mouse wheel), panning (middle button, or space
// plus left button) and the framing shortcuts. It runs every frame regardless
// of modifier keys so navigation always works.
func (e *Editor) handleView(mx, my float64, space bool) {
	_, wy := ebiten.Wheel()
	if wy != 0 && e.mouseInCanvas {
		e.cam.ZoomAt(mx, my, math.Pow(1.1, wy))
	}

	e.panning = ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) ||
		(space && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft))
	if e.panning {
		e.cam.Pan(mx-e.lastMouseX, my-e.lastMouseY)
		e.dragging = false // panning takes over from a handle drag
	}

	if inpututil.IsKeyJustPressed(ebiten.Key0) {
		e.resetView()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		e.frameContent()
	}

	// Glow: B toggles the bloom, Shift+B cycles the variant.
	if inpututil.IsKeyJustPressed(ebiten.KeyB) {
		shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
		if shift {
			e.glowVariant = (e.glowVariant + 1) % 3
			e.status = "glow variant: " + e.glowVariant.String()
		} else {
			e.glowEnabled = !e.glowEnabled
			e.status = "glow " + onOff(e.glowEnabled)
		}
	}

	// K toggles the CRT post-process on the preview.
	if inpututil.IsKeyJustPressed(ebiten.KeyK) {
		e.crtEnabled = !e.crtEnabled
		e.status = "crt " + onOff(e.crtEnabled)
	}
}

// resetView frames the asset's size box.
func (e *Editor) resetView() {
	e.cam.FitBox(e.canvasRect(), 0, 0, e.asset.Size.W, e.asset.Size.H)
	e.status = "view reset"
}

// frameContent frames the drawn geometry; with no geometry it frames the size
// box instead.
func (e *Editor) frameContent() {
	minX, minY, maxX, maxY, ok := e.contentBounds()
	if !ok {
		e.resetView()
		return
	}
	e.cam.FitBox(e.canvasRect(), minX, minY, maxX, maxY)
	e.status = "framed content"
}

// contentBounds returns the bounding box of all path vertices on visible
// layers. ok is false when there is nothing drawn.
func (e *Editor) contentBounds() (minX, minY, maxX, maxY float64, ok bool) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for li := range e.asset.Layers {
		if e.asset.Layers[li].Hidden {
			continue
		}
		for _, p := range e.asset.Layers[li].Paths {
			for _, c := range p.Commands {
				if c.Op == asset.OpClose {
					continue
				}
				ok = true
				minX = math.Min(minX, c.X)
				minY = math.Min(minY, c.Y)
				maxX = math.Max(maxX, c.X)
				maxY = math.Max(maxY, c.Y)
			}
		}
	}
	return minX, minY, maxX, maxY, ok
}
