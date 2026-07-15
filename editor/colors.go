package editor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	ui "github.com/crgimenes/minigui"
	"linefire/render"
)

// Color panel geometry (logical pixels), below the hardpoints panel.
const (
	colorPanelX      = 12
	colorPanelY      = 300
	colorPanelMargin = 8
	colorPanelW      = 200
	colorPanelH      = 290
)

// toggleColorPanel shows or hides the color palette panel; opening it closes the
// other left-slot panels.
func (e *Editor) toggleColorPanel() {
	e.colorPanel = !e.colorPanel
	if e.colorPanel {
		e.hpPanel = false
		e.sndPanel = false
	}
	e.status = "colors panel " + onOff(e.colorPanel)
}

// runColorPanel drives the VGA palette: rows of swatches that set the current
// layer's stroke and fill, plus a transparent-fill option.
func (e *Editor) runColorPanel() {
	in := ui.InputFromEbiten()
	e.colors.Begin(in, colorPanelX, colorPanelY)
	e.colors.Label("COLORS (L)")

	// Color is per layer: show which layer is being painted and let the user move
	// between layers (or add one), so a multi-color ship is made of several layers.
	layer := e.currentLayer()
	name := layer.Name
	if name == "" {
		name = "(unnamed)"
	}
	e.colors.Label(fmt.Sprintf("layer %d/%d: %s", e.layerIdx+1, len(e.asset.Layers), name))
	if e.colors.Button("lyr.prev", "<") {
		e.selectLayer(-1)
	}
	e.colors.SameLine()
	if e.colors.Button("lyr.next", ">") {
		e.selectLayer(1)
	}
	e.colors.SameLine()
	if e.colors.Button("lyr.add", "+layer") {
		e.addLayer()
	}

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

// swatchRow lays the 16 VGA colors out in two rows of eight, calling set with
// the chosen hex; the swatch matching current is highlighted.
func (e *Editor) swatchRow(prefix, current string, set func(string)) {
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

// afterColorPick records a color change and closes the panel, so it does not
// stay on screen after a swatch is chosen.
func (e *Editor) afterColorPick(status string) {
	e.markDirty()
	e.syncPaletteIndices()
	e.colorPanel = false
	e.status = status
}

// colorPanelHovered reports whether the cursor is over the open color panel.
func (e *Editor) colorPanelHovered(mx, my float64) bool {
	if !e.colorPanel {
		return false
	}
	x := float64(colorPanelX - colorPanelMargin)
	y := float64(colorPanelY - colorPanelMargin)
	return mx >= x && mx < x+colorPanelW && my >= y && my < y+colorPanelH
}

// drawColorPanel renders the panel backdrop and the ui draw commands.
func (e *Editor) drawColorPanel(screen *ebiten.Image) {
	x := float32(colorPanelX - colorPanelMargin)
	y := float32(colorPanelY - colorPanelMargin)
	vector.FillRect(screen, x, y, colorPanelW, colorPanelH, colorPanelBg, false)
	vector.StrokeRect(screen, x, y, colorPanelW, colorPanelH, 1, colorBox, false)
	e.colors.Render(screen)
}
