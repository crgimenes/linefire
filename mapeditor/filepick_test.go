package mapeditor

import (
	"path/filepath"
	"testing"

	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/level"
)

// TestSavePathLabelUntitled: an unnamed map reads "untitled" in the status bar, a named one
// shows its path.
func TestSavePathLabelUntitled(t *testing.T) {
	if got := (&MapEditor{}).savePathLabel(); got != "untitled" {
		t.Fatalf("an unnamed map should read \"untitled\", got %q", got)
	}
	if got := (&MapEditor{savePath: "levels/a.lfm"}).savePathLabel(); got != "levels/a.lfm" {
		t.Fatalf("a named map should show its path, got %q", got)
	}
}

// TestSaveOnUntitledPromptsInsteadOfWriting: Cmd+S on an untitled map must route to the Save
// As panel, never invent a path or clear dirty. (A save-as is pre-marked in flight so the
// panel goroutine is a no-op and the test stays headless-safe.)
func TestSaveOnUntitledPromptsInsteadOfWriting(t *testing.T) {
	e := &MapEditor{level: level.New(), savePath: "", dirty: true}
	e.saveResult = make(chan string, 1) // a pick is "already in flight": pickSaveMapAs no-ops
	e.save()
	if e.savePath != "" {
		t.Fatal("saving an untitled map must not invent a path")
	}
	if !e.dirty {
		t.Fatal("nothing was written, so dirty must stay set")
	}
}

// TestAutoSaveSkipsUntitled: a blur must never pop a modal — autosave writes only a NAMED map.
func TestAutoSaveSkipsUntitled(t *testing.T) {
	e := &MapEditor{level: level.New(), savePath: "", dirty: true}
	e.autoSave()
	if !e.dirty {
		t.Fatal("autosave on an untitled map must be a no-op (dirty stays set)")
	}
	if e.saveResult != nil {
		t.Fatal("autosave must not open the save panel")
	}
}

// TestPollSavePickWritesToChosenPath: a finished Save As names the map and writes it, and the
// result loads back through filoio.
func TestPollSavePickWritesToChosenPath(t *testing.T) {
	e := &MapEditor{level: level.New(), dirty: true}
	tmp := filepath.Join(t.TempDir(), "chosen.lfm")
	ch := make(chan string, 1)
	ch <- tmp
	e.saveResult = ch

	e.pollSavePick()

	if e.savePath != tmp {
		t.Fatalf("save-as should repoint savePath to %q, got %q", tmp, e.savePath)
	}
	if e.dirty {
		t.Fatal("a written map is no longer dirty")
	}
	if _, err := filoio.LoadLevel(tmp); err != nil {
		t.Fatalf("the saved map should load back: %v", err)
	}
}

// TestSuggestedSaveNameHasNoExtension is the fix for "untitled.lfm.lfm": the save panel
// appends the extension itself, so the pre-filled name must be the bare stem.
func TestSuggestedSaveNameHasNoExtension(t *testing.T) {
	if got := suggestedSaveName(""); got != "untitled" {
		t.Fatalf("an untitled map should suggest \"untitled\" (no ext), got %q", got)
	}
	if got := suggestedSaveName("levels/map0001.lfm"); got != "map0001" {
		t.Fatalf("a named map should suggest its stem without the extension, got %q", got)
	}
}

func TestEnsureExt(t *testing.T) {
	cases := []struct{ in, ext, want string }{
		{"level", ".lfm", "level.lfm"},
		{"level.lfm", ".lfm", "level.lfm"},
		{"level.LFM", ".lfm", "level.LFM"}, // already has it (case-insensitive)
		{"maps/stage1", ".lfm", "maps/stage1.lfm"},
	}
	for _, c := range cases {
		if got := ensureExt(c.in, c.ext); got != c.want {
			t.Errorf("ensureExt(%q, %q) = %q, want %q", c.in, c.ext, got, c.want)
		}
	}
}

func TestDialogDirPrefersMapFolder(t *testing.T) {
	e := &MapEditor{savePath: "levels/stage1.lfm", assetDir: "assets"}
	if got := e.dialogDir(); got != "levels" {
		t.Fatalf("with a save path the panel should open next to the map, got %q", got)
	}
	e = &MapEditor{savePath: "", assetDir: "assets"}
	if got := e.dialogDir(); got != "assets" {
		t.Fatalf("without a save path the panel should open in the assets dir, got %q", got)
	}
}

// TestPollOpenPickHandsPathToHost: a finished open choice is routed to the onOpenMap host
// callback (which reloads the editor), and the in-flight channel is cleared.
func TestPollOpenPickHandsPathToHost(t *testing.T) {
	e := &MapEditor{}
	ch := make(chan string, 1)
	ch <- "maps/picked.lfm"
	e.openResult = ch

	var got string
	e.onOpenMap = func(p string) { got = p }
	e.pollOpenPick()

	if got != "maps/picked.lfm" {
		t.Fatalf("the host should be handed the chosen path, got %q", got)
	}
	if e.openResult != nil {
		t.Fatal("the in-flight open channel should be cleared after applying")
	}
}

// TestPollOpenPickCancelDoesNotOpen: an empty choice (the user cancelled) must not call the
// host.
func TestPollOpenPickCancelDoesNotOpen(t *testing.T) {
	e := &MapEditor{}
	ch := make(chan string, 1)
	ch <- ""
	e.openResult = ch

	called := false
	e.onOpenMap = func(string) { called = true }
	e.pollOpenPick()

	if called {
		t.Fatal("cancelling the open panel must not reload a map")
	}
	if e.openResult != nil {
		t.Fatal("the channel should be cleared even on cancel")
	}
}
