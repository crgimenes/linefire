// Package editor implements the minimal Linefire vector editor: an ebiten game
// with a large drawing canvas on the left and a real-size preview plus a
// properties panel on the right.
package editor

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	ui "github.com/crgimenes/minigui"
	"linefire/asset"
	"linefire/editorkit"
	"linefire/render"
)

// Window and layout geometry, in logical screen pixels.
const (
	screenWidth   = 1000
	screenHeight  = 700
	toolbarHeight = 30
	panelWidth    = 300

	previewBoxX = screenWidth - panelWidth + 16
	previewBoxY = toolbarHeight + 24
)

// toolID identifies the active editing tool. Values match the number-key
// shortcuts 1..7.
type toolID int

const (
	toolSelect    toolID = 1 // select / move a handle
	toolLine      toolID = 2 // draw a line path
	toolOrigin    toolID = 3 // set the asset origin/center
	toolWeapon    toolID = 4 // add a weapon hardpoint
	toolThruster  toolID = 5 // add a thruster hardpoint
	toolCollision toolID = 6 // add collision shapes (circle/rect/triangle)
	toolDelete    toolID = 7 // click a point to delete it
)

// strokePalette and fillPalette are the small fixed sets of colors the editor
// cycles through for the current layer.
var (
	strokePalette = []string{"#80ffff", "#ff80ff", "#ffff80", "#80ff80", "#ff8080", "#ffffff"}
	fillPalette   = []string{"transparent", "#001820", "#200018", "#0a0a14", "#000000"}
)

// Editor is the ebiten.Game implementation that owns the document and all
// transient editing state.
type Editor struct {
	asset    *asset.Asset
	savePath string // file to write on save

	tool toolID

	// Canvas camera (zoom and pan) and pan-tracking state.
	cam        editorkit.Camera
	lastMouseX float64
	lastMouseY float64
	panning    bool

	// Index of the layer currently being edited.
	layerIdx int

	// Target of mirror/rotate transforms.
	scope scopeID

	// Glow (bloom) preview. Separate renderers keep stable buffer sizes for the
	// canvas and the preview.
	glowEnabled bool
	glowVariant render.GlowVariant
	glowMain    *render.Glow
	glowPreview *render.Glow

	// CRT post-process, applied to the preview only (the in-game look).
	crtEnabled bool
	crt        *render.CRT
	previewBuf *ebiten.Image

	// In-progress path (final M/L/C commands). The pen tool builds curves: a click
	// adds a corner (L), a click-drag adds a smooth point (C) where the drag is the
	// control handle. penAnchor is the vertex being placed while the button is held;
	// draftLastHandle is the previous vertex's out-handle. drafting is true between
	// the first click and the Enter/double-click/close that commits.
	draft           []asset.Command
	drafting        bool
	penActive       bool
	penAnchor       asset.Point
	draftLastHandle asset.Point

	// Last left-click, for double-click detection (finishes an open line).
	lastClickTick int64
	lastClickX    float64
	lastClickY    float64

	// Active handle being dragged with the select tool.
	active    handle
	hasActive bool
	dragging  bool

	// Collision authoring: the kind being drawn and the points clicked so far
	// for the in-progress shape.
	collKind  string
	collDraft []asset.Point

	strokeIdx int
	fillIdx   int

	// Per-frame input state, refreshed at the start of Update.
	snapDisabled  bool        // Alt/Option held
	mouseInCanvas bool        // cursor is over the drawing canvas
	cursor        asset.Point // cursor position in asset space (snapped if snapping)
	snapActive    bool        // a snap target was found this frame
	snapPoint     asset.Point // the snapped target, when snapActive

	dirty      bool   // asset has unsaved changes
	shownTitle string // last title pushed to the window

	// Apple-style autosave: persist on field/window blur (and on close / Cmd+S).
	prevWinFocus   bool
	prevFieldFocus bool

	// onBack, when set by the host app, returns to the map editor (a mode switch
	// in the same window). When nil the editor is standalone and shows no Back.
	onBack func()

	// Undo/redo history (snapshots of the asset, coalescing continuous gestures).
	dirtyThisFrame bool
	hist           *editorkit.History[*asset.Asset]

	status string // short message shown in the panel

	// Clickable top tool bar (always shown).
	bar ui.Context

	// Hardpoints panel (toggled with F2): the ui toolkit driving the list and the
	// rename field. While a field has focus, single-key shortcuts are suspended.
	gui     ui.Context
	hpPanel bool
	hpSel   int

	// Color palette panel (toggled with F3).
	colors     ui.Context
	colorPanel bool

	// Sounds panel (toggled with U): author the asset's `sounds` block by ear.
	snd        ui.Context
	sndPanel   bool
	sndSel     int
	sndSeedBuf string   // seed field edit buffer (parsed on change)
	sndBufFor  int      // which sound index the buffer mirrors (-1 = none)
	sndMusic   []string // mp3 songs found when the panel opened (theme picks)
}

// New creates an editor for the given asset. savePath is the file used when
// saving; pass an empty string to use the default file name.
func New(a *asset.Asset, savePath string) *Editor {
	if savePath == "" {
		savePath = asset.DefaultFileName
	}
	e := &Editor{
		asset:        a,
		savePath:     savePath,
		tool:         toolLine,
		collKind:     asset.CollisionCircle,
		glowEnabled:  true,
		glowMain:     render.NewGlow(),
		glowPreview:  render.NewGlow(),
		crt:          render.NewCRT(),
		crtEnabled:   true,
		hist:         editorkit.NewHistory(func(a *asset.Asset) *asset.Asset { return a.Clone() }, 200),
		prevWinFocus: true,
		sndBufFor:    -1,
	}
	e.syncPaletteIndices()
	e.resetView()
	e.status = "ready"
	return e
}

// Run configures the window and starts the ebiten game loop.
func Run(a *asset.Asset, savePath string) error {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Linefire Editor")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowClosingHandled(true)
	ebiten.SetRunnableOnUnfocused(true) // keep ticking when unfocused so blur autosave fires
	return ebiten.RunGame(New(a, savePath))
}

// Layout uses a fixed logical resolution; the window may be resized but the
// content keeps these dimensions.
func (e *Editor) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

// Update advances one frame of input handling.
func (e *Editor) Update() error {
	before := e.asset.Clone()
	e.dirtyThisFrame = false

	e.handleInput()
	e.commitHistory(before)

	e.autoSaveOnBlur()
	e.updateTitle()
	return e.handleClose()
}

// autoSaveOnBlur persists unsaved edits when a text field or the window loses
// focus (Apple-style); save-on-close and Cmd+S cover the rest.
func (e *Editor) autoSaveOnBlur() {
	winFocus := ebiten.IsFocused()
	fieldFocus := e.gui.HasFocus()
	if (e.prevWinFocus && !winFocus) || (e.prevFieldFocus && !fieldFocus) {
		e.autoSave()
	}
	e.prevWinFocus = winFocus
	e.prevFieldFocus = fieldFocus
}

// autoSave writes the asset only when there are unsaved changes.
func (e *Editor) autoSave() {
	if e.dirty {
		e.save()
	}
}

// back returns to the host (map editor) after autosaving. No-op when standalone.
func (e *Editor) back() {
	if e.onBack == nil {
		return
	}
	e.autoSave()
	e.onBack()
}

// SetOnBack wires the host callback that returns to the map editor. Called by the
// editapp host; nil means standalone (no Back button).
func (e *Editor) SetOnBack(fn func()) {
	e.onBack = fn
}

// markDirty records that the asset has unsaved changes this frame.
func (e *Editor) markDirty() {
	e.dirty = true
	e.dirtyThisFrame = true
}

// updateTitle reflects the file name and unsaved state in the window title,
// only calling into ebiten when the title actually changes.
func (e *Editor) updateTitle() {
	title := "Linefire Editor — " + e.savePath
	if e.dirty {
		title += " *"
	}
	if title != e.shownTitle {
		ebiten.SetWindowTitle(title)
		e.shownTitle = title
	}
}

// handleClose autosaves and quits on the window close button (Apple-style: no
// unsaved-changes prompt). In the merged app, closing the window quits everything.
func (e *Editor) handleClose() error {
	if ebiten.IsWindowBeingClosed() {
		e.autoSave()
		return ebiten.Termination
	}
	return nil
}

// canvasRect is the screen region used for the zoomed drawing canvas.
func (e *Editor) canvasRect() image.Rectangle {
	return image.Rect(0, toolbarHeight, screenWidth-panelWidth, screenHeight)
}

// canvasView returns the current asset->screen transform for the canvas. It is
// mutated by zooming and panning; see view.go.
func (e *Editor) canvasView() render.View {
	return e.cam.View
}

// currentLayer returns the layer being edited, keeping layerIdx within bounds
// and guaranteeing at least one layer exists.
func (e *Editor) currentLayer() *asset.Layer {
	if len(e.asset.Layers) == 0 {
		e.asset.Layers = append(e.asset.Layers, asset.Layer{
			Name:        "main",
			Stroke:      strokePalette[0],
			StrokeWidth: 2,
			Fill:        "transparent",
		})
	}
	if e.layerIdx < 0 {
		e.layerIdx = 0
	}
	if e.layerIdx >= len(e.asset.Layers) {
		e.layerIdx = len(e.asset.Layers) - 1
	}
	return &e.asset.Layers[e.layerIdx]
}

// syncPaletteIndices aligns the cycling indices with the current layer colors
// so cycling starts from whatever the loaded asset uses.
func (e *Editor) syncPaletteIndices() {
	layer := e.currentLayer()
	for i, c := range strokePalette {
		if c == layer.Stroke {
			e.strokeIdx = i
		}
	}
	for i, c := range fillPalette {
		if c == layer.Fill {
			e.fillIdx = i
		}
	}
}
