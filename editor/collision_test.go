package editor

import (
	"testing"

	"linefire/asset"
)

func TestPlaceCircleCollision(t *testing.T) {
	e := newTestEditor()
	e.collKind = asset.CollisionCircle

	e.placeCollisionPoint(10, 10) // center
	if len(e.asset.Collisions) != 0 {
		t.Fatal("circle should not finalize on the first click")
	}
	e.placeCollisionPoint(13, 14) // radius point: dist = 5

	if len(e.asset.Collisions) != 1 {
		t.Fatalf("expected 1 collision, got %d", len(e.asset.Collisions))
	}
	s := e.asset.Collisions[0]
	if s.Kind != asset.CollisionCircle || s.Radius != 5 {
		t.Fatalf("unexpected circle: %+v", s)
	}
	if len(e.collDraft) != 0 {
		t.Fatal("draft should be cleared after finalizing")
	}
}

func TestPlaceRectCollision(t *testing.T) {
	e := newTestEditor()
	e.collKind = asset.CollisionRect

	e.placeCollisionPoint(0, 0)
	e.placeCollisionPoint(20, 10)

	if len(e.asset.Collisions) != 1 {
		t.Fatalf("expected 1 collision, got %d", len(e.asset.Collisions))
	}
	s := e.asset.Collisions[0]
	if s.Kind != asset.CollisionRect || len(s.Points) != 2 {
		t.Fatalf("unexpected rect: %+v", s)
	}
}

func TestPlaceTriangleCollision(t *testing.T) {
	e := newTestEditor()
	e.collKind = asset.CollisionTriangle

	e.placeCollisionPoint(0, 0)
	e.placeCollisionPoint(10, 0)
	if len(e.asset.Collisions) != 0 {
		t.Fatal("triangle should not finalize before the third point")
	}
	e.placeCollisionPoint(5, 8)

	if len(e.asset.Collisions) != 1 || len(e.asset.Collisions[0].Points) != 3 {
		t.Fatalf("unexpected triangle: %+v", e.asset.Collisions)
	}
	err := asset.Validate(e.asset)
	if err != nil {
		t.Fatalf("triangle collision invalid: %v", err)
	}
}

func TestCycleCollKind(t *testing.T) {
	e := newTestEditor()
	if e.collKind != asset.CollisionCircle {
		t.Fatalf("default collision kind should be circle")
	}
	e.cycleCollKind()
	if e.collKind != asset.CollisionRect {
		t.Fatalf("expected rect, got %s", e.collKind)
	}
	e.cycleCollKind()
	e.cycleCollKind()
	if e.collKind != asset.CollisionCircle {
		t.Fatalf("kind should wrap back to circle, got %s", e.collKind)
	}
}

func TestDeleteCollisionShapeByIndex(t *testing.T) {
	e := newTestEditor()
	e.asset.Collisions = []asset.CollisionShape{
		{Kind: asset.CollisionCircle, Points: []asset.Point{{X: 1, Y: 1}}, Radius: 2},
		{Kind: asset.CollisionRect, Points: []asset.Point{{X: 0, Y: 0}, {X: 4, Y: 4}}},
	}
	e.active = handle{kind: handleCollision, shapeIdx: 0}
	e.hasActive = true
	e.deleteSelected()

	if len(e.asset.Collisions) != 1 || e.asset.Collisions[0].Kind != asset.CollisionRect {
		t.Fatalf("wrong shape removed: %+v", e.asset.Collisions)
	}
}

func TestMoveTriangleVertex(t *testing.T) {
	e := newTestEditor()
	e.asset.Collisions = []asset.CollisionShape{
		{Kind: asset.CollisionTriangle, Points: []asset.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 8}}},
	}
	h := handle{kind: handleCollision, shapeIdx: 0, pointIdx: 2}
	e.moveHandle(h, 6, 9)

	got := e.asset.Collisions[0].Points[2]
	if got.X != 6 || got.Y != 9 {
		t.Fatalf("triangle vertex not moved: %+v", got)
	}
	// Other points untouched.
	if e.asset.Collisions[0].Points[0].X != 0 {
		t.Fatal("moving one vertex disturbed another")
	}
}
