package render

import "testing"

func TestGlowVariantString(t *testing.T) {
	cases := map[GlowVariant]string{
		GlowStable: "stable",
		GlowPulse:  "pulse",
		GlowLaser:  "laser",
	}
	for v, want := range cases {
		got := v.String()
		if got != want {
			t.Errorf("GlowVariant(%d).String() = %q, want %q", v, got, want)
		}
	}
}

func TestNewGlow(t *testing.T) {
	g := NewGlow()
	if g == nil {
		t.Fatal("NewGlow returned nil")
	}
	if g.emissive != nil {
		t.Fatal("buffers should be created lazily, not in NewGlow")
	}
}
