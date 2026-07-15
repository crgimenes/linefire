package render

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"linefire/asset"
)

// glowColorScale brightens the emissive source feeding the bloom; the game then
// controls the final halo strength via GlowOptions.Intensity.
const glowColorScale = 1.0

// minStrokePx keeps a transformed stroke from collapsing below ~1 device pixel,
// where a sub-pixel line would shimmer as the camera rotates.
const minStrokePx = 1.0

type meshPath struct {
	path        *vector.Path
	bounds      image.Rectangle
	fill        color.RGBA
	fillVisible bool
	stroke      color.RGBA
	strokeWidth float64
}

// Mesh is cached vector path data in world/asset coordinates. Draw transforms
// only visible paths and lets Ebitengine's modern vector renderer handle the
// final stroke/fill quality, avoiding rotated bitmap artifacts.
type Mesh struct {
	paths []meshPath
	work  vector.Path
}

// Empty reports whether the mesh has nothing to draw.
func (m *Mesh) Empty() bool {
	return m == nil || len(m.paths) == 0
}

// Draw renders cached geometry into dst, transformed by geo.
func (m *Mesh) Draw(dst *ebiten.Image, geo ebiten.GeoM, smooth bool) {
	m.DrawTinted(dst, geo, smooth, ebiten.ColorScale{})
}

// DrawTinted renders the cached geometry like Draw, but multiplies every path color
// by tint — e.g. a dimmed, translucent scale to draw a faded ghost. The zero
// ColorScale is identity, so Draw is just DrawTinted with no tint.
func (m *Mesh) DrawTinted(dst *ebiten.Image, geo ebiten.GeoM, smooth bool, tint ebiten.ColorScale) {
	if m.Empty() {
		return
	}

	screen := dst.Bounds()
	scale := geoScale(geo)
	for i := range m.paths {
		p := &m.paths[i]
		if !meshPathVisible(p.bounds, geo, screen) {
			continue
		}

		m.work.Reset()
		m.work.AddPath(p.path, &vector.AddPathOptions{GeoM: geo})
		if p.fillVisible {
			var cs ebiten.ColorScale
			cs.ScaleWithColor(p.fill)
			cs.ScaleWithColorScale(tint)
			vector.FillPath(dst, &m.work, &vector.FillOptions{}, &vector.DrawPathOptions{
				AntiAlias:  smooth,
				ColorScale: cs,
			})
		}
		if p.strokeWidth > 0 {
			var cs ebiten.ColorScale
			cs.ScaleWithColor(p.stroke)
			cs.ScaleWithColorScale(tint)
			width := math.Max(p.strokeWidth*scale, minStrokePx)
			vector.StrokePath(dst, &m.work, &vector.StrokeOptions{
				Width:    float32(width),
				LineCap:  vector.LineCapRound,
				LineJoin: vector.LineJoinRound,
			}, &vector.DrawPathOptions{
				AntiAlias:  smooth,
				ColorScale: cs,
			})
		}
	}
}

// BuildLayersMesh caches the crisp geometry of layers: fills for closed paths
// and strokes for visible strokes.
func BuildLayersMesh(layers []asset.Layer) *Mesh {
	m := &Mesh{}
	for i := range layers {
		layer := &layers[i]
		if layer.Hidden {
			continue
		}

		fillCol, fillVisible := ParseColor(layer.Fill)
		strokeCol, strokeVisible := ParseColor(layer.Stroke)
		for _, path := range layer.Paths {
			fillOK := fillVisible && path.Closed()
			strokeWidth := 0.0
			if strokeVisible && layer.StrokeWidth > 0 {
				strokeWidth = layer.StrokeWidth
			}
			if !fillOK && strokeWidth == 0 {
				continue
			}
			m.paths = append(m.paths, meshPath{
				path:        buildPath(path, View{Scale: 1}),
				bounds:      assetPathBounds(path, strokeWidth/2+2),
				fill:        fillCol,
				fillVisible: fillOK,
				stroke:      strokeCol,
				strokeWidth: strokeWidth,
			})
		}
	}
	return m
}

// BuildGlowMesh caches only glowing strokes for the bloom emissive source.
func BuildGlowMesh(layers []asset.Layer) *Mesh {
	m := &Mesh{}
	for i := range layers {
		layer := &layers[i]
		if layer.Hidden || layer.StrokeWidth <= 0 || layer.Glow <= 0 {
			continue
		}
		strokeCol, strokeVisible := ParseColor(layer.Stroke)
		if !strokeVisible {
			continue
		}
		for _, path := range layer.Paths {
			m.paths = append(m.paths, meshPath{
				path:        buildPath(path, View{Scale: 1}),
				bounds:      assetPathBounds(path, layer.StrokeWidth/2+2),
				stroke:      scaleColor(strokeCol, layer.Glow*glowColorScale),
				strokeWidth: layer.StrokeWidth,
			})
		}
	}
	return m
}

func geoScale(geo ebiten.GeoM) float64 {
	sx := math.Hypot(geo.Element(0, 0), geo.Element(1, 0))
	sy := math.Hypot(geo.Element(0, 1), geo.Element(1, 1))
	switch {
	case sx == 0 && sy == 0:
		return 1
	case sx == 0:
		return sy
	case sy == 0:
		return sx
	default:
		return (sx + sy) / 2
	}
}

func assetPathBounds(path asset.Path, margin float64) image.Rectangle {
	path = path.Flatten() // curves bound by their segments, not the control hull
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	found := false

	for _, command := range path.Commands {
		switch command.Op {
		case asset.OpMoveTo, asset.OpLineTo:
			minX = math.Min(minX, command.X)
			minY = math.Min(minY, command.Y)
			maxX = math.Max(maxX, command.X)
			maxY = math.Max(maxY, command.Y)
			found = true
		}
	}
	if !found {
		return image.Rectangle{}
	}

	return image.Rect(
		int(math.Floor(minX-margin)),
		int(math.Floor(minY-margin)),
		int(math.Ceil(maxX+margin)),
		int(math.Ceil(maxY+margin)),
	)
}

func meshPathVisible(bounds image.Rectangle, geo ebiten.GeoM, screen image.Rectangle) bool {
	if bounds.Empty() {
		return false
	}
	minX, minY, maxX, maxY := transformedBounds(bounds, geo)
	return maxX >= float64(screen.Min.X) &&
		minX <= float64(screen.Max.X) &&
		maxY >= float64(screen.Min.Y) &&
		minY <= float64(screen.Max.Y)
}

func transformedBounds(rect image.Rectangle, geo ebiten.GeoM) (float64, float64, float64, float64) {
	x0, y0 := geo.Apply(float64(rect.Min.X), float64(rect.Min.Y))
	x1, y1 := geo.Apply(float64(rect.Max.X), float64(rect.Min.Y))
	x2, y2 := geo.Apply(float64(rect.Max.X), float64(rect.Max.Y))
	x3, y3 := geo.Apply(float64(rect.Min.X), float64(rect.Max.Y))

	minX := math.Min(math.Min(x0, x1), math.Min(x2, x3))
	minY := math.Min(math.Min(y0, y1), math.Min(y2, y3))
	maxX := math.Max(math.Max(x0, x1), math.Max(x2, x3))
	maxY := math.Max(math.Max(y0, y1), math.Max(y2, y3))
	return minX, minY, maxX, maxY
}

// scaleColor multiplies the RGB channels by f (clamped to 0..255), keeping alpha.
func scaleColor(c color.RGBA, f float64) color.RGBA {
	clamp := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}
	return color.RGBA{
		R: clamp(float64(c.R) * f),
		G: clamp(float64(c.G) * f),
		B: clamp(float64(c.B) * f),
		A: c.A,
	}
}
