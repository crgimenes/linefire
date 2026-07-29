// Package mapeditor implements the Linefire stage (phase) editor: a top-down
// 2D world editor for walls, the player start, enemy/power-up spawns and
// trigger zones. It shares editorkit (camera, grid, history) and render
// (vector + glow) with the asset editor.
package mapeditor

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/editorkit"
	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/render"
	ui "github.com/crgimenes/minigui"
)

// Window and layout geometry, in logical screen pixels.
const (
	screenWidth   = 1100
	screenHeight  = 800 // tall enough for the wrapped filter rows + the map panel
	toolbarHeight = 56  // two rows of buttons (tools/edit on top, view/content below)
	panelWidth    = 320
)

// toolID identifies the active editing tool (number-key shortcuts 1..5).
type toolID int

const (
	toolSelect toolID = 1 // select / move a handle
	toolWall   toolID = 2 // draw wall polylines
	toolStart  toolID = 3 // set the player start
	toolSpawn  toolID = 4 // place a spawn/portal/exit (the selected list item decides)
	toolZone   toolID = 5 // add a trigger zone (rect/circle)
	toolDelete toolID = 6 // click a node to delete it
)

// MapEditor is the ebiten.Game for stage editing.
type MapEditor struct {
	level    *level.Level
	savePath string
	assetDir string // directory scanned for placeable assets
	tool     toolID

	cam        editorkit.Camera
	lastMouseX float64
	lastMouseY float64
	panning    bool

	// In-progress wall path (final M/L/C commands). The pen tool builds curves: a
	// click adds a corner (L), a click-drag adds a smooth point (C) where the drag
	// is the control handle. penAnchor is the vertex being placed while the button
	// is held; draftLastHandle is the previous vertex's out-handle.
	draft           []asset.Command
	drafting        bool
	penActive       bool
	penAnchor       asset.Point
	draftLastHandle asset.Point

	// Last left-click, for double-click detection (finishes an open wall).
	lastClickTick int64
	lastClickX    float64
	lastClickY    float64

	// In-progress zone shape.
	zoneKind  string
	zoneDraft []asset.Point

	// Selection / drag.
	active    handle
	hasActive bool
	dragging  bool

	// Joining loose ends (kutta-style, implicit in Select): open-path endpoints show as red
	// squares; clicking one arms the join (joinFrom), clicking another welds/closes. A press on
	// an endpoint is ambiguous — click (join) or drag (move) — so it stays a pending join until
	// the mouse travels past the drag threshold (pressX/pressY anchor that test).
	joinFrom       handle
	hasJoinFrom    bool
	pendingJoin    bool
	pressX, pressY float64

	// Asset palette for placing spawns.
	palette    []paletteEntry
	paletteIdx int

	// Per-frame input state.
	snapDisabled  bool
	mouseInCanvas bool
	cursor        asset.Point
	snapActive    bool
	snapPoint     asset.Point

	// View options.
	glowEnabled bool
	preview     bool // negative-space map preview (N): playable interior black, exterior glow
	glow        *render.Glow

	// Reference backdrop: a raster image (a screenshot of a classic map, etc.) drawn behind the
	// canvas as a tracing template. PURELY TRANSIENT — held only in memory, never written to the
	// .lfm (writeSave serializes only *level.Level), so it is a drawing aid and nothing more.
	backdrop        *ebiten.Image
	backdropPos     asset.Point // world position of the image's top-left corner
	backdropScale   float64     // world units per image pixel
	backdropAlpha   float64     // 0..1 opacity
	backdropVisible bool
	imgResult       chan string // pending native open-panel result (reference image)

	bar        ui.Context    // clickable top tool bar
	assetList  ui.Context    // scrollable asset picker in the right panel
	assetIcons []ui.IconSlot // this frame's visible thumbnail slots, drawn in Draw

	assetFilter   string     // asset list category filter ("" = all)
	assetFiltered []int      // palette indices currently shown (maps list rows -> palette)
	filterOpen    bool       // whether the category dropdown is expanded
	filterList    ui.Context // the OPEN dropdown: a floating overlay drawn above the asset rows

	lastListClickIdx  int   // palette index of the last list click (for double-click)
	lastListClickTick int64 // tick of the last list click

	dirResult  chan string // pending native folder-chooser result (assets dir picker)
	openResult chan string // pending native open-panel result (open map)
	saveResult chan string // pending native save-panel result (save map as)
	svgResult  chan string // pending native open-panel result (import SVG)

	colors    ui.Context // wall color picker popup
	colorOpen bool

	spawnPanel    ui.Context // property panel: selected spawn/entry/zone, else the map itself
	panelModePrev string     // last shown property set; a change drops the panel focus

	// New-asset modal: pick a kind + file name, create it, open it for editing.
	newAsset     ui.Context
	newAssetOpen bool
	newAssetName string
	newAssetKind string

	dirty      bool
	shownTitle string

	// Apple-style autosave: persist on field/window blur (and on close / Cmd+S).
	prevWinFocus   bool
	prevFieldFocus bool

	dirtyThisFrame bool
	hist           *editorkit.History[*level.Level]

	status string

	// onOpenAsset / onOpenMap, when set by the host app, open a file for editing in
	// the same window (a mode switch): an asset double-click / New asset dialog goes
	// to onOpenAsset, a map double-click goes to onOpenMap.
	onOpenAsset func(path string)
	onOpenMap   func(path string)
}

// New creates a map editor for the given level. savePath is the file used when
// saving; assetDir is scanned for placeable spawn assets.
func New(l *level.Level, savePath, assetDir string) *MapEditor {
	// An empty savePath is left empty on purpose: the map is "untitled" and Cmd+S (or S)
	// opens the native save panel to name it, like a normal macOS app — no forced default.
	e := &MapEditor{
		level:            l,
		savePath:         savePath,
		assetDir:         assetDir,
		tool:             toolWall,
		zoneKind:         level.ZoneRect,
		glowEnabled:      true,
		glow:             render.NewGlow(),
		hist:             editorkit.NewHistory(func(x *level.Level) *level.Level { return x.Clone() }, 200),
		prevWinFocus:     true, // window starts focused; a later blur triggers autosave
		lastListClickIdx: -1,
	}
	// The map is unbounded: allow zooming far out to see a whole long stage,
	// and far in for precision.
	e.cam.MinScale = 0.01
	e.cam.MaxScale = 256
	e.loadPalette()
	e.frameContent()
	e.status = "ready"
	return e
}

// Layout uses a fixed logical resolution.
func (e *MapEditor) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

// Update advances one frame.
func (e *MapEditor) Update() error {
	before := e.level.Clone()
	e.dirtyThisFrame = false

	e.pollDirPick()
	e.pollOpenPick()
	e.pollSavePick()
	e.pollSVGImport()
	e.pollBackdrop()
	e.runToolbar()
	e.runAssetList()
	e.runSpawnPanel()
	if e.colorOpen {
		e.runColorPanel()
	}
	if e.newAssetOpen {
		e.runNewAssetDialog()
	}
	e.handleInput()
	// While a text field is focused, coalesce edits into one undo step.
	e.hist.Commit(before, e.dirtyThisFrame, e.dragging || e.spawnPanel.HasFocus())

	e.autoSaveOnBlur()
	e.updateTitle()
	return e.handleClose()
}

// autoSaveOnBlur persists unsaved edits the moment a text field or the whole
// window loses focus (Apple-style). Save-on-close and Cmd+S cover the rest; undo
// and git versioning are the safety net.
func (e *MapEditor) autoSaveOnBlur() {
	winFocus := ebiten.IsFocused()
	fieldFocus := e.spawnPanel.HasFocus()
	if (e.prevWinFocus && !winFocus) || (e.prevFieldFocus && !fieldFocus) {
		e.autoSave()
	}
	e.prevWinFocus = winFocus
	e.prevFieldFocus = fieldFocus
}

// autoSave writes the level when there are unsaved changes AND it already has a name. An
// untitled map is skipped: a blur must never pop a modal save panel (that is Cmd+S's job).
func (e *MapEditor) autoSave() {
	if e.dirty && e.savePath != "" {
		e.writeSave()
	}
}

// SetOnOpenAsset wires the host callback that opens an asset file for editing in
// the same window. Called by the editapp host; nil means standalone (no-op).
func (e *MapEditor) SetOnOpenAsset(fn func(path string)) {
	e.onOpenAsset = fn
}

// SetOnOpenMap wires the host callback that opens a different map for editing.
func (e *MapEditor) SetOnOpenMap(fn func(path string)) {
	e.onOpenMap = fn
}

// Persist writes any unsaved changes now, e.g. before the host switches to a
// different map or asset.
func (e *MapEditor) Persist() {
	e.autoSave()
}

// ReloadPalette rescans the asset/map directory, e.g. after returning from the
// asset editor or creating a new asset.
func (e *MapEditor) ReloadPalette() {
	e.loadPalette()
}

// ResetTitle forces the next updateTitle to push to the window, used when the host
// re-activates this editor after another mode changed the window title.
func (e *MapEditor) ResetTitle() {
	e.shownTitle = ""
}

// markDirty records an unsaved change this frame.
func (e *MapEditor) markDirty() {
	e.dirty = true
	e.dirtyThisFrame = true
}

// updateTitle reflects file name and unsaved state in the window title.
func (e *MapEditor) updateTitle() {
	title := "Linefire Map Editor — " + e.savePath
	if e.dirty {
		title += " *"
	}
	if title != e.shownTitle {
		ebiten.SetWindowTitle(title)
		e.shownTitle = title
	}
}

// handleClose autosaves and quits on the window close button (Apple-style: no
// unsaved-changes prompt — the file is always kept current).
func (e *MapEditor) handleClose() error {
	if ebiten.IsWindowBeingClosed() {
		e.autoSave()
		return ebiten.Termination
	}
	return nil
}

// undo / redo restore level snapshots from the shared history.
func (e *MapEditor) undo() {
	doc, ok := e.hist.Undo(e.level)
	if !ok {
		e.status = "nothing to undo"
		return
	}
	e.level = doc
	e.afterHistorySwap()
	e.status = "undo"
}

func (e *MapEditor) redo() {
	doc, ok := e.hist.Redo(e.level)
	if !ok {
		e.status = "nothing to redo"
		return
	}
	e.level = doc
	e.afterHistorySwap()
	e.status = "redo"
}

func (e *MapEditor) afterHistorySwap() {
	e.hasActive = false
	e.dragging = false
	e.resetDraft()
	e.zoneDraft = nil
	e.dirty = true
}

// save is the Cmd+S / S action: write to the current file, or — when the map is still
// untitled (no path) — fall through to the native Save As panel to name it first.
func (e *MapEditor) save() {
	if e.savePath == "" {
		e.pickSaveMapAs()
		return
	}
	e.writeSave()
}

// writeSave writes the level to its current path. Callers guarantee savePath is set.
func (e *MapEditor) writeSave() {
	err := filoio.SaveLevel(e.savePath, e.level)
	if err != nil {
		e.status = "save error: " + err.Error()
		return
	}
	e.dirty = false
	e.status = "saved " + e.savePath
}

// savePathLabel is the file shown in the status bar, or "untitled" before the map is named.
func (e *MapEditor) savePathLabel() string {
	if e.savePath == "" {
		return "untitled"
	}
	return e.savePath
}

// currentWallLayer returns the layer new wall paths are added to.
func (e *MapEditor) currentWallLayer() *asset.Layer {
	if len(e.level.Walls) == 0 {
		e.level.Walls = append(e.level.Walls, asset.Layer{
			Name: "walls", Stroke: "#80ffff", StrokeWidth: 2, Fill: "transparent", Glow: 0.8,
		})
	}
	return &e.level.Walls[0]
}
