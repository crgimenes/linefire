package render

import (
	_ "embed"
	"sync"

	"github.com/crgimenes/devengine/log"
	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed crt.kage
var crtSrc []byte

var (
	crtShaderOnce sync.Once
	crtShader     *ebiten.Shader
)

// loadCRTShader compiles the CRT shader once. On failure it logs and returns
// nil so the editor falls back to presenting the scene unmodified.
func loadCRTShader() *ebiten.Shader {
	crtShaderOnce.Do(func() {
		s, err := ebiten.NewShader(crtSrc)
		if err != nil {
			log.Printf("crt: shader failed to compile: %v", err)
			return
		}
		crtShader = s
	})
	return crtShader
}

// CRTOptions are the tunable parameters of the CRT post-process.
type CRTOptions struct {
	Curvature     float64
	ScanIntensity float64
	Aberration    float64 // pixels
	Vignette      float64
}

// DefaultCRTOptions returns a subtle CRT look — gentle curvature and faint
// scanlines/aberration, meant to suggest a CRT rather than distort heavily.
func DefaultCRTOptions() CRTOptions {
	return CRTOptions{
		Curvature:     0.04,
		ScanIntensity: 0.18,
		Aberration:    0.6,
		Vignette:      0.20,
	}
}

// CRT applies the CRT post-process shader.
type CRT struct{}

// NewCRT returns a CRT post-processor.
func NewCRT() *CRT {
	return &CRT{}
}

// Present draws src onto dst through the CRT shader, positioned by geo. If the
// shader is unavailable it copies src across unchanged so the editor still
// works.
func (c *CRT) Present(dst, src *ebiten.Image, geo ebiten.GeoM, opts CRTOptions) {
	shader := loadCRTShader()
	if shader == nil {
		dst.DrawImage(src, &ebiten.DrawImageOptions{GeoM: geo})
		return
	}
	b := src.Bounds()
	op := &ebiten.DrawRectShaderOptions{GeoM: geo}
	op.Images[0] = src
	op.Uniforms = map[string]any{
		"Curvature":     float32(opts.Curvature),
		"ScanIntensity": float32(opts.ScanIntensity),
		"Aberration":    float32(opts.Aberration),
		"Vignette":      float32(opts.Vignette),
	}
	dst.DrawRectShader(b.Dx(), b.Dy(), shader, op)
}
