package mapeditor

import (
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/crgimenes/linefire/render"
	ui "github.com/crgimenes/minigui"
)

// Wall color popup geometry (logical pixels), pinned to the top-left of the canvas.
const (
	colorPanelX      = 12
	colorPanelY      = toolbarHeight + 12
	colorPanelMargin = 8
	colorPanelW      = 200
	colorPanelH      = 150
)

// toggleColorPanel shows or hides the wall color picker.
func (e *MapEditor) toggleColorPanel() {
	e.colorOpen = !e.colorOpen
	e.status = "wall color " + onOff(e.colorOpen)
}

// runColorPanel drives the VGA palette for the wall layer from live input.
func (e *MapEditor) runColorPanel() {
	e.colorPanelWith(ui.InputFromEbiten())
}

// colorPanelWith is the testable core of runColorPanel: picking a stroke or fill
// recolors the walls (the glow derives from the stroke color), then closes the
// popup. Mirrors the asset editor's color panel, reusing ui.VGAPalette.
func (e *MapEditor) colorPanelWith(in ui.Input) {
	layer := e.currentWallLayer()
	e.colors.Begin(in, colorPanelX, colorPanelY)
	e.colors.Label("WALL COLOR (L)")

	e.colors.Label("stroke:")
	e.swatchRow("s", layer.Stroke, func(hex string) {
		layer.Stroke = hex
		e.afterColorPick("stroke " + hex)
	})

	e.colors.Label("fill:")
	e.swatchRow("f", layer.Fill, func(hex string) {
		layer.Fill = hex
		e.afterColorPick("fill " + hex)
	})
	if e.colors.Button("f.none", "transparent") {
		layer.Fill = "transparent"
		e.afterColorPick("fill transparent")
	}

	e.colors.End()
}

// swatchRow lays the 16 VGA colors out in two rows of eight, calling set with the
// chosen hex; the swatch matching current is highlighted.
func (e *MapEditor) swatchRow(prefix, current string, set func(string)) {
	for i, hex := range ui.VGAPalette {
		col, _ := render.ParseColor(hex)
		id := ui.ID(prefix + strconv.Itoa(i))
		if e.colors.Swatch(id, col, strings.EqualFold(hex, current)) {
			set(hex)
		}
		if (i+1)%8 != 0 {
			e.colors.SameLine()
		}
	}
}

// afterColorPick records the change and closes the popup so it does not linger.
func (e *MapEditor) afterColorPick(status string) {
	e.markDirty()
	e.colorOpen = false
	e.status = status
}

// colorPanelHovered reports whether the cursor is over the open color popup, so
// canvas clicks there do not also place geometry.
func (e *MapEditor) colorPanelHovered(mx, my float64) bool {
	if !e.colorOpen {
		return false
	}
	x := float64(colorPanelX - colorPanelMargin)
	y := float64(colorPanelY - colorPanelMargin)
	return mx >= x && mx < x+colorPanelW && my >= y && my < y+colorPanelH
}

// drawColorPanel renders the popup backdrop and its ui draw commands.
func (e *MapEditor) drawColorPanel(screen *ebiten.Image) {
	x := float32(colorPanelX - colorPanelMargin)
	y := float32(colorPanelY - colorPanelMargin)
	vector.FillRect(screen, x, y, colorPanelW, colorPanelH, colorPanelBg, false)
	vector.StrokeRect(screen, x, y, colorPanelW, colorPanelH, 1, colorAxis, false)
	e.colors.Render(screen)
}
