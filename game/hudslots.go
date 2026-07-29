package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/render"
)

// The slot strip: four tiles at the bottom-right showing what each slot holds — the two
// live weapons (cycled with 1/2; the nose fires with Space, the turret with left mouse),
// the shield, and the combat computer (4 toggles it). Weapon and computer tiles draw the
// item's actual pickup DRAWING; the shield tile lights up with the active shield type. It
// is the visual half of the arsenal.

// hudDimIcon fades an inactive tile's icon (shield off, computer off).
var hudDimIcon = func() ebiten.ColorScale {
	var c ebiten.ColorScale
	c.ScaleAlpha(0.35)
	return c
}()

// iconMesh is a cached pickup-asset mesh with the geometry needed to fit it in a tile.
type iconMesh struct {
	mesh   *render.Mesh
	origin asset.Point
	ext    float64 // max(width, height): the side the tile scales against
}

// icon lazily loads (and caches on the Game) an asset's mesh by name, so a tile draws the
// real drawing, not a placeholder. A missing asset caches as a nil mesh, so the disk is
// read at most once per name and the tile just stays empty.
func (g *Game) icon(name string) *iconMesh {
	if g.iconCache == nil {
		g.iconCache = map[string]*iconMesh{}
	}
	if ic, ok := g.iconCache[name]; ok {
		return ic
	}
	ic := &iconMesh{ext: 24}
	a, err := filoio.LoadAssetFS(g.content, g.mapDir, name)
	if err == nil {
		ic.mesh = render.BuildLayersMesh(a.Layers)
		ic.origin = a.Origin
		if e := math.Max(a.Size.W, a.Size.H); e > 0 {
			ic.ext = e
		}
	}
	g.iconCache[name] = ic
	return ic
}

// drawWeaponSlots draws the four slot tiles at the bottom-right of the HUD.
func (g *Game) drawWeaponSlots(dst *ebiten.Image) {
	const tile, gap, margin = 34.0, 6.0, 12.0
	labels := [4]string{"1 Sp", "2 L", "3", "4"}
	w := float64(dst.Bounds().Dx())
	h := float64(dst.Bounds().Dy())
	total := 4*tile + 3*gap
	x0 := w - margin - total
	y := h - margin - tile
	for i := range 4 {
		bx := x0 + float64(i)*(tile+gap)
		vector.FillRect(dst, float32(bx), float32(y), tile, tile, hudPanelBg, false)
		vector.StrokeRect(dst, float32(bx), float32(y), tile, tile, 1, hudPanelBorder, true)
		ebitenutil.DebugPrintAt(dst, labels[i], int(bx)+3, int(y)+2)
		g.drawSlotIcon(dst, i, bx+tile/2, y+tile/2+5, tile-12)
	}
}

// drawSlotIcon draws the item in tile i: weapons in 0/1, the shield in 2, the computer in
// 3. An empty/inactive tile shows a dash or a dimmed icon.
func (g *Game) drawSlotIcon(dst *ebiten.Image, i int, cx, cy, box float64) {
	switch i {
	case 0, 1:
		if !g.slots[i].filled {
			g.drawTileDash(dst, cx, cy)
			return
		}
		g.drawIcon(dst, "wpn_"+weaponKeys[g.arsenal[g.slotArsIdx[i]]], cx, cy, box, false)
	case 2: // shield: light the active type, dash when none is charged
		switch {
		case g.bubbleTime > 0:
			g.drawIcon(dst, "bubblepower", cx, cy, box, false)
		case g.reflectTime > 0:
			g.drawIcon(dst, "reflectpower", cx, cy, box, false)
		default:
			g.drawTileDash(dst, cx, cy)
		}
	case 3: // computer: full when engaged, dimmed when off, dash when not yet salvaged
		if !g.hasComputer {
			g.drawTileDash(dst, cx, cy)
			return
		}
		g.drawIcon(dst, "computer", cx, cy, box, !g.autoFire)
	}
}

func (g *Game) drawTileDash(dst *ebiten.Image, cx, cy float64) {
	ebitenutil.DebugPrintAt(dst, "-", int(cx)-2, int(cy)-4)
}

// drawIcon draws an asset's pickup mesh centered at (cx,cy), scaled so its longest side is
// `box` logical pixels; dim fades it (an inactive slot).
func (g *Game) drawIcon(dst *ebiten.Image, name string, cx, cy, box float64, dim bool) {
	ic := g.icon(name)
	if ic == nil || ic.mesh == nil || ic.mesh.Empty() {
		return
	}
	scale := box / ic.ext
	var geo ebiten.GeoM
	geo.Translate(-ic.origin.X, -ic.origin.Y)
	geo.Scale(scale, scale)
	geo.Translate(cx, cy)
	if dim {
		ic.mesh.DrawTinted(dst, geo, true, hudDimIcon)
		return
	}
	ic.mesh.Draw(dst, geo, true)
}
