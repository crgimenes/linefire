// Package editapp hosts the Linefire map and asset editors in a single window.
// Ebitengine is single-window per process, so "open the asset editor in another
// window" is a mode switch: double-clicking an asset in the map editor (or
// creating one) swaps the active editor to the asset editor, and its Back button
// (or Esc) returns to the map. The map editor is kept alive across the switch.
package editapp

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/crgimenes/linefire/editor"
	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/mapeditor"
)

const (
	winWidth  = 1100
	winHeight = 720
)

// App is the ebiten.Game that delegates each frame to whichever editor is active.
// A mode switch requested mid-Update (a double-click, New asset, or Back) is
// staged in pending and applied at the next frame boundary, so the active editor
// always finishes its frame as the active editor (no detached half-frame).
type App struct {
	mapEd    *mapeditor.MapEditor
	assetDir string // scanned for placeable assets/maps; reused for opened maps
	cur      ebiten.Game
	pending  ebiten.Game
}

// New builds the host around a map editor and wires the mode switches.
func New(lvl *level.Level, savePath, assetDir string) *App {
	app := &App{assetDir: assetDir}
	app.mapEd = app.newMapEditor(lvl, savePath)
	app.cur = app.mapEd
	return app
}

// newMapEditor builds a map editor wired to the host's mode switches.
func (a *App) newMapEditor(lvl *level.Level, savePath string) *mapeditor.MapEditor {
	ed := mapeditor.New(lvl, savePath, a.assetDir)
	ed.SetOnOpenAsset(a.openAsset)
	ed.SetOnOpenMap(a.openMap)
	return ed
}

// openAsset stages a switch to the asset editor for path; a failed load is ignored
// (the map editor's own status already reflects the click).
func (a *App) openAsset(path string) {
	doc, err := filoio.LoadAsset(path)
	if err != nil {
		return
	}
	ed := editor.New(doc, path)
	ed.SetOnBack(a.backToMap)
	a.pending = ed
}

// openMap stages a switch to a different map: the current map is saved, then the
// map editor is rebuilt on the chosen level (it becomes the new home).
func (a *App) openMap(path string) {
	doc, err := filoio.LoadLevel(path)
	if err != nil {
		return
	}
	a.mapEd.Persist()
	a.mapEd = a.newMapEditor(doc, path)
	a.pending = a.mapEd
}

// backToMap stages a return to the map editor, refreshing its palette to pick up
// edits or newly created assets.
func (a *App) backToMap() {
	a.mapEd.ReloadPalette()
	a.mapEd.ResetTitle() // the asset editor changed the window title; refresh it
	a.pending = a.mapEd
}

// applyPending swaps in a staged editor at a frame boundary.
func (a *App) applyPending() {
	if a.pending != nil {
		a.cur = a.pending
		a.pending = nil
	}
}

// Update applies any staged mode switch at the frame boundary, then advances the
// active editor. Staging the swap (rather than switching mid-Update) keeps each
// frame's Update and Draw on the same editor.
func (a *App) Update() error {
	a.applyPending()
	return a.cur.Update()
}

// Draw renders the active editor.
func (a *App) Draw(screen *ebiten.Image) { a.cur.Draw(screen) }

// Layout returns the active editor's logical resolution.
func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	return a.cur.Layout(outsideWidth, outsideHeight)
}

// Run opens the single window and starts the game loop.
func Run(lvl *level.Level, savePath, assetDir string) error {
	ebiten.SetWindowSize(winWidth, winHeight)
	ebiten.SetWindowTitle("Linefire Edit")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowClosingHandled(true)
	ebiten.SetRunnableOnUnfocused(true) // keep ticking when unfocused so blur autosave fires
	return ebiten.RunGame(New(lvl, savePath, assetDir))
}
