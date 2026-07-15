package mapeditor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"linefire/filoio"
)

// playtest saves the open map and launches the game runtime straight onto it, so
// authoring a stage and trying it is one key (F5) instead of a save plus a terminal
// round trip. The map must be named first — an untitled map has nowhere to load from.
func (e *MapEditor) playtest() {
	if e.savePath == "" {
		e.status = "playtest: name the map first (Cmd+S)"
		return
	}
	e.writeSave() // persist the current edits so the launched game sees them
	err := launchPlaytest(e.savePath)
	if err != nil {
		e.status = "playtest: " + err.Error()
		return
	}
	e.status = "playtest: launching " + mapStem(e.savePath)
}

// launchPlaytest starts `go run ./cmd/linefire` on the map at path, resolving assets
// and music the way the shipped game does: content rooted at the directory that holds
// the map's folder (so a sibling music/ still resolves), the map's folder as -mapdir,
// and the file stem as -map. It runs from the module root and does not wait — the game
// opens its own window while the editor keeps running.
func launchPlaytest(mapPath string) error {
	abs, err := filepath.Abs(mapPath)
	if err != nil {
		return err
	}
	mapDir := filepath.Dir(abs)  // e.g. /repo/gameassets
	root := filepath.Dir(mapDir) // e.g. /repo (holds gameassets/ and music/)
	module, err := moduleRoot(mapDir)
	if err != nil {
		return err
	}
	// #nosec G204 -- a dev tool launching the local game on the file being edited
	cmd := exec.Command("go", "run", "./cmd/linefire",
		"-dir", root,
		"-mapdir", filepath.Base(mapDir),
		"-map", mapStem(abs))
	cmd.Dir = module
	cmd.Stdout = os.Stdout // surface the game's build/run output on the editor's console
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

// moduleRoot walks up from dir to the directory holding go.mod (where `go run
// ./cmd/linefire` resolves), erroring if the map is edited outside the repository.
func moduleRoot(dir string) (string, error) {
	for {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// mapStem is a map's file name without its .lfm extension — the name the game and
// portals refer to it by.
func mapStem(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filoio.ExtLevel)
}
