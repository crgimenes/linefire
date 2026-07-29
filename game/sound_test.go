package game

import (
	"testing"

	"github.com/crgimenes/linefire/asset"
)

// The pure synthesis (variations, loops, stereo packing) is tested in the sfx
// package; here we test the game-side resolution: which sound an event plays.

func TestRenderVariationsDelegatesToSfx(t *testing.T) {
	vs := renderVariations("explosion", 202)
	if len(vs) == 0 {
		t.Fatal("a known preset should render variations")
	}
	if renderVariations("not-a-preset", 0) != nil {
		t.Fatal("an unknown base should render nil")
	}
}

func TestResolveEventFallbackAndOverride(t *testing.T) {
	// No asset: the per-event fallback is used.
	req, ok := resolveEvent(nil, "pickup")
	if !ok || req != eventFallback["pickup"] {
		t.Fatalf("nil asset should use the pickup fallback, got %+v ok=%v", req, ok)
	}

	// Unknown event, no asset: not ok.
	if _, ok := resolveEvent(nil, "nope"); ok {
		t.Fatal("an unknown event without a fallback should not resolve")
	}

	// Asset override: base+seed replace the fallback; an unset volume keeps it.
	a := &asset.Asset{Sounds: []asset.Sound{{Event: "pickup", Base: "blip", Seed: 9}}}
	req, ok = resolveEvent(a, "pickup")
	if !ok || req.base != "blip" || req.seed != 9 {
		t.Fatalf("asset should override base/seed, got %+v", req)
	}
	if req.vol != eventFallback["pickup"].vol {
		t.Fatalf("an unset volume should keep the fallback, got %v", req.vol)
	}

	// A custom event declared only on the asset resolves.
	a = &asset.Asset{Sounds: []asset.Sound{{Event: "thruster", Base: "jump", Volume: 0.5}}}
	req, ok = resolveEvent(a, "thruster")
	if !ok || req.base != "jump" || req.vol != 0.5 {
		t.Fatalf("asset-only event should resolve, got %+v ok=%v", req, ok)
	}

	// A muted declared sound silences the event entirely — the fallback must NOT
	// kick in ("switch off without deleting").
	a = &asset.Asset{Sounds: []asset.Sound{{Event: "pickup", Base: "blip", Muted: true}}}
	if _, ok := resolveEvent(a, "pickup"); ok {
		t.Fatal("a muted sound must silence the event, not fall back")
	}
}
