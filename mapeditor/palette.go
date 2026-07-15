package mapeditor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"linefire/asset"
	"linefire/filoio"
)

// paletteEntry is a placeable thing discovered on disk: either a saved asset
// (placed as a spawn of its declared kind) or a map (placed as a portal to it).
// The kind is intrinsic — the asset declares what it is, and a map is a map — so
// placement routes itself without the user choosing a role.
type paletteEntry struct {
	ref    string       // asset REFERENCE written into the spawn (the portal visual, for maps)
	file   string       // the entry's own document (asset or level), opened on double-click
	name   string       // display name; for maps, the destination map stem (the portal target)
	asset  *asset.Asset // the asset, or a visual used for the thumbnail (portal/diamond)
	isMap  bool         // true => placing creates a portal to `name`
	isExit bool         // true => placing creates a named arrival point (portal exit)
}

// exitThumb is the diamond drawn for the "portal exit" list item, matching the
// in-canvas entry marker.
var exitThumb = &asset.Asset{
	Layers: []asset.Layer{{
		Stroke:      "#80ffc0",
		StrokeWidth: 2,
		Fill:        "transparent",
		Paths: []asset.Path{{Commands: []asset.Command{
			{Op: asset.OpMoveTo, X: 14, Y: 2},
			{Op: asset.OpLineTo, X: 26, Y: 14},
			{Op: asset.OpLineTo, X: 14, Y: 26},
			{Op: asset.OpLineTo, X: 2, Y: 14},
			{Op: asset.OpClose},
		}}},
	}},
}

// loadPalette scans assetDir for placeable things: asset JSONs become asset
// entries (each carrying its own kind), and level JSONs become map entries that
// place a portal. The portal visual itself is not listed directly — you make a
// portal by picking a map.
func (e *MapEditor) loadPalette() {
	dir := e.assetDir
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		e.status = "palette: cannot read " + dir
		return
	}

	portalAsset, _ := filoio.LoadAsset(filoio.AssetPath(dir, "portal")) // the visual used for map portals; may be nil

	var pal []paletteEntry
	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if entry.IsDir() || (ext != filoio.ExtAsset && ext != filoio.ExtLevel) {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		stem := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))

		if ext == filoio.ExtLevel {
			// A map: placing it drops a portal (drawn as the portal asset) to that map.
			// The extension says what the document is, so nothing has to sniff its content.
			pal = append(pal, paletteEntry{ref: "portal", file: full, name: stem, asset: portalAsset, isMap: true})
			continue
		}
		a, err := filoio.LoadAsset(full)
		if err != nil {
			continue // not a valid asset or map; skip
		}
		if a.Kind == "portal" {
			continue // the portal visual is system-placed via maps, not chosen directly
		}
		pal = append(pal, paletteEntry{ref: stem, file: full, name: stem, asset: a})
	}
	sort.Slice(pal, func(i, j int) bool { return pal[i].name < pal[j].name })

	// A synthetic "portal exit" item keeps the same place-from-the-list workflow
	// for named arrival points (it is not a real asset on disk).
	pal = append(pal, paletteEntry{name: "portal exit", asset: exitThumb, isExit: true})
	e.palette = pal
}

// currentPaletteEntry returns the selected placeable, if any.
func (e *MapEditor) currentPaletteEntry() (paletteEntry, bool) {
	if len(e.palette) == 0 {
		return paletteEntry{}, false
	}
	if e.paletteIdx < 0 || e.paletteIdx >= len(e.palette) {
		e.paletteIdx = 0
	}
	return e.palette[e.paletteIdx], true
}

// cyclePalette advances the selected placeable.
func (e *MapEditor) cyclePalette() {
	if len(e.palette) == 0 {
		e.status = "palette is empty"
		return
	}
	e.paletteIdx = (e.paletteIdx + 1) % len(e.palette)
	e.status = "palette: " + e.palette[e.paletteIdx].name
}

// assetByRef returns a loaded asset for a spawn reference ("turret"), using the
// palette cache when possible. Returns nil if it cannot be loaded.
func (e *MapEditor) assetByRef(ref string) *asset.Asset {
	for i := range e.palette {
		if e.palette[i].ref == ref {
			return e.palette[i].asset
		}
	}
	a, err := filoio.LoadAsset(filoio.AssetPath(e.assetDir, ref))
	if err != nil {
		return nil
	}
	return a
}
