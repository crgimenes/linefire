package mapeditor

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/crgimenes/native/filedialog"

	"linefire/asset"
	"linefire/level"
	"linefire/svgimport"
)

// SVG import: pick a drawing with the native panel and drop its shapes into the open map's
// wall layer, so a level's geometry can be drawn in Inkscape/Figma and finished here. The
// walls are APPENDED (header, spawns and existing walls are kept) and the whole thing is one
// undo step. Same off-the-loop pattern as the open/save panels (see filepick.go).

// openSVGDialog shows the OS open panel filtered to .svg and returns the path, or "".
func openSVGDialog(dir string) string {
	opts := filedialog.Options{Title: "Import SVG", Directory: dir, Extensions: []string{"svg"}}
	return runDialog(func() string { return filedialog.Open(opts) })
}

// pickImportSVG launches the SVG open panel off the game loop; pollSVGImport applies it.
func (e *MapEditor) pickImportSVG() {
	if e.svgResult != nil {
		return
	}
	ch := make(chan string, 1)
	e.svgResult = ch
	dir := e.dialogDir()
	go func() { ch <- openSVGDialog(dir) }()
	e.status = "import SVG…"
}

// pollSVGImport applies a finished SVG choice on the game loop.
func (e *MapEditor) pollSVGImport() {
	if e.svgResult == nil {
		return
	}
	select {
	case p := <-e.svgResult:
		e.svgResult = nil
		if p == "" {
			e.status = "SVG import cancelled"
			return
		}
		e.importSVGFile(p)
	default:
	}
}

// importSVGFile reads the SVG at path and merges it into the open map: a shape marked with a
// known label becomes a SPAWN at its center (a palette asset by name, "portal[:target]", or
// "player-start"), and every other shape becomes a WALL. Everything is appended (header and
// existing content are kept) and dirtied as one undo step.
func (e *MapEditor) importSVGFile(path string) {
	data, err := os.ReadFile(path) // #nosec G304 -- the user chose this file in the OS open panel
	if err != nil {
		e.status = "SVG read error: " + err.Error()
		return
	}
	shapes, err := svgimport.Shapes(data)
	if err != nil {
		e.status = "SVG import error: " + err.Error()
		return
	}
	layer := e.currentWallLayer()
	walls, spawns, starts := 0, 0, 0
	for i := range shapes {
		s := &shapes[i]
		c := svgimport.Centroid(s.Points)
		switch {
		case s.Label == "player-start":
			e.level.PlayerStart = level.Start{X: math.Round(c.X), Y: math.Round(c.Y), Angle: -90}
			starts++
		case e.placeSpawnFromLabel(s.Label, c):
			spawns++
		default:
			layer.Paths = append(layer.Paths, svgWallPath(s.Points))
			walls++
		}
	}
	e.markDirty()
	e.status = fmt.Sprintf("SVG %s: %d walls, %d spawns, %d start", filepath.Base(path), walls, spawns, starts)
}

// placeSpawnFromLabel drops a spawn at c for a recognized marker label — a portal (bare
// "portal" or "portal:target") or a palette asset's name — and reports whether it matched.
// An empty or unknown label matches nothing, so the caller makes the shape a wall.
func (e *MapEditor) placeSpawnFromLabel(label string, c asset.Point) bool {
	if label == "" {
		return false
	}
	x, y := math.Round(c.X), math.Round(c.Y)
	if label == "portal" || strings.HasPrefix(label, "portal:") {
		target := ""
		if after, ok := strings.CutPrefix(label, "portal:"); ok {
			target = after
		}
		e.level.Spawns = append(e.level.Spawns, level.Spawn{
			Name: fmt.Sprintf("portal_%d", len(e.level.Spawns)+1), Kind: "portal",
			Target: target, X: x, Y: y, Angle: -90,
		})
		return true
	}
	entry, ok := e.entryByName(label)
	if !ok {
		return false
	}
	kind := assetKind(entry)
	e.level.Spawns = append(e.level.Spawns, level.Spawn{
		Name:  fmt.Sprintf("%s_%d", kind, len(e.level.Spawns)+1),
		Asset: entry.ref, Kind: kind, X: x, Y: y, Angle: -90,
	})
	return true
}

// entryByName finds a palette entry by its display name (the SVG marker label uses it).
func (e *MapEditor) entryByName(name string) (paletteEntry, bool) {
	for i := range e.palette {
		if e.palette[i].name == name {
			return e.palette[i], true
		}
	}
	return paletteEntry{}, false
}

// svgWallPath turns an SVG outline into a closed wall path, snapping each coordinate to a
// whole world unit — walls are coarse, and it trims the curve-flattening float noise.
func svgWallPath(pts []asset.Point) asset.Path {
	cmds := make([]asset.Command, 0, len(pts)+2)
	cmds = append(cmds, asset.Command{Op: asset.OpMoveTo, X: math.Round(pts[0].X), Y: math.Round(pts[0].Y)})
	for _, p := range pts[1:] {
		cmds = append(cmds, asset.Command{Op: asset.OpLineTo, X: math.Round(p.X), Y: math.Round(p.Y)})
	}
	cmds = append(cmds, asset.Command{Op: asset.OpClose})
	return asset.Path{Commands: cmds}
}
