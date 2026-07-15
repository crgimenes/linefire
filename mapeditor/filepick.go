package mapeditor

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/crgimenes/native/filedialog"
	"github.com/hajimehoshi/ebiten/v2"

	"linefire/filoio"
)

// mapPanelExt is the map extension the file panels filter on: filoio.ExtLevel (".lfm")
// without its leading dot, the form filedialog.Options.Extensions wants.
var mapPanelExt = strings.TrimPrefix(filoio.ExtLevel, ".")

// Native OS open/save panels for maps, run off the game loop. filedialog drives the real
// AppKit panels on macOS (cgo-free, via purego) and is a graceful no-op stub on Windows and
// Linux for now (their GTK/Win32 backends land later) — so open/save-as simply cancel there,
// leaving the in-panel asset list and plain S-save as the fallback.
//
// The panels are modal and must run on the MAIN thread; ebiten.RunOnMainThread blocks
// forever if called FROM the main thread, so every dialog is launched on a goroutine (like
// pickAssetsDir) and its result polled back on the game loop.

// openMapDialog shows the OS open panel for a .lfm and returns the chosen path, or "".
func openMapDialog(dir string) string {
	opts := filedialog.Options{Title: "Open map", Directory: dir, Extensions: []string{mapPanelExt}}
	return runDialog(func() string { return filedialog.Open(opts) })
}

// saveMapDialog shows the OS save panel for a .lfm and returns the chosen path, or "".
func saveMapDialog(dir, suggested string) string {
	opts := filedialog.Options{Title: "Save map as", Directory: dir, Filename: suggested, Extensions: []string{mapPanelExt}}
	return runDialog(func() string { return filedialog.Save(opts) })
}

// runDialog invokes a native panel on the main thread on macOS (where the real backend
// lives); elsewhere the stub needs no UI thread, so it runs directly.
func runDialog(show func() string) string {
	if runtime.GOOS != "darwin" {
		return show()
	}
	var path string
	ebiten.RunOnMainThread(func() { path = show() })
	return path
}

// pickOpenMap launches the open panel off the game loop; pollOpenPick applies the choice.
// Ignored if a pick is already in flight.
func (e *MapEditor) pickOpenMap() {
	if e.openResult != nil {
		return
	}
	ch := make(chan string, 1)
	e.openResult = ch
	dir := e.dialogDir()
	go func() { ch <- openMapDialog(dir) }()
	e.status = "open map…"
}

// pollOpenPick hands a finished open choice to the host (which reloads the editor on it).
func (e *MapEditor) pollOpenPick() {
	if e.openResult == nil {
		return
	}
	select {
	case p := <-e.openResult:
		e.openResult = nil
		if p == "" {
			e.status = "open cancelled"
			return
		}
		if e.onOpenMap != nil {
			e.onOpenMap(p)
		}
	default:
	}
}

// pickSaveMapAs launches the save panel off the game loop; pollSavePick writes the map.
func (e *MapEditor) pickSaveMapAs() {
	if e.saveResult != nil {
		return
	}
	ch := make(chan string, 1)
	e.saveResult = ch
	dir := e.dialogDir()
	suggested := suggestedSaveName(e.savePath)
	go func() { ch <- saveMapDialog(dir, suggested) }()
	e.status = "save map as…"
}

// pollSavePick repoints savePath at a finished save-as choice and writes it.
func (e *MapEditor) pollSavePick() {
	if e.saveResult == nil {
		return
	}
	select {
	case p := <-e.saveResult:
		e.saveResult = nil
		if p == "" {
			e.status = "save-as cancelled"
			return
		}
		e.savePath = ensureExt(p, filoio.ExtLevel)
		e.writeSave() // the name is set now; write straight to it
	default:
	}
}

// dialogDir is the directory a file panel opens in: next to the current map if it has a
// path, else the assets folder.
func (e *MapEditor) dialogDir() string {
	if e.savePath != "" {
		return filepath.Dir(e.savePath)
	}
	return e.assetDir
}

// suggestedSaveName is the base name the save panel pre-fills: the current map's stem, or
// "untitled" before it is named. It carries NO extension on purpose — the panel appends the
// one from Options.Extensions, so including ".lfm" here would double it (untitled.lfm.lfm).
func suggestedSaveName(savePath string) string {
	if savePath == "" {
		return "untitled"
	}
	base := filepath.Base(savePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// ensureExt appends ext (with its dot) to p when p does not already end in it, so a
// save-as name without the extension still lands as a .lfm.
func ensureExt(p, ext string) string {
	if strings.EqualFold(filepath.Ext(p), ext) {
		return p
	}
	return p + ext
}
