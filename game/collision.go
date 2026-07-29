package game

import (
	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
	"math"
)

// assetRadius returns an asset's collision radius from its first circle
// collision shape, or a small default (also used for nil assets).
func assetRadius(a *asset.Asset) float64 {
	if a == nil {
		return 6
	}
	for _, s := range a.Collisions {
		if s.Kind == asset.CollisionCircle && len(s.Points) == 1 && s.Radius > 0 {
			return s.Radius
		}
	}
	return 6
}

// wallSegments flattens every wall path into line segments in world space.
func wallSegments(l *level.Level) []segment {
	var segs []segment
	for li := range l.Walls {
		for _, p := range l.Walls[li].Paths {
			p = p.Flatten() // curved walls collide as their flattened segments
			var first, prev asset.Point
			have := false
			for _, c := range p.Commands {
				switch c.Op {
				case asset.OpMoveTo:
					first = asset.Point{X: c.X, Y: c.Y}
					prev = first
					have = true
				case asset.OpLineTo:
					if have {
						segs = append(segs, segment{prev.X, prev.Y, c.X, c.Y})
					}
					prev = asset.Point{X: c.X, Y: c.Y}
					have = true
				case asset.OpClose:
					if have {
						segs = append(segs, segment{prev.X, prev.Y, first.X, first.Y})
					}
				}
			}
		}
	}
	return segs
}

// lineOfSight reports whether the straight segment from (ax,ay) to (bx,by) is
// clear of walls — used so an enemy only fires when it can actually see the
// player instead of shooting into a wall.
func (g *Game) lineOfSight(ax, ay, bx, by float64) bool {
	// SPIKE: with a region field the rock — including anything the player dug — is the
	// truth, so sight travels down a fresh tunnel. Procedural maps have no field and
	// keep the segment test.
	if g.flood != nil {
		return !g.rockOnSegment(ax, ay, bx, by, 0)
	}
	for _, s := range g.segs {
		if segmentsIntersect(ax, ay, bx, by, s.ax, s.ay, s.bx, s.by) {
			return false
		}
	}
	return true
}

// rockOnSegment walks (ax,ay)-(bx,by) at half-cell steps and reports whether any point
// has less than `radius` clearance from rock (radius 0 = the point is inside rock).
// Returns true at the first blocked sample.
func (g *Game) rockOnSegment(ax, ay, bx, by, radius float64) bool {
	f := g.flood
	length := math.Hypot(bx-ax, by-ay)
	n := max(int(length/(f.cell*0.5)), 1)
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		x, y := ax+(bx-ax)*t, ay+(by-ay)*t
		if radius <= 0 {
			if f.rockAt(x, y) {
				return true
			}
			continue
		}
		if f.clearanceAt(x, y) < radius {
			return true
		}
	}
	return false
}

// clearPath reports whether a disc of the given radius can travel the straight
// segment (ax,ay)-(bx,by) without any wall coming within the radius. Path
// smoothing uses it so an enemy only shortcuts to where its hull actually fits,
// instead of cutting a corner into a wall tip.
func (g *Game) clearPath(ax, ay, bx, by, radius float64) bool {
	if g.flood != nil {
		return !g.rockOnSegment(ax, ay, bx, by, radius)
	}
	rr := radius * radius
	for _, s := range g.segs {
		if segmentDistSq(ax, ay, bx, by, s.ax, s.ay, s.bx, s.by) <= rr {
			return false
		}
	}
	return true
}

// collides reports whether the ship circle at (x, y) overlaps any wall segment.
func (g *Game) collides(x, y float64) bool {
	return g.collidesAt(x, y, g.radius)
}

// collidesAt reports whether a circle of the given radius at (x, y) overlaps any
// wall segment, so enemies can use it with their own radius too.
func (g *Game) collidesAt(x, y, radius float64) bool {
	// SPIKE: the region field is the truth — a hull only fits where there is at least
	// `radius` of clearance from rock. This is what lets the ship enter a dug tunnel,
	// where no wall segment exists at all.
	if g.flood != nil {
		return g.flood.clearanceAt(x, y) < radius
	}
	r2 := radius * radius
	for _, s := range g.segs {
		if distPointSegmentSq(x, y, s.ax, s.ay, s.bx, s.by) <= r2 {
			return true
		}
	}
	return false
}

// distPointSegmentSq returns the squared distance from (px,py) to the segment
// (ax,ay)-(bx,by).
func distPointSegmentSq(px, py, ax, ay, bx, by float64) float64 {
	cx, cy := closestOnSegment(px, py, ax, ay, bx, by)
	return sq(px-cx) + sq(py-cy)
}

// closestOnSegment returns the point on segment (ax,ay)-(bx,by) nearest (px,py).
func closestOnSegment(px, py, ax, ay, bx, by float64) (float64, float64) {
	dx, dy := bx-ax, by-ay
	if dx == 0 && dy == 0 {
		return ax, ay
	}
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return ax + t*dx, ay + t*dy
}

func sq(v float64) float64 { return v * v }
