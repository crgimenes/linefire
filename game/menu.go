package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	ui "github.com/crgimenes/minigui"
)

// Pause-menu geometry in logical screen space (scaled to device resolution when
// drawn). The minigui panel auto-sizes to its content, so menuW/menuH are only a
// rough estimate used to center it; the panel never clips its widgets.
const (
	menuItemW = 140 // widest row (the volume slider); the buttons share it
	menuRowH  = 24  // approx widget row height, for centering only
)

// menuW / menuH estimate the panel size for centering (the real size is auto).
func (g *Game) menuW() float64 { return menuItemW + 40 }
func (g *Game) menuH() float64 { return 7 * menuRowH } // title + volume + mute + resume + restart + quit

// menuOrigin is the panel's top-left, centered in the current logical window.
func (g *Game) menuOrigin() (float64, float64) {
	w, h := g.logicalSize()
	return float64(w)/2 - g.menuW()/2, float64(h)/2 - g.menuH()/2
}

var menuDim = color.RGBA{0x00, 0x00, 0x00, 0xb4}

// updatePauseMenu runs the ESC pause menu: a titled minigui panel with the audio
// settings (volume slider + mute, applied live and persisted on leave) plus
// Resume (or Esc again) and Quit. The mouse is divided by the device ratio so
// hit-testing matches the scaled-up drawing.
func (g *Game) updatePauseMenu() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.closePauseMenu()
		return nil
	}

	in := ui.InputFromEbiten()
	if g.dpr > 0 {
		in.MouseX /= g.dpr
		in.MouseY /= g.dpr
	}
	// Buttons/toggles size to SetItemWidth but the slider sizes to the style's
	// FieldW; align both so every row in the menu shares one width.
	style := g.menu.Style()
	if style.FieldW != menuItemW {
		style.FieldW = menuItemW
		g.menu.SetStyle(style)
	}

	mx, my := g.menuOrigin()
	g.menu.Begin(in, mx, my)
	g.menu.BeginPanel("PAUSED", mx, my) // auto-sizes to the content below
	g.menu.SetItemWidth(menuItemW)
	if g.sfx != nil {
		g.menu.Label("volume:")
		if g.menu.Slider("menu.vol", &g.sfx.master, 0, 1) {
			g.sfx.setMaster(g.sfx.master) // re-level the running loops/music live
		}
		if g.menu.Toggle("menu.mute", "muted (F6)", g.sfx.muted) {
			g.sfx.muted = !g.sfx.muted
			if g.sfx.muted {
				g.sfx.stopLoops()
				g.sfx.stopMusic()
			}
		}
	}
	resume := g.menu.Button("resume", "Resume")
	restart := g.menu.Button("restart", "Restart")
	// Under the editor this button does not quit anything — it hands the window back.
	// Saying "Quit" there would read as "lose my map", which is the opposite of true.
	leaveLabel := "Quit"
	if g.embedded {
		leaveLabel = "Back to editor"
	}
	quit := g.menu.Button("quit", leaveLabel)
	g.menu.EndPanel()
	g.menu.End()

	if resume {
		g.closePauseMenu()
		g.fireBlocked = true // the resuming click must not also fire the turret
	}
	if restart {
		g.sfx.saveConfig()  // persist any audio change before the state is wiped
		g.restartCampaign() // fresh run from the first map (also drops the pause)
		g.fireBlocked = true
		return nil
	}
	if quit {
		return g.endGame()
	}
	return nil
}

// closePauseMenu resumes play and persists whatever audio settings were touched.
func (g *Game) closePauseMenu() {
	g.paused = false
	g.sfx.saveConfig()
}

// drawPauseMenu dims the frozen world and draws the menu (its panel chrome is part
// of the minigui draw list) through the shared logical overlay, so it reads the
// same at any DPI.
func (g *Game) drawPauseMenu(screen *ebiten.Image) {
	g.presentOverlay(screen, func(dst *ebiten.Image) {
		w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
		vector.FillRect(dst, 0, 0, float32(w), float32(h), menuDim, false)
		g.menu.Render(dst)
	})
}
