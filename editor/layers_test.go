package editor

import (
	"testing"

	"github.com/crgimenes/linefire/asset"
)

func TestAddAndSelectLayer(t *testing.T) {
	e := newTestEditor() // one default layer
	e.addLayer()
	if len(e.asset.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(e.asset.Layers))
	}
	if e.layerIdx != 1 {
		t.Fatalf("new layer should be current, layerIdx = %d", e.layerIdx)
	}

	// Tab wraps around the layer list.
	e.selectLayer(1)
	if e.layerIdx != 0 {
		t.Fatalf("selectLayer(+1) should wrap to 0, got %d", e.layerIdx)
	}
	e.selectLayer(-1)
	if e.layerIdx != 1 {
		t.Fatalf("selectLayer(-1) should wrap to 1, got %d", e.layerIdx)
	}
}

func TestDeleteLayerKeepsAtLeastOne(t *testing.T) {
	e := newTestEditor()
	e.deleteLayer() // only one layer: must be refused
	if len(e.asset.Layers) != 1 {
		t.Fatalf("the last layer must not be removable, got %d", len(e.asset.Layers))
	}

	e.addLayer()
	e.addLayer() // 3 layers, current = 2
	e.deleteLayer()
	if len(e.asset.Layers) != 2 {
		t.Fatalf("expected 2 layers after delete, got %d", len(e.asset.Layers))
	}
	if e.layerIdx != 1 {
		t.Fatalf("layerIdx should clamp to 1, got %d", e.layerIdx)
	}
}

func TestMoveLayerReordersDrawOrder(t *testing.T) {
	e := newTestEditor()
	e.asset.Layers[0].Name = "a"
	e.addLayer()
	e.currentLayer().Name = "b" // layers: [a, b], current = 1 (b)

	e.moveLayer(-1) // move b down/behind -> [b, a]
	if e.asset.Layers[0].Name != "b" || e.asset.Layers[1].Name != "a" {
		t.Fatalf("unexpected order: %s, %s", e.asset.Layers[0].Name, e.asset.Layers[1].Name)
	}
	if e.layerIdx != 0 {
		t.Fatalf("layerIdx should follow the moved layer, got %d", e.layerIdx)
	}

	// Moving past the edge is a no-op.
	e.moveLayer(-1)
	if e.asset.Layers[0].Name != "b" {
		t.Fatalf("move past edge should be a no-op")
	}
}

func TestToggleLayerVisibility(t *testing.T) {
	e := newTestEditor()
	if e.currentLayer().Hidden {
		t.Fatal("layers start visible")
	}
	e.toggleLayerVisibility()
	if !e.currentLayer().Hidden {
		t.Fatal("layer should be hidden after toggle")
	}
	e.toggleLayerVisibility()
	if e.currentLayer().Hidden {
		t.Fatal("layer should be visible again")
	}
}

func TestHiddenLayerExcludedFromHandles(t *testing.T) {
	e := newTestEditor()
	e.asset.Layers[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 10, Y: 10},
	}}}
	// Origin is always a handle; the vertex adds one more when visible.
	visible := len(e.collectHandles())
	e.asset.Layers[0].Hidden = true
	hidden := len(e.collectHandles())
	if hidden >= visible {
		t.Fatalf("hidden layer vertices must be excluded: visible=%d hidden=%d", visible, hidden)
	}
}

func TestCurrentLayerClampsIndex(t *testing.T) {
	e := newTestEditor()
	e.layerIdx = 99 // out of range
	if e.currentLayer() != &e.asset.Layers[0] {
		t.Fatal("currentLayer should clamp an out-of-range index")
	}
	if e.layerIdx != 0 {
		t.Fatalf("layerIdx should be clamped to 0, got %d", e.layerIdx)
	}
}
