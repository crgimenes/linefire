package editor

import (
	"os"
	"slices"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/crgimenes/gion"
	ui "github.com/crgimenes/minigui"

	"linefire/asset"
	"linefire/editorkit"
	"linefire/sfx"
)

// Sounds panel: author the asset's `sounds` block by ear. The EVENTS an asset can
// respond to come pre-filled from its kind (nobody remembers the exact names), and
// the base sound is picked from a scrollable list of what exists — regular events
// list the effect bases, a "theme" lists the music moods. Play previews exactly
// what the game will synthesize (both go through sfx); a continuous sound previews
// as a loop until stop.

// Sounds panel geometry (logical pixels). It shares the left slot with the
// hardpoints/colors panels — only one of them is open at a time.
const (
	sndPanelX      = hpPanelX
	sndPanelY      = hpPanelY
	sndPanelMargin = hpPanelMargin
	sndPanelW      = 252
	sndPanelH      = 620
)

// eventsFor suggests the sound events an asset of this kind actually responds to
// in the runtime, so authoring picks instead of remembering names.
func eventsFor(kind string) []string {
	switch kind {
	case "enemy", "turret", "rusher", "sniper", "tank":
		return []string{"fire", "destroy", "theme"}
	case "heal", "shield", "score", "computer":
		return []string{"pickup", "break"}
	case "player":
		return []string{"thruster", "hit"}
	}
	// Unknown kinds get the full set of runtime events.
	return []string{"fire", "pickup", "destroy", "hit", "thruster", "theme", "break"}
}

// eventDefaults is the starting sound per event, so a fresh entry is playable
// before any tweaking.
var eventDefaults = map[string]asset.Sound{
	"fire":     {Event: "fire", Base: "laser", Volume: 0.4},
	"destroy":  {Event: "destroy", Base: "explosion", Volume: 0.9},
	"pickup":   {Event: "pickup", Base: "pickup", Volume: 0.8},
	"hit":      {Event: "hit", Base: "hit", Volume: 0.7},
	"thruster": {Event: "thruster", Base: "engine", Volume: 0.2, Continuous: true},
	"theme":    {Event: "theme", Base: "battle"},
	"break":    {Event: "break", Base: "hit", Volume: 0.5},
}

// defaultSoundFor returns the pre-filled entry for an event (a bare one when the
// event has no default).
func defaultSoundFor(event string) asset.Sound {
	s, ok := eventDefaults[event]
	if !ok {
		return asset.Sound{Event: event, Base: "blip"}
	}
	return s
}

// nextSuggestedSound picks the first suggested event this asset does not declare
// yet, so "+ add" walks through what is missing instead of duplicating.
func nextSuggestedSound(a *asset.Asset) asset.Sound {
	declared := map[string]bool{}
	for i := range a.Sounds {
		declared[a.Sounds[i].Event] = true
	}
	for _, ev := range eventsFor(a.Kind) {
		if !declared[ev] {
			return defaultSoundFor(ev)
		}
	}
	return defaultSoundFor(eventsFor(a.Kind)[0]) // all declared: start from the first
}

// listMusicFiles returns the .mp3 paths under dir (as "dir/name" — how a level or
// theme references them), sorted; nil when the directory is absent. Scanned when
// the panel opens, so authoring sees the songs that exist right now.
func listMusicFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, en := range entries {
		if en.IsDir() || !sfx.IsMusicFile(en.Name()) {
			continue
		}
		out = append(out, dir+"/"+en.Name())
	}
	slices.Sort(out)
	return out
}

// toggleSoundPanel shows or hides the sounds panel; opening it closes the other
// left-slot panels and rescans the songs, and hiding it drops field focus and
// the preview.
func (e *Editor) toggleSoundPanel() {
	e.sndPanel = !e.sndPanel
	if e.sndPanel {
		e.hpPanel = false
		e.colorPanel = false
		e.sndMusic = listMusicFiles("music")
	} else {
		e.snd.ClearFocus()
		editorkit.StopPreview()
	}
	e.status = "sounds panel " + onOff(e.sndPanel)
}

// runSoundPanel drives the panel each frame it is open: the list of declared
// sounds, fields for the selected one, and add/remove/play.
func (e *Editor) runSoundPanel() {
	in := ui.InputFromEbiten()
	e.snd.Begin(in, sndPanelX, sndPanelY)
	e.snd.Label("SOUNDS (U)")

	n := len(e.asset.Sounds)
	if e.sndSel >= n {
		e.sndSel = n - 1
	}
	if e.sndSel < 0 {
		e.sndSel = 0
	}

	if n > 0 {
		names := make([]string, n)
		for i := range e.asset.Sounds {
			s := &e.asset.Sounds[i]
			names[i] = s.Event + "  " + s.Base
			if s.Muted {
				names[i] += "  [off]"
			}
		}
		e.snd.List("snd.list", names, &e.sndSel)
	} else {
		e.snd.Label("(none)")
	}

	if e.snd.Button("snd.add", "+ add") {
		e.asset.Sounds = append(e.asset.Sounds, nextSuggestedSound(e.asset))
		e.sndSel = len(e.asset.Sounds) - 1
		e.markDirty()
	}
	if n > 0 {
		e.snd.SameLine()
		if e.snd.Button("snd.del", "del") {
			e.asset.Sounds = append(e.asset.Sounds[:e.sndSel], e.asset.Sounds[e.sndSel+1:]...)
			e.markDirty()
		}
	}

	if len(e.asset.Sounds) > 0 && e.sndSel < len(e.asset.Sounds) {
		e.runSoundFields(&e.asset.Sounds[e.sndSel])
	}
	e.snd.End()
}

// runSoundFields edits one sound entry: the event picked from the kind's
// suggestions, the base picked from the available sounds, seed, volume, the
// continuous flag, and play/stop by ear.
func (e *Editor) runSoundFields(s *asset.Sound) {
	e.snd.Label("event:")
	e.runEventPicker(s)

	e.snd.Label("base:")
	e.runBasePicker(s)

	// The seed is edited through a string buffer, re-synced whenever the selection
	// changes, so partial typing does not fight the parse.
	if e.sndBufFor != e.sndSel {
		e.sndSeedBuf = strconv.FormatInt(s.Seed, 10)
		e.sndBufFor = e.sndSel
	}
	e.snd.Label("seed:")
	if e.snd.TextField("snd.seed", &e.sndSeedBuf) {
		v, err := strconv.ParseInt(e.sndSeedBuf, 10, 64)
		if err == nil {
			s.Seed = v
			e.markDirty()
		}
	}

	e.snd.Label("volume (0 = preset's own):")
	if e.snd.Slider("snd.vol", &s.Volume, 0, 1) {
		e.markDirty()
	}

	if e.snd.Toggle("snd.cont", "continuous", s.Continuous) {
		s.Continuous = !s.Continuous
		e.markDirty()
	}
	e.snd.SameLine()
	if e.snd.Toggle("snd.mute", "muted", s.Muted) {
		s.Muted = !s.Muted // off without deleting: the tuning survives
		e.markDirty()
	}
	e.snd.SameLine()
	if e.snd.Button("snd.play", "play") {
		e.previewSound(s)
	}
	e.snd.SameLine()
	if e.snd.Button("snd.stop", "stop") {
		editorkit.StopPreview()
	}
}

// previewSound plays one entry the way the game will: a theme streams its mp3 or
// renders its gion track and loops, a continuous sound loops its recipe, anything
// else plays once.
func (e *Editor) previewSound(s *asset.Sound) {
	if s.Event == "theme" {
		if sfx.IsMusicFile(s.Base) {
			// #nosec G304 -- previewing the song the asset references is the feature
			data, err := os.ReadFile(s.Base)
			if err != nil {
				return
			}
			editorkit.PlayPreviewMP3(gion.DefaultRate, data, s.Volume)
			return
		}
		editorkit.PlayPreviewLoop(gion.DefaultRate, sfx.Track(s.Base, s.Seed), s.Volume)
		return
	}
	pcm := sfx.Render(s.Base, s.Seed)
	if s.Continuous {
		editorkit.PlayPreviewLoop(gion.DefaultRate, pcm, s.Volume)
		return
	}
	editorkit.PlayPreview(gion.DefaultRate, pcm, s.Volume)
}

// runEventPicker shows the kind's suggested events as chips (they are few by
// design), so authoring picks a valid runtime event instead of remembering names.
// Switching to or from "theme" also swaps the base to a fitting default, because
// a theme's bases are music moods while every other event uses effect bases.
func (e *Editor) runEventPicker(s *asset.Sound) {
	const perRow = 3
	for i, ev := range eventsFor(e.asset.Kind) {
		if i%perRow != 0 {
			e.snd.SameLine()
		}
		if e.snd.Toggle(ui.ID("snd.ev."+ev), ev, s.Event == ev) && s.Event != ev {
			from, to := s.Event, ev
			s.Event = ev
			if (from == "theme") != (to == "theme") {
				s.Base = defaultSoundFor(to).Base
			}
			e.markDirty()
		}
	}
}

// runBasePicker lists the sounds this event can use — for a theme the available
// mp3 songs plus the music moods, effect bases otherwise — as a scrollable list,
// so many sounds never crowd the panel. Selecting a row sets the base.
func (e *Editor) runBasePicker(s *asset.Sound) {
	bases := sfx.Bases()
	if s.Event == "theme" {
		bases = append(append([]string{}, e.sndMusic...), sfx.Moods()...)
	}
	sel := slices.Index(bases, s.Base) // -1 (no highlight) for an unknown base
	if e.snd.List("snd.bases", bases, &sel) && sel >= 0 && sel < len(bases) {
		s.Base = bases[sel]
		e.markDirty()
	}
}

// sndPanelHovered reports whether the cursor is over the open panel, so its clicks
// and scroll are not also consumed by the canvas tools.
func (e *Editor) sndPanelHovered(mx, my float64) bool {
	if !e.sndPanel {
		return false
	}
	x := float64(sndPanelX - sndPanelMargin)
	y := float64(sndPanelY - sndPanelMargin)
	return mx >= x && mx < x+sndPanelW && my >= y && my < y+sndPanelH
}

// drawSoundPanel renders the panel backdrop and the ui draw commands on top.
func (e *Editor) drawSoundPanel(screen *ebiten.Image) {
	x := float32(sndPanelX - sndPanelMargin)
	y := float32(sndPanelY - sndPanelMargin)
	vector.FillRect(screen, x, y, sndPanelW, sndPanelH, colorPanelBg, false)
	vector.StrokeRect(screen, x, y, sndPanelW, sndPanelH, 1, colorBox, false)
	e.snd.Render(screen)
}
