//go:build !js

package game

// The native render profile: render at the display's full device resolution with
// the full motion-blur budget — Metal/DirectX/GL absorb it comfortably.
const (
	renderScaleCap = 0 // 0 = uncapped: use the OS device scale factor as-is
	blurSampleCap  = maxBlurSamples
	forceGCEvery   = 0   // native GC pacing is fine; finalizers run promptly enough
	fogTexScale    = 1.5 // fog texture resolution: texels per world unit (crisp when scaled up)
)
