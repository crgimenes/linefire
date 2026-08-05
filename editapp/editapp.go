// Package editapp hosts the Linefire map and asset editors in a single window.
// Ebitengine is single-window per process, so "open the asset editor in another
// window" is a mode switch: double-clicking an asset in the map editor (or
// creating one) swaps the active editor to the asset editor, and its Back button
// (or Esc) returns to the map. The map editor is kept alive across the switch.
package editapp

import (
	"errors"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/crgimenes/linefire/editor"
	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/game"
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
	playing  bool // the active mode is a playtest, not an editor
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
	ed.SetOnPlaytest(a.playtest)
	return ed
}

// playtest stages the game on the map being edited — the same window, the same frame
// budget, no save and no rebuild. lvl is the editor's clone, so the session is free to
// blow up walls and spend pickups without touching the document.
func (a *App) playtest(lvl *level.Level, name string) {
	player, err := filoio.LoadAsset(filoio.AssetPath(a.assetDir, "player"))
	if err != nil {
		return // no player asset next to the map: stay in the editor
	}
	a.pending = game.NewPlaytest(player, lvl, a.assetDir, name)
	a.playing = true
	title := "Linefire Playtest — F5 returns to editing"
	if name != "" {
		title = "Linefire Playtest: " + name + " — F5 returns to editing"
	}
	ebiten.SetWindowTitle(title)
}

// stopPlaytest stages the return to the map editor, exactly as it was left.
func (a *App) stopPlaytest() {
	a.playing = false
	a.pending = a.mapEd
	a.mapEd.ResetTitle()
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
	// F5 is the whole loop: it starts a playtest from the editor and ends one from
	// inside the game. Handled here rather than in the game so it works from any game
	// state (paused, dead, mid-stage) — the editor is always one key away.
	if a.playing && inpututil.IsKeyJustPressed(ebiten.KeyF5) {
		a.stopPlaytest()
		return nil
	}
	err := a.cur.Update()
	if a.playing && errors.Is(err, ebiten.Termination) {
		// A playtest ends its loop for two very different reasons. Quitting from
		// inside the game means "back to the editor". The WINDOW closing means the
		// window closes — swallowing that would leave the app impossible to quit.
		if ebiten.IsWindowBeingClosed() {
			a.mapEd.Persist() // the editor's own close autosave never ran this frame
			return err
		}
		a.stopPlaytest()
		return nil
	}
	return err
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
