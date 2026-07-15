package editor

import (
	"testing"

	"linefire/asset"
)

func TestSelectHardpointSetsActiveHandle(t *testing.T) {
	a := asset.New()
	a.Hardpoints = []asset.Hardpoint{
		{Name: "a", Kind: asset.KindWeapon, X: 5, Y: 6},
		{Name: "b", Kind: asset.KindThruster, X: 7, Y: 8},
	}
	e := New(a, "")

	e.selectHardpoint(1)
	if !e.hasActive || e.active.kind != handleHardpoint || e.active.index != 1 {
		t.Fatalf("active not set to hardpoint 1: %+v hasActive=%v", e.active, e.hasActive)
	}
	if e.active.x != 7 || e.active.y != 8 {
		t.Fatalf("active position = (%v,%v), want (7,8)", e.active.x, e.active.y)
	}

	e.selectHardpoint(5) // out of range
	if e.hasActive {
		t.Fatal("out-of-range selection should clear the active handle")
	}
}

func TestHardpointPanelHover(t *testing.T) {
	e := New(asset.New(), "")

	e.hpPanel = false
	if e.hpPanelHovered(hpPanelX, hpPanelY) {
		t.Fatal("a closed panel is never hovered")
	}

	e.hpPanel = true
	if !e.hpPanelHovered(hpPanelX, hpPanelY) {
		t.Fatal("cursor inside the open panel should hover")
	}
	if e.hpPanelHovered(900, 600) {
		t.Fatal("cursor far from the panel should not hover")
	}
}

func TestDeleteToolRemovesHandleAtCursor(t *testing.T) {
	a := asset.New()
	a.Hardpoints = []asset.Hardpoint{{Name: "g", Kind: asset.KindWeapon, X: 20, Y: 20}}
	e := New(a, "")
	e.tool = toolDelete

	// Nothing under a far-away cursor: the hardpoint stays.
	e.deleteAtCursor(-1000, -1000)
	if len(e.asset.Hardpoints) != 1 {
		t.Fatalf("delete on empty space should keep the hardpoint, got %d", len(e.asset.Hardpoints))
	}

	// Click exactly on the hardpoint's screen position: it is removed.
	sx, sy := e.canvasView().Project(20, 20)
	e.deleteAtCursor(float64(sx), float64(sy))
	if len(e.asset.Hardpoints) != 0 {
		t.Fatalf("delete tool should remove the hardpoint, %d left", len(e.asset.Hardpoints))
	}
}
