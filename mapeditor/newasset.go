package mapeditor

import (
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	ui "github.com/crgimenes/minigui"
	"linefire/asset"
	"linefire/filoio"
)

// newAssetKinds are the categories offered when creating an asset, in cycle order.
var newAssetKinds = []string{"enemy", "heal", "shield", "score", "ally", "npc", "player", "decor"}

// New-asset modal geometry (logical pixels), floating over the canvas.
const (
	newAssetX      = 220
	newAssetY      = 110
	newAssetMargin = 8
	newAssetW      = 300
	newAssetH      = 168
)

// openNewAssetDialog shows the modal that creates a new asset file.
func (e *MapEditor) openNewAssetDialog() {
	e.newAssetOpen = true
	e.newAssetName = ""
	if e.newAssetKind == "" {
		e.newAssetKind = newAssetKinds[0]
	}
	e.status = "new asset: pick a kind and name"
}

// closeNewAssetDialog hides the modal and drops its keyboard focus.
func (e *MapEditor) closeNewAssetDialog() {
	e.newAssetOpen = false
	e.newAsset.ClearFocus()
}

// cycleNewAssetKind advances the chosen asset category.
func (e *MapEditor) cycleNewAssetKind() {
	for i, k := range newAssetKinds {
		if k == e.newAssetKind {
			e.newAssetKind = newAssetKinds[(i+1)%len(newAssetKinds)]
			return
		}
	}
	e.newAssetKind = newAssetKinds[0]
}

// runNewAssetDialog drives the modal each frame. Esc cancels; Create writes the
// file and opens it for editing.
func (e *MapEditor) runNewAssetDialog() {
	e.newAsset.Begin(ui.InputFromEbiten(), newAssetX, newAssetY)
	e.newAsset.Label("NEW ASSET")
	if e.newAsset.Button("na.kind", "kind: "+e.newAssetKind) {
		e.cycleNewAssetKind()
	}
	e.newAsset.Label("file name (no extension):")
	e.newAsset.TextField("na.name", &e.newAssetName)
	if e.newAsset.Button("na.create", "Create") {
		e.createNewAsset()
	}
	e.newAsset.SameLine()
	if e.newAsset.Button("na.cancel", "Cancel") {
		e.closeNewAssetDialog()
	}
	e.newAsset.End()
}

// createNewAsset writes a fresh asset of the chosen kind, refreshes the palette,
// and opens it for editing. It refuses to overwrite an existing file.
func (e *MapEditor) createNewAsset() {
	name := strings.TrimSuffix(strings.TrimSpace(e.newAssetName), filoio.ExtAsset)
	if name == "" {
		name = e.newAssetKind
	}
	// Keep the file inside the asset directory: no path separators or traversal.
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		e.status = "invalid file name"
		return
	}
	dir := e.assetDir
	if dir == "" {
		dir = "."
	}
	path := filoio.AssetPath(dir, name)
	_, statErr := os.Stat(path)
	if statErr == nil {
		e.status = "file exists: " + path
		return
	}

	a := asset.New()
	a.Kind = e.newAssetKind
	a.Name = name
	err := filoio.SaveAsset(path, a)
	if err != nil {
		e.status = "new asset error: " + err.Error()
		return
	}

	e.closeNewAssetDialog()
	e.loadPalette()
	if e.onOpenAsset != nil {
		e.onOpenAsset(path)
		return
	}
	e.status = "created " + path
}

// newAssetHovered reports whether the cursor is over the open modal, so canvas
// clicks there do not also place geometry.
func (e *MapEditor) newAssetHovered(mx, my float64) bool {
	if !e.newAssetOpen {
		return false
	}
	x := float64(newAssetX - newAssetMargin)
	y := float64(newAssetY - newAssetMargin)
	return mx >= x && mx < x+newAssetW && my >= y && my < y+newAssetH
}

// drawNewAssetDialog renders the modal backdrop and its ui draw commands.
func (e *MapEditor) drawNewAssetDialog(screen *ebiten.Image) {
	x := float32(newAssetX - newAssetMargin)
	y := float32(newAssetY - newAssetMargin)
	vector.FillRect(screen, x, y, newAssetW, newAssetH, colorPanelBg, false)
	vector.StrokeRect(screen, x, y, newAssetW, newAssetH, 1, colorSelect, false)
	e.newAsset.Render(screen)
}
