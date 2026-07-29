package mapeditor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/render"
	ui "github.com/crgimenes/minigui"
)

func newTestMapEditor() *MapEditor {
	return New(level.New(), "", "")
}

// clickWall simulates a pen click (a corner vertex), closing the draft when the
// point lands on the start.
func clickWall(e *MapEditor, x, y float64) {
	e.cursor = asset.Point{X: x, Y: y}
	if e.atDraftStart() {
		e.closeWall()
		return
	}
	e.beginWallNode(asset.Point{X: x, Y: y})
	e.finishWallNode(asset.Point{X: x, Y: y})
}

// dragWall simulates a pen click-drag (a smooth vertex): the anchor is at (ax,ay)
// and the drag releases at (ex,ey), which sets the curve handle.
func dragWall(e *MapEditor, ax, ay, ex, ey float64) {
	e.beginWallNode(asset.Point{X: ax, Y: ay})
	e.cursor = asset.Point{X: ex, Y: ey}
	e.finishWallNode(asset.Point{X: ex, Y: ey})
}

func TestPenDrawsCurve(t *testing.T) {
	e := newTestMapEditor()
	clickWall(e, 0, 0)         // corner start
	dragWall(e, 50, 0, 50, 30) // drag -> smooth cubic vertex
	if len(e.draft) != 2 {
		t.Fatalf("want 2 draft commands, got %d", len(e.draft))
	}
	if e.draft[1].Op != asset.OpCubicTo || len(e.draft[1].Ctrl) != 2 {
		t.Fatalf("a drag should produce a cubic with two controls, got %+v", e.draft[1])
	}
}

func TestEditCurveControl(t *testing.T) {
	e := newTestMapEditor()
	clickWall(e, 0, 0)
	dragWall(e, 50, 0, 50, 30)
	e.commitWall()

	var ctrl handle
	found := false
	for _, h := range e.collectHandles() {
		if h.kind == hControl {
			ctrl, found = h, true
			break
		}
	}
	if !found {
		t.Fatal("a committed curve should expose control handles")
	}

	e.moveHandle(ctrl, 99, 99)
	cmds := e.currentWallLayer().Paths[0].Commands
	last := cmds[len(cmds)-1]
	moved := false
	for _, cp := range last.Ctrl {
		if cp.X == 99 && cp.Y == 99 {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("dragging the control handle should move the control point: %+v", last.Ctrl)
	}
}

func TestAddWallCommits(t *testing.T) {
	e := newTestMapEditor()
	start := len(e.currentWallLayer().Paths)

	clickWall(e, 0, 0)
	clickWall(e, 10, 0)
	if !e.drafting {
		t.Fatal("should be drafting after two points")
	}
	e.commitWall()

	if len(e.currentWallLayer().Paths) != start+1 {
		t.Fatalf("wall not committed")
	}
	if e.drafting {
		t.Fatal("draft should be cleared after commit")
	}
}

func TestWallOpenLine(t *testing.T) {
	e := newTestMapEditor()
	clickWall(e, 0, 0)
	clickWall(e, 50, 0)
	clickWall(e, 50, 50)
	e.commitWall() // right-click/Enter finish: open polyline
	paths := e.currentWallLayer().Paths
	if paths[len(paths)-1].Closed() {
		t.Fatal("expected an open wall line")
	}
}

func TestWallClosesOnStart(t *testing.T) {
	e := newTestMapEditor()
	clickWall(e, 0, 0)
	clickWall(e, 10, 0)
	clickWall(e, 10, 10)
	clickWall(e, 0, 0) // back to start -> closes and commits
	paths := e.currentWallLayer().Paths
	last := paths[len(paths)-1]
	if last.Commands[len(last.Commands)-1].Op != asset.OpClose {
		t.Fatalf("path should be closed, got %+v", last.Commands)
	}
}

func TestWallSnapsToStartToClose(t *testing.T) {
	e := newTestMapEditor()
	e.cam.View = render.View{Scale: 1}
	e.level.Editor.SnapEnabled = true
	e.level.Editor.SnapRadiusPx = 8
	e.level.PlayerStart = level.Start{X: 500, Y: 500} // keep it away from the origin

	clickWall(e, 0, 0)
	clickWall(e, 40, 0)
	clickWall(e, 40, 40)

	// The mouse a few pixels from the start (0,0) should snap onto it (so clicking
	// closes the loop), even though it is not exactly on the start.
	x, y := e.resolveCursor(3, 3, nil)
	if x != 0 || y != 0 {
		t.Fatalf("cursor should snap to the draft start, got %v,%v", x, y)
	}
	clickWall(e, x, y)
	paths := e.currentWallLayer().Paths
	last := paths[len(paths)-1]
	if last.Commands[len(last.Commands)-1].Op != asset.OpClose {
		t.Fatal("a snapped click on the start should close the wall")
	}
}

func TestPlaceSpawnNeedsPalette(t *testing.T) {
	e := newTestMapEditor()
	e.palette = nil
	e.placeSpawn(10, 10)
	if len(e.level.Spawns) != 0 {
		t.Fatal("should not place a spawn with an empty palette")
	}

	e.palette = []paletteEntry{{ref: "enemy", name: "enemy", asset: asset.New()}}
	e.placeSpawn(10, 10)
	if len(e.level.Spawns) != 1 || e.level.Spawns[0].Asset != "enemy" {
		t.Fatalf("spawn not placed: %+v", e.level.Spawns)
	}
}

func TestAssetListPickArmsSpawnTool(t *testing.T) {
	e := newTestMapEditor()
	e.palette = []paletteEntry{
		{ref: "a", name: "a", asset: asset.New()},
		{ref: "b", name: "b", asset: asset.New()},
		{ref: "c", name: "c", asset: asset.New()},
	}
	e.paletteIdx = 0
	e.tool = toolSelect

	// Click the second row of the list. Layout above the list: a Label then the
	// category filter row, each rowH+gap tall; rows are rowH (ui: rowH=22, gap=4).
	x := float64(screenWidth-panelWidth) + 16 + 100
	y := float64(assetListTop) + (22 + 4) + (22 + 4) + 1*22 + 11
	e.assetListWith(ui.Input{MouseX: x, MouseY: y, MouseClicked: true})

	if e.paletteIdx != 1 {
		t.Fatalf("paletteIdx = %d, want 1", e.paletteIdx)
	}
	if e.tool != toolSpawn {
		t.Fatalf("tool = %d, want toolSpawn after picking an asset", e.tool)
	}
}

func TestPlaceExitItemCreatesEntry(t *testing.T) {
	e := newTestMapEditor()
	e.palette = []paletteEntry{{name: "portal exit", isExit: true}}
	e.paletteIdx = 0

	e.placeSpawn(120, 240)
	if len(e.level.Entries) != 1 {
		t.Fatalf("exit item should create an entry: %+v", e.level.Entries)
	}
	// Placing auto-selects the new item, so its panel shows without a Select click.
	if !e.hasActive || e.active.kind != hEntry || e.active.index != 0 {
		t.Fatalf("placed entry should be auto-selected: %+v", e.active)
	}
}

func TestPlaceAutoSelectsSpawn(t *testing.T) {
	e := newTestMapEditor()
	e.palette = []paletteEntry{{ref: "x", name: "x", asset: asset.New()}}
	e.paletteIdx = 0

	e.placeSpawn(10, 10)
	if !e.hasActive || e.active.kind != hSpawn || e.active.index != 0 {
		t.Fatalf("placed spawn should be auto-selected: %+v", e.active)
	}
}

func TestDoubleClickOpensAsset(t *testing.T) {
	e := newTestMapEditor()
	e.palette = []paletteEntry{{ref: "enemy", file: "enemy.lfa", name: "enemy", asset: asset.New()}}
	opened := ""
	e.onOpenAsset = func(path string) { opened = path }

	e.handleListClickAt(0, 1) // first click: select
	if opened != "" {
		t.Fatal("a single click should not open the asset")
	}
	e.handleListClickAt(0, 2) // second click within the gap: open
	if opened != "enemy.lfa" {
		t.Fatalf("double-click should open the asset, got %q", opened)
	}
}

func TestDirPickApplies(t *testing.T) {
	dir := t.TempDir()
	e := New(level.New(), filepath.Join(dir, "lvl.json"), "")
	ch := make(chan string, 1)
	ch <- dir // simulate the native chooser returning a folder
	e.dirResult = ch

	e.pollDirPick()
	if e.assetDir != dir {
		t.Fatalf("pollDirPick should repoint the palette at the chosen dir, got %q", e.assetDir)
	}
	if e.dirResult != nil {
		t.Fatal("dirResult should be cleared after applying")
	}
}

func TestDoubleClickOpensMap(t *testing.T) {
	e := newTestMapEditor()
	e.palette = []paletteEntry{{file: "gameassets/map0002.json", name: "map0002", isMap: true}}
	opened := ""
	e.onOpenMap = func(path string) { opened = path }

	e.handleListClickAt(0, 1)
	e.handleListClickAt(0, 2)
	if opened != "gameassets/map0002.json" {
		t.Fatalf("double-click on a map should open it, got %q", opened)
	}
}

func TestNewAssetCreatesFileAndOpens(t *testing.T) {
	dir := t.TempDir()
	e := New(level.New(), filepath.Join(dir, "lvl.json"), dir)
	opened := ""
	e.onOpenAsset = func(path string) { opened = path }

	e.openNewAssetDialog()
	e.newAssetKind = "enemy"
	e.newAssetName = "grunt"
	e.createNewAsset()

	path := filoio.AssetPath(dir, "grunt")
	_, err := os.Stat(path)
	if err != nil {
		t.Fatalf("asset file not created: %v", err)
	}
	if opened != path {
		t.Fatalf("new asset should open for editing, opened %q", opened)
	}
	if e.newAssetOpen {
		t.Fatal("dialog should close after creating")
	}
	a, err := filoio.LoadAsset(path)
	if err != nil || a.Kind != "enemy" || a.Name != "grunt" {
		t.Fatalf("created asset wrong: %+v err=%v", a, err)
	}
}

func TestNewAssetRejectsBadName(t *testing.T) {
	dir := t.TempDir()
	e := New(level.New(), filepath.Join(dir, "lvl.json"), dir)
	called := false
	e.onOpenAsset = func(string) { called = true }

	e.openNewAssetDialog()
	e.newAssetKind = "enemy"
	e.newAssetName = "../evil"
	e.createNewAsset()

	if called {
		t.Fatal("a name with a path separator must be rejected")
	}
	_, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil.json"))
	if err == nil {
		t.Fatal("the file escaped the asset directory")
	}
}

func TestPlaceMapEntryCreatesPortal(t *testing.T) {
	e := newTestMapEditor()
	e.palette = []paletteEntry{{ref: "gameassets/portal.json", name: "map0002", isMap: true}}
	e.paletteIdx = 0

	e.placeSpawn(50, 60)
	if len(e.level.Spawns) != 1 {
		t.Fatalf("portal not placed: %+v", e.level.Spawns)
	}
	s := e.level.Spawns[0]
	if s.Kind != "portal" || s.Target != "map0002" || s.Asset != "gameassets/portal.json" {
		t.Fatalf("picking a map should place a portal to it: %+v", s)
	}
	if assetKind(e.palette[0]) != "map" {
		t.Fatal("a map palette entry should report kind 'map'")
	}
}

func TestIsLevelFile(t *testing.T) {
}

func TestAssetFilter(t *testing.T) {
	e := newTestMapEditor()
	mk := func(kind string) paletteEntry {
		a := asset.New()
		a.Kind = kind
		return paletteEntry{ref: kind + ".json", name: kind, asset: a}
	}
	e.palette = []paletteEntry{mk("enemy"), mk("heal"), mk("shield"), mk("enemy")}

	kinds := e.paletteKinds()
	if len(kinds) != 3 || kinds[0] != "enemy" || kinds[1] != "heal" || kinds[2] != "shield" {
		t.Fatalf("paletteKinds = %v, want [enemy heal shield]", kinds)
	}

	e.assetFilter = ""
	got := e.filteredPaletteIndices()
	if len(got) != 4 {
		t.Fatalf("no filter should show all 4, got %v", got)
	}
	e.assetFilter = "enemy"
	got = e.filteredPaletteIndices()
	if len(got) != 2 || got[0] != 0 || got[1] != 3 {
		t.Fatalf("enemy filter = %v, want [0 3]", got)
	}
}

// TestFilterDropdown checks the category dropdown's label and selected row map to the active
// filter — "All"/row 0 when unfiltered, the category name/its row otherwise.
func TestFilterDropdown(t *testing.T) {
	e := &MapEditor{}
	cats := []string{"All", "enemy", "heal"}
	if e.filterLabel() != "All" || e.filterIndex(cats) != 0 {
		t.Fatalf("unfiltered should be All/row0, got %q/%d", e.filterLabel(), e.filterIndex(cats))
	}
	e.assetFilter = "heal"
	if e.filterLabel() != "heal" || e.filterIndex(cats) != 2 {
		t.Fatalf("filter heal should be heal/row2, got %q/%d", e.filterLabel(), e.filterIndex(cats))
	}
}

func TestWallColorPick(t *testing.T) {
	e := newTestMapEditor()
	e.colorOpen = true

	// Click the first stroke swatch. Panel at (colorPanelX, colorPanelY): a Label,
	// the "stroke:" Label, then the swatch row (ui: rowH=22, gap=4, swatchSize=18).
	x := float64(colorPanelX) + 9
	y := float64(colorPanelY) + (22 + 4) + (22 + 4) + 9
	e.colorPanelWith(ui.Input{MouseX: x, MouseY: y, MouseClicked: true})

	got := e.currentWallLayer().Stroke
	if got != ui.VGAPalette[0] {
		t.Fatalf("wall stroke = %q, want %q", got, ui.VGAPalette[0])
	}
	if e.colorOpen {
		t.Fatal("color panel should close after picking")
	}
}

func TestPlaceAndSelectEntry(t *testing.T) {
	e := newTestMapEditor()
	e.placeEntry(120, 240)
	if len(e.level.Entries) != 1 {
		t.Fatalf("entry not placed: %+v", e.level.Entries)
	}
	if e.level.Entries[0].Name == "" {
		t.Fatal("placed entry should get a default name")
	}

	e.active = handle{kind: hEntry, index: 0}
	e.hasActive = true
	en, ok := e.selectedEntry()
	if !ok || en != &e.level.Entries[0] {
		t.Fatal("selectedEntry should point at the placed entry")
	}
	if !e.spawnPanelActive() {
		t.Fatal("panel should be active for a selected entry")
	}

	// Deleting the selected entry removes it.
	e.deleteSelected()
	if len(e.level.Entries) != 0 {
		t.Fatalf("entry not deleted: %+v", e.level.Entries)
	}
}

func TestSpawnPanelPortalTarget(t *testing.T) {
	e := newTestMapEditor()
	e.level.Spawns = []level.Spawn{{Name: "p", Asset: "portal.json", Kind: "portal", X: 0, Y: 0}}
	e.active = handle{kind: hSpawn, index: 0}
	e.hasActive = true

	sp, ok := e.selectedPortal()
	if !ok || sp.Kind != "portal" {
		t.Fatal("selectedPortal should return the portal spawn")
	}
	if !e.spawnPanelActive() {
		t.Fatal("panel should be active for a selected portal")
	}

	// Click the target field (Label rowH+gap above it) to focus it.
	x := float64(screenWidth-panelWidth) + 16 + 100
	y := float64(spawnPanelTop) + (22 + 4) + 11
	e.spawnPanelWith(ui.Input{MouseX: x, MouseY: y, MouseClicked: true})
	if !e.spawnPanel.HasFocus() {
		t.Fatal("clicking the target field should focus it")
	}

	// A non-portal selection switches the panel to the MAP properties and drops
	// the portal field's focus (single-key shortcuts resume).
	e.level.Spawns[0].Kind = "enemy"
	e.spawnPanelWith(ui.Input{})
	if e.spawnPanel.HasFocus() {
		t.Fatal("focus should clear when no portal is selected")
	}
	if !e.spawnPanelActive() {
		t.Fatal("the panel stays active: with nothing selected it shows the map properties")
	}
}

func TestPlaceZoneRect(t *testing.T) {
	e := newTestMapEditor()
	e.zoneKind = level.ZoneRect
	e.placeZonePoint(0, 0)
	if len(e.level.Zones) != 0 {
		t.Fatal("rect should need two clicks")
	}
	e.placeZonePoint(20, 10)
	if len(e.level.Zones) != 1 || e.level.Zones[0].Kind != level.ZoneRect {
		t.Fatalf("zone rect not created: %+v", e.level.Zones)
	}
}

func TestPlaceZoneCircle(t *testing.T) {
	e := newTestMapEditor()
	e.zoneKind = level.ZoneCircle
	e.placeZonePoint(10, 10)
	e.placeZonePoint(13, 14) // radius 5
	if len(e.level.Zones) != 1 || e.level.Zones[0].Radius != 5 {
		t.Fatalf("zone circle not created: %+v", e.level.Zones)
	}
}

func TestDeleteSelectedSpawn(t *testing.T) {
	e := newTestMapEditor()
	e.level.Spawns = []level.Spawn{{Name: "a", Asset: "x.json", Kind: "enemy"}, {Name: "b", Asset: "x.json", Kind: "enemy"}}
	e.active = handle{kind: hSpawn, index: 0}
	e.hasActive = true
	e.deleteSelected()
	if len(e.level.Spawns) != 1 || e.level.Spawns[0].Name != "b" {
		t.Fatalf("wrong spawn removed: %+v", e.level.Spawns)
	}
}

func TestDeleteSelectedOrHint(t *testing.T) {
	e := newTestMapEditor()

	e.hasActive = false
	e.deleteSelectedOrHint()
	if e.status != "nothing selected to delete" {
		t.Fatalf("expected hint when nothing selected, got %q", e.status)
	}

	e.level.Spawns = []level.Spawn{{Name: "a", Asset: "x.json", Kind: "enemy"}}
	e.active = handle{kind: hSpawn, index: 0}
	e.hasActive = true
	e.deleteSelectedOrHint()
	if len(e.level.Spawns) != 0 {
		t.Fatalf("selected spawn not deleted via button path: %+v", e.level.Spawns)
	}
}

func TestMoveWallVertex(t *testing.T) {
	e := newTestMapEditor()
	e.level.Walls[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 0, Y: 0},
		{Op: asset.OpLineTo, X: 10, Y: 0},
	}}}
	h := handle{kind: hWallVertex, layer: 0, path: 0, cmd: 1}
	e.moveHandle(h, 12, 5)
	got := e.level.Walls[0].Paths[0].Commands[1]
	if got.X != 12 || got.Y != 5 {
		t.Fatalf("vertex not moved: %+v", got)
	}
}

func TestSnapAllToGrid(t *testing.T) {
	e := newTestMapEditor()
	e.level.Editor.SnapToGrid = false // pixel grid
	e.level.PlayerStart = level.Start{X: 9.4, Y: 10.6}
	e.level.Spawns = []level.Spawn{{Name: "a", Asset: "x.json", Kind: "enemy", X: 3.7, Y: 4.2}}
	e.snapAllToGrid()
	if e.level.PlayerStart.X != 9 || e.level.PlayerStart.Y != 11 {
		t.Fatalf("player start not snapped: %+v", e.level.PlayerStart)
	}
	if e.level.Spawns[0].X != 4 {
		t.Fatalf("spawn not snapped: %+v", e.level.Spawns[0])
	}
}

func TestUndoRestoresLevel(t *testing.T) {
	e := newTestMapEditor()
	orig := len(e.level.Spawns)
	e.palette = []paletteEntry{{ref: "x", name: "x", asset: asset.New()}}

	// One settled edit: place a spawn, then an idle frame.
	before := e.level.Clone()
	e.dirtyThisFrame = false
	e.placeSpawn(1, 1)
	e.hist.Commit(before, e.dirtyThisFrame, e.dragging)
	e.dirtyThisFrame = false
	e.hist.Commit(e.level.Clone(), false, false)

	e.undo()
	if len(e.level.Spawns) != orig {
		t.Fatalf("undo did not restore spawns: %d", len(e.level.Spawns))
	}
}
