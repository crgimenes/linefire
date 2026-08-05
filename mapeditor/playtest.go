package mapeditor

import (
	"path/filepath"
	"strings"

	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/level"
)

// SetOnPlaytest wires the host's play-in-this-window switch (see editapp). Without it
// F5 reports that playtesting is unavailable rather than doing something surprising.
func (e *MapEditor) SetOnPlaytest(fn func(lvl *level.Level, name string)) {
	e.onPlaytest = fn
}

// playtest hands the map being edited to the host to play right here, in this window.
// It does NOT save first and does not need a named file: the whole point is trying a
// change the instant it is made, so the loop is one key each way and an experiment can
// be undone instead of persisted. The host gets a clone, so the play session can never
// disturb the document under edit.
func (e *MapEditor) playtest() {
	if e.onPlaytest == nil {
		e.status = "playtest: unavailable (no host)"
		return
	}
	name := mapStem(e.savePath) // "" while the map is still untitled
	e.onPlaytest(e.level.Clone(), name)
	e.status = "playtest: Esc for the menu, then Back to editor"
}

// mapStem is a map's file name without its .lfm extension — the name the game and
// portals refer to it by. Empty for an unsaved map.
func mapStem(path string) string {
	if path == "" {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(path), filoio.ExtLevel)
}
