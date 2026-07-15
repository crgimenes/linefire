package mapeditor

import (
	"image"
	"math"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	ui "github.com/crgimenes/minigui"
	"linefire/asset"
	"linefire/editorkit"
	"linefire/render"
)

// assetListTop is the panel y where the asset picker starts, below the
// properties readout and above the legend.
const assetListTop = toolbarHeight + 290

// assetThumbPx is the side of each list row's asset thumbnail, in logical pixels.
const assetThumbPx = 18

// runAssetList drives the scrollable asset picker in the right panel from live
// input. Each entry shows a thumbnail, the asset name and its category; picking
// one selects it as the spawn asset and arms the spawn tool, so authoring is
// "click an asset, then click the map". With many assets the list scrolls, which
// a row of buttons could not.
func (e *MapEditor) runAssetList() {
	e.assetListWith(ui.InputFromEbiten())
}

// assetListWith is the testable core of runAssetList, taking an explicit input
// snapshot instead of sampling the live ebiten state.
func (e *MapEditor) assetListWith(in ui.Input) {
	px := float64(screenWidth - panelWidth)

	// While the filter dropdown is OPEN it floats OVER the asset rows (an overlay, not in-flow —
	// in-flow pushed the whole list down over the panels below). A click inside the overlay must
	// not also land on the row underneath, so the list gets a click-masked input.
	listIn := in
	if e.filterOpen && e.filterOverlayRect().Contains(in.MouseX, in.MouseY) {
		listIn.MouseClicked = false
		listIn.MouseDown = false
	}

	e.assetList.Begin(listIn, px+16, assetListTop)
	e.assetList.Label("ASSETS (pick + place)")
	if len(e.palette) == 0 {
		e.assetList.Label("none (run in a dir with assets)")
		e.assetIcons = nil
		e.assetFiltered = nil
		e.assetList.End()
		return
	}

	// The filter dropdown BUTTON lives in the flow; the open category list is the overlay,
	// driven after the asset list (see runFilterOverlay below).
	arrow := " v"
	if e.filterOpen {
		arrow = " ^"
	}
	if e.assetList.Button("flt.toggle", "Filter: "+e.filterLabel()+arrow) {
		e.filterOpen = !e.filterOpen
	}

	filtered := e.filteredPaletteIndices()
	e.assetFiltered = filtered
	items := make([]string, len(filtered))
	sel := 0
	for i, pi := range filtered {
		items[i] = e.palette[pi].name + "  [" + assetKind(e.palette[pi]) + "]"
		if pi == e.paletteIdx {
			sel = i
		}
	}

	changed, clicked, icons := e.assetList.ListWithIcons("assets", items, &sel, assetThumbPx)
	e.assetIcons = icons
	if changed && sel >= 0 && sel < len(filtered) {
		e.paletteIdx = filtered[sel]
		e.setTool(toolSpawn)
		e.status = "asset: " + e.palette[e.paletteIdx].name
	}
	if clicked >= 0 && clicked < len(filtered) {
		e.handleListClick(filtered[clicked])
	}
	e.assetList.End()

	if e.runFilterOverlay(in) { // the floating category list, over the rows
		shown := e.filteredPaletteIndices()
		if len(shown) > 0 {
			e.paletteIdx = shown[0] // keep the highlight on a visible row
		}
	}
}

// handleListClick tracks list clicks for double-click detection: a double-click
// on an asset entry opens it for editing (a mode switch handled by the host).
func (e *MapEditor) handleListClick(pi int) {
	e.handleListClickAt(pi, ebiten.Tick())
}

// handleListClickAt is the testable core of handleListClick, taking the tick.
func (e *MapEditor) handleListClickAt(pi int, tick int64) {
	dbl := pi == e.lastListClickIdx && editorkit.DoubleClick(tick, e.lastListClickTick, 0, 0)
	e.lastListClickIdx = pi
	e.lastListClickTick = tick
	if dbl {
		e.openAssetEntry(pi)
	}
}

// openAssetEntry asks the host to open the palette item for editing: a map opens
// the map editor on it, an asset opens the asset editor. The portal-exit
// pseudo-item is not a file, so it is ignored.
func (e *MapEditor) openAssetEntry(pi int) {
	if pi < 0 || pi >= len(e.palette) {
		return
	}
	en := e.palette[pi]
	if en.isExit || en.file == "" {
		e.status = "double-click an asset or map to edit it"
		return
	}
	if en.isMap {
		if e.onOpenMap != nil {
			e.onOpenMap(en.file)
		}
		return
	}
	if e.onOpenAsset != nil {
		e.onOpenAsset(en.file)
	}
}

// allFilterLabel is the dropdown entry (and button text) that clears the category filter.
const allFilterLabel = "All"

// filterRect is a screen rectangle with a point test (the toolkit keeps no rect type).
type filterRect struct{ x0, y0, x1, y1 float64 }

func (r filterRect) Contains(x, y float64) bool {
	return x >= r.x0 && x < r.x1 && y >= r.y0 && y < r.y1
}

// filterOverlayRect is where the open category dropdown floats: just under its button,
// over the asset rows. Sized for the capped list (6 rows) plus the border.
func (e *MapEditor) filterOverlayRect() filterRect {
	px := float64(screenWidth - panelWidth)
	rows := min(
		// + the "All" row
		len(e.paletteKinds())+1,
		// the toolkit list scrolls past 6 rows
		6)
	y0 := float64(assetListTop + 54) // below the ASSETS label + the Filter button
	return filterRect{x0: px + 16, y0: y0, x1: px + panelWidth - 16, y1: y0 + float64(rows*26) + 4}
}

// runFilterOverlay drives the floating category list while the dropdown is open, OVER the asset
// rows rather than pushing them down. Picking a row (or "All") sets the filter and closes the
// overlay. Reports whether the active filter changed this frame.
func (e *MapEditor) runFilterOverlay(in ui.Input) bool {
	if !e.filterOpen {
		return false
	}
	r := e.filterOverlayRect()
	e.filterList.Begin(in, r.x0, r.y0)
	cats := append([]string{allFilterLabel}, e.paletteKinds()...)
	sel := e.filterIndex(cats)
	picked := e.filterList.List("flt.list", cats, &sel)
	e.filterList.End()
	if !picked {
		return false // still open, no new pick this frame
	}
	e.filterOpen = false
	next := "" // row 0 ("All") clears the filter
	if sel > 0 && sel < len(cats) {
		next = cats[sel]
	}
	if next == e.assetFilter {
		return false
	}
	e.assetFilter = next
	return true
}

// filterLabel is the active category shown on the dropdown button ("All" when unfiltered).
func (e *MapEditor) filterLabel() string {
	if e.assetFilter == "" {
		return allFilterLabel
	}
	return e.assetFilter
}

// filterIndex is the row of the active filter within cats (0 = the "All" row).
func (e *MapEditor) filterIndex(cats []string) int {
	for i, c := range cats {
		if c == e.assetFilter {
			return i
		}
	}
	return 0
}

// paletteKinds returns the distinct asset categories present in the palette,
// sorted, for the filter row.
func (e *MapEditor) paletteKinds() []string {
	seen := map[string]bool{}
	var ks []string
	for i := range e.palette {
		k := assetKind(e.palette[i])
		if !seen[k] {
			seen[k] = true
			ks = append(ks, k)
		}
	}
	sort.Strings(ks)
	return ks
}

// filteredPaletteIndices returns the palette indices that match the active
// category filter (all of them when no filter is set).
func (e *MapEditor) filteredPaletteIndices() []int {
	var out []int
	for i := range e.palette {
		if e.assetFilter == "" || assetKind(e.palette[i]) == e.assetFilter {
			out = append(out, i)
		}
	}
	return out
}

// drawAssetThumbs renders each visible list row's asset thumbnail into the icon
// slot the list reserved. The toolkit cannot draw vector assets, so this runs
// after the list's own fills and text are flushed.
func (e *MapEditor) drawAssetThumbs(dst *ebiten.Image) {
	for _, slot := range e.assetIcons {
		if slot.Index < 0 || slot.Index >= len(e.assetFiltered) {
			continue
		}
		pi := e.assetFiltered[slot.Index]
		if pi < 0 || pi >= len(e.palette) {
			continue
		}
		drawAssetThumb(dst, e.palette[pi].asset, slot.X, slot.Y, slot.Size)
	}
}

// drawAssetThumb draws an asset's geometry fit and centered into a size×size box
// at (x,y), clipped to that box so strokes never bleed into neighboring rows.
func drawAssetThumb(dst *ebiten.Image, a *asset.Asset, x, y, size float64) {
	if a == nil {
		return
	}
	minX, minY, maxX, maxY, ok := layersBounds(a.Layers)
	if !ok {
		return
	}
	span := math.Max(maxX-minX, maxY-minY)
	if span <= 0 {
		return
	}
	const margin = 1.0
	scale := (size - 2*margin) / span
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	v := render.View{
		OffsetX: x + size/2 - cx*scale,
		OffsetY: y + size/2 - cy*scale,
		Scale:   scale,
	}
	box := image.Rect(int(x), int(y), int(x+size), int(y+size))
	render.DrawLayers(dst.SubImage(box).(*ebiten.Image), a.Layers, v, true)
}

// layersBounds returns the bounding box of every drawable point across the
// layers, reporting ok=false when there is no geometry.
func layersBounds(layers []asset.Layer) (minX, minY, maxX, maxY float64, ok bool) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for i := range layers {
		for _, p := range layers[i].Paths {
			for _, c := range p.Commands {
				if c.Op == asset.OpClose {
					continue
				}
				minX = math.Min(minX, c.X)
				minY = math.Min(minY, c.Y)
				maxX = math.Max(maxX, c.X)
				maxY = math.Max(maxY, c.Y)
			}
		}
	}
	return minX, minY, maxX, maxY, !math.IsInf(minX, 1)
}
