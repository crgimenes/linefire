package mapeditor

import (
	"os"

	"github.com/crgimenes/gion"
	ui "github.com/crgimenes/minigui"

	"linefire/editorkit"
	"linefire/level"
	"linefire/sfx"
)

// spawnPanelTop is the panel y where the property panel sits — anchored to the BOTTOM so its
// tallest form (the map panel: label+field+hint+play/stop ≈ 110px) always fits on screen, no
// matter how tall the toolbar or the asset list above grow.
const spawnPanelTop = screenHeight - 150

// runSpawnPanel drives the property panel for the selected spawn from live input.
// Today it only edits a portal's target map name; other per-spawn fields can join
// it later.
func (e *MapEditor) runSpawnPanel() {
	e.spawnPanelWith(ui.InputFromEbiten())
}

// panelMode drops the panel's keyboard focus whenever the shown property set
// changes (portal -> map, zone -> entry...), so a focused field from the previous
// selection does not keep suppressing the single-key shortcuts.
func (e *MapEditor) panelMode(mode string) {
	if e.panelModePrev != mode {
		e.spawnPanel.ClearFocus()
		e.panelModePrev = mode
	}
}

// spawnPanelWith is the testable core of runSpawnPanel. For a selected portal it
// edits the destination map and entry; for a selected entry it edits the entry
// name. When nothing is selected it shows the MAP's own properties (stage music).
func (e *MapEditor) spawnPanelWith(in ui.Input) {
	px := float64(screenWidth - panelWidth)
	sp, ok := e.selectedPortal()
	if ok {
		e.panelMode("portal")
		e.spawnPanel.Begin(in, px+16, spawnPanelTop)
		e.spawnPanel.Label("PORTAL -> map  or  map:label")
		if e.spawnPanel.TextField("spawn.target", &sp.Target) {
			e.markDirty()
		}
		e.spawnPanel.End()
		return
	}
	en, ok := e.selectedEntry()
	if ok {
		e.panelMode("entry")
		e.spawnPanel.Begin(in, px+16, spawnPanelTop)
		e.spawnPanel.Label("ENTRY name:")
		if e.spawnPanel.TextField("entry.name", &en.Name) {
			e.markDirty()
		}
		e.spawnPanel.End()
		return
	}
	z, ok := e.selectedZone()
	if ok {
		e.panelMode("zone")
		e.spawnPanel.Begin(in, px+16, spawnPanelTop)
		e.spawnPanel.Label("ZONE name / trigger (exit=goal):")
		if e.spawnPanel.TextField("zone.name", &z.Name) {
			e.markDirty()
		}
		if e.spawnPanel.TextField("zone.trigger", &z.Trigger) {
			e.markDirty()
		}
		e.spawnPanel.End()
		return
	}
	e.mapPanelWith(in, px)
}

// mapPanelWith edits the MAP's own properties when nothing is selected: the stage
// music — an .mp3 path or a gion mood name — with a played-by-ear preview. The
// hint flags a missing file or an unknown mood before anything plays silence.
func (e *MapEditor) mapPanelWith(in ui.Input, px float64) {
	e.panelMode("map")
	e.spawnPanel.Begin(in, px+16, spawnPanelTop)
	e.spawnPanel.Label("MAP music (.mp3 path or gion mood):")
	if e.spawnPanel.TextField("map.music", &e.level.Music) {
		e.markDirty()
	}
	e.spawnPanel.Label(musicHint(e.level.Music))
	if e.spawnPanel.Button("map.play", "play") {
		e.previewMusic()
	}
	e.spawnPanel.SameLine()
	if e.spawnPanel.Button("map.stop", "stop") {
		editorkit.StopPreview()
	}
	e.spawnPanel.End()
}

// musicHint classifies the music value for the panel: silence, a readable file, a
// gion mood, or a typo.
func musicHint(m string) string {
	switch {
	case m == "":
		return "(silence)"
	case sfx.IsMusicFile(m):
		_, err := os.Stat(m)
		if err != nil {
			return "file NOT FOUND"
		}
		return "file ok"
	case sfx.KnownMood(m):
		return "gion mood (lead muted)"
	default:
		return "unknown (no sound)"
	}
}

// previewMusic plays the stage theme the way the game will: a streamed mp3 file or
// the rendered gion track.
func (e *MapEditor) previewMusic() {
	m := e.level.Music
	if sfx.IsMusicFile(m) {
		// #nosec G304 -- previewing the music file the level document names is the feature
		data, err := os.ReadFile(m)
		if err != nil {
			return
		}
		editorkit.PlayPreviewMP3(gion.DefaultRate, data, 0.8)
		return
	}
	editorkit.PlayPreview(gion.DefaultRate, sfx.Track(m, e.level.MusicSeed), 0.8)
}

// selectedZone returns the selected zone (when a zone point is selected), so the
// panel can edit its name and trigger.
func (e *MapEditor) selectedZone() (*level.Zone, bool) {
	if !e.hasActive || e.active.kind != hZone {
		return nil, false
	}
	if e.active.shapeIdx < 0 || e.active.shapeIdx >= len(e.level.Zones) {
		return nil, false
	}
	return &e.level.Zones[e.active.shapeIdx], true
}

// selectedPortal returns the selected spawn when it is a portal, so the property
// panel knows whether to show the target/entry fields.
func (e *MapEditor) selectedPortal() (*level.Spawn, bool) {
	if !e.hasActive || e.active.kind != hSpawn {
		return nil, false
	}
	if e.active.index < 0 || e.active.index >= len(e.level.Spawns) {
		return nil, false
	}
	s := &e.level.Spawns[e.active.index]
	if s.Kind != "portal" {
		return nil, false
	}
	return s, true
}

// selectedEntry returns the selected arrival point, so the panel can edit its name.
func (e *MapEditor) selectedEntry() (*level.Entry, bool) {
	if !e.hasActive || e.active.kind != hEntry {
		return nil, false
	}
	if e.active.index < 0 || e.active.index >= len(e.level.Entries) {
		return nil, false
	}
	return &e.level.Entries[e.active.index], true
}

// spawnPanelActive reports whether the property panel is on screen, so the caller
// can render it. It always is: with nothing selected it shows the map properties.
func (e *MapEditor) spawnPanelActive() bool {
	return true
}
