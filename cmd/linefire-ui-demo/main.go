// Command linefire-ui-demo is a sandbox for the ui package: a panel with a
// label, a button and an editable text field, so the immediate-mode toolkit can
// be exercised on its own before it is wired into the editors.
package main

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	ui "github.com/crgimenes/minigui"
)

type demo struct {
	gui   ui.Context
	count int
	name  string
	items []string
	sel   int
}

func (d *demo) Update() error {
	in := ui.InputFromEbiten()
	d.gui.Begin(in, 40, 40)
	d.gui.Label("Linefire UI demo")
	if d.gui.Button("inc", fmt.Sprintf("count: %d", d.count)) {
		d.count++
	}
	d.gui.TextField("name", &d.name)
	d.gui.List("items", d.items, &d.sel)
	d.gui.Label(fmt.Sprintf("selected: %s", d.items[d.sel]))
	d.gui.End()
	return nil
}

func (d *demo) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x06, 0x08, 0x0c, 0xff})
	d.gui.Render(screen)
}

func (d *demo) Layout(int, int) (int, int) {
	return 520, 420
}

func main() {
	items := make([]string, 12)
	for i := range items {
		items[i] = fmt.Sprintf("item %02d", i)
	}

	ebiten.SetWindowSize(520, 420)
	ebiten.SetWindowTitle("Linefire UI demo")
	err := ebiten.RunGame(&demo{name: "world", items: items})
	if err != nil {
		panic(err)
	}
}
