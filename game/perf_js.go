//go:build js

package game

// The web render profile. WebGL cannot afford the native pipeline: a phone's 3×
// device scale factor makes every full-screen pass (the bloom blurs, the fog mask,
// each motion-blur accumulation blit) ~9× the pixels of the logical view, and the
// game crawls — the iPhone playtest that motivated this ran at a fraction of full
// speed, and even a desktop browser dragged at 2×. Rendering at 1× and letting the
// browser upscale is nearly invisible on a CRT-glow look (the bloom is blurry by
// design), and the blur budget drops with it: fewer, cheaper accumulation blits.
const (
	renderScaleCap = 1.0
	blurSampleCap  = 3

	// forceGCEvery runs the Go GC every N frames (~10s). Ebiten frees dead GPU
	// textures via FINALIZERS, and with a small, stable Go heap (the soak test pins
	// it) automatic GC runs rarely — so the textures of every swapped map, attract
	// arena and Rift room pile up browser-side until iOS kills the tab (the "starts
	// fast, slowly dies, reloads" cycle from the iPhone playtest). A periodic GC
	// bounds that lag for a few ms every 10s.
	forceGCEvery = 600
)
