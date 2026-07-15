package render

import (
	_ "embed"
	"image"
	"math"
	"sync"

	"github.com/crgimenes/devengine/log"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"linefire/asset"
)

//go:embed blur.kage
var blurSrc []byte

var (
	blurShaderOnce sync.Once
	blurShader     *ebiten.Shader
)

// loadBlurShader compiles the Gaussian blur shader once. On failure it logs and
// returns nil so the editor keeps working without glow.
func loadBlurShader() *ebiten.Shader {
	blurShaderOnce.Do(func() {
		s, err := ebiten.NewShader(blurSrc)
		if err != nil {
			log.Printf("glow: blur shader failed to compile: %v", err)
			return
		}
		blurShader = s
	})
	return blurShader
}

// GlowVariant selects the look of the bloom.
type GlowVariant int

const (
	GlowStable GlowVariant = iota // steady halo
	GlowPulse                     // halo pulses over time
	GlowLaser                     // bright tight core plus a wide soft halo
)

// String names the variant for the UI.
func (v GlowVariant) String() string {
	switch v {
	case GlowPulse:
		return "pulse"
	case GlowLaser:
		return "laser"
	default:
		return "stable"
	}
}

// GlowOptions controls a glow pass.
type GlowOptions struct {
	Variant    GlowVariant
	Time       float64 // seconds, for the pulse variant
	Intensity  float64 // base multiplier; 0 is treated as 1
	Spread     float64 // blur step in target pixels; 0 uses the variant default
	Iterations int     // separable blur iterations; 0 uses the variant default
	AntiAlias  bool
}

// glowDownscale is how much smaller the blur buffers are than the emissive: the
// bloom is a soft, low-frequency halo, so blurring at half resolution is visually
// indistinguishable while costing a quarter of the fill rate — the blur passes are
// the render's per-frame hot path (run once per motion-blur sub-frame). Emissive
// stays full-res so the bright sources are still crisp before they are blurred.
const glowDownscale = 2

// Glow renders an asset's strokes with an additive bloom. It owns offscreen
// buffers sized to the region it is used for; use one Glow per region (e.g. one
// for the canvas and one for the preview) so the buffers stay a stable size.
type Glow struct {
	w, h     int           // emissive / region size (full resolution)
	bw, bh   int           // blur buffer size (downscaled)
	emissive *ebiten.Image // full-res bright sources
	a, b     *ebiten.Image // half-res ping-pong for the separable blur
}

// NewGlow returns an empty glow renderer; buffers are created on first use.
func NewGlow() *Glow {
	return &Glow{}
}

func (g *Glow) ensure(w, h int) {
	if g.emissive != nil && g.w == w && g.h == h {
		return
	}
	g.w, g.h = w, h
	g.bw = max(1, w/glowDownscale)
	g.bh = max(1, h/glowDownscale)
	g.emissive = ebiten.NewImage(w, h)
	g.a = ebiten.NewImage(g.bw, g.bh)
	g.b = ebiten.NewImage(g.bw, g.bh)
}

// Apply adds a bloom for the given layers into dst, confined to region, using
// the screen-space view (the editors' CPU-transform path). The crisp geometry
// should already be drawn; this only adds the additive glow on top.
func (g *Glow) Apply(dst *ebiten.Image, layers []asset.Layer, view View, region image.Rectangle, opts GlowOptions) {
	g.Bloom(dst, region, opts, func(emissive *ebiten.Image) {
		local := View{
			OffsetX: view.OffsetX - float64(region.Min.X),
			OffsetY: view.OffsetY - float64(region.Min.Y),
			Scale:   view.Scale,
		}
		drawEmissive(emissive, layers, local, 1, opts.AntiAlias)
	})
}

// Bloom blurs whatever fillEmissive draws into the (cleared) emissive buffer and
// adds it to dst with the variant's intensity. This decouples the bloom from how
// the bright source is produced: the editors fill it via a View; the game draws
// a world mesh with the camera GeoM (so the GPU does the transform). It is a
// no-op if the shader is unavailable.
func (g *Glow) Bloom(dst *ebiten.Image, region image.Rectangle, opts GlowOptions, fillEmissive func(emissive *ebiten.Image)) {
	shader := loadBlurShader()
	if shader == nil {
		return
	}
	w, h := region.Dx(), region.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	g.ensure(w, h)

	g.emissive.Clear()
	fillEmissive(g.emissive)

	intensity := opts.Intensity
	if intensity == 0 {
		intensity = 1
	}
	if opts.Variant == GlowPulse {
		intensity *= 0.55 + 0.45*math.Sin(opts.Time*4)
	}
	spread := opts.Spread
	if spread <= 0 {
		spread = 2.5
	}
	iters := opts.Iterations
	if iters <= 0 {
		iters = 2
	}

	var geo ebiten.GeoM
	geo.Translate(float64(region.Min.X), float64(region.Min.Y))

	if opts.Variant == GlowLaser {
		g.bloomPass(dst, shader, geo, spread*1.6, iters, 0.6*intensity) // wide soft halo
		g.bloomPass(dst, shader, geo, spread*0.4, 1, 1.3*intensity)     // tight bright core
		return
	}
	g.bloomPass(dst, shader, geo, spread, iters, intensity)
}

// bloomPass blurs the emissive buffer (separably, iters times at the given spread)
// at reduced resolution and adds the upscaled result to dst with the given
// brightness gain. The emissive is downsampled once, blurred cheaply, then scaled
// back up on the additive draw; the spread is divided by the downscale so the halo
// keeps the same on-screen width.
func (g *Glow) bloomPass(dst *ebiten.Image, shader *ebiten.Shader, geo ebiten.GeoM, spread float64, iters int, gain float64) {
	inv := 1.0 / float64(glowDownscale)

	// Downsample the full-res emissive into the half-res ping-pong buffer.
	g.a.Clear()
	var down ebiten.DrawImageOptions
	down.GeoM.Scale(inv, inv)
	down.Filter = ebiten.FilterLinear
	g.a.DrawImage(g.emissive, &down)

	src, dst2 := g.a, g.b
	step := spread * inv // the tap step is in blur-buffer pixels
	for range iters {
		g.blur(dst2, src, shader, step, 0)
		g.blur(src, dst2, shader, 0, step)
	}

	op := &ebiten.DrawImageOptions{Blend: ebiten.BlendLighter}
	op.GeoM.Scale(float64(glowDownscale), float64(glowDownscale))
	op.GeoM.Concat(geo)
	op.Filter = ebiten.FilterLinear
	op.ColorScale.Scale(float32(gain), float32(gain), float32(gain), float32(gain))
	dst.DrawImage(src, op)
}

// blur runs one separable Gaussian pass from src into dst along (dx, dy), over the
// blur-buffer resolution.
func (g *Glow) blur(dst, src *ebiten.Image, shader *ebiten.Shader, dx, dy float64) {
	dst.Clear()
	op := &ebiten.DrawRectShaderOptions{}
	op.Images[0] = src
	op.Uniforms = map[string]any{"Direction": []float32{float32(dx), float32(dy)}}
	dst.DrawRectShader(g.bw, g.bh, shader, op)
}

// drawEmissive draws every visible layer's strokes scaled by the layer glow and
// the global intensity, building the source the bloom blurs. Fills do not glow.
func drawEmissive(dst *ebiten.Image, layers []asset.Layer, v View, intensity float64, antialias bool) {
	for i := range layers {
		l := &layers[i]
		if l.Hidden || l.StrokeWidth <= 0 {
			continue
		}
		col, ok := ParseColor(l.Stroke)
		if !ok {
			continue
		}
		scale := l.Glow * intensity
		if scale <= 0 {
			continue
		}
		var cs ebiten.ColorScale
		cs.ScaleWithColor(col)
		cs.Scale(float32(scale), float32(scale), float32(scale), 1)

		stroke := &vector.StrokeOptions{
			Width:    float32(l.StrokeWidth * v.Scale),
			LineCap:  vector.LineCapRound,
			LineJoin: vector.LineJoinRound,
		}
		for _, p := range l.Paths {
			path := buildPath(p, v)
			vector.StrokePath(dst, path, stroke, &vector.DrawPathOptions{
				AntiAlias:  antialias,
				ColorScale: cs,
			})
		}
	}
}
