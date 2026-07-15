package asset

// flattenMaxDepth caps the recursive subdivision so a degenerate curve can never
// loop forever.
const flattenMaxDepth = 12

// hasCurves reports whether the path contains any Bézier command.
func (p Path) hasCurves() bool {
	for i := range p.Commands {
		switch p.Commands[i].Op {
		case OpQuadTo, OpCubicTo:
			return true
		}
	}
	return false
}

// Flatten returns the path with every Bézier curve (Q/C) subdivided into line
// segments within flattenTol of the true curve, so consumers that only
// understand M/L/Z — the renderers, collision, bounds, the flood/fog geometry —
// need no curve handling. A path with no curves is returned unchanged (no
// allocation), so non-curved assets pay nothing.
func (p Path) Flatten() Path {
	if !p.hasCurves() {
		return p
	}
	out := make([]Command, 0, len(p.Commands)*4)
	var pen Point
	for _, c := range p.Commands {
		switch c.Op {
		case OpQuadTo:
			if len(c.Ctrl) >= 1 {
				end := Point{X: c.X, Y: c.Y}
				flattenQuad(pen, c.Ctrl[0], end, flattenTol, 0, &out)
				pen = end
			}
		case OpCubicTo:
			if len(c.Ctrl) >= 2 {
				end := Point{X: c.X, Y: c.Y}
				flattenCubic(pen, c.Ctrl[0], c.Ctrl[1], end, flattenTol, 0, &out)
				pen = end
			}
		default: // M, L, Z pass through unchanged
			out = append(out, Command{Op: c.Op, X: c.X, Y: c.Y})
			if c.Op != OpClose {
				pen = Point{X: c.X, Y: c.Y}
			}
		}
	}
	return Path{Commands: out}
}

// flattenQuad subdivides a quadratic Bézier (p0 control p1) into line commands.
func flattenQuad(p0, c, p1 Point, tol float64, depth int, out *[]Command) {
	if depth >= flattenMaxDepth || distPointSegSq(c, p0, p1) <= tol*tol {
		*out = append(*out, Command{Op: OpLineTo, X: p1.X, Y: p1.Y})
		return
	}
	a := mid(p0, c)
	b := mid(c, p1)
	m := mid(a, b)
	flattenQuad(p0, a, m, tol, depth+1, out)
	flattenQuad(m, b, p1, tol, depth+1, out)
}

// flattenCubic subdivides a cubic Bézier (p0 c1 c2 p1) into line commands.
func flattenCubic(p0, c1, c2, p1 Point, tol float64, depth int, out *[]Command) {
	flat := distPointSegSq(c1, p0, p1) <= tol*tol && distPointSegSq(c2, p0, p1) <= tol*tol
	if depth >= flattenMaxDepth || flat {
		*out = append(*out, Command{Op: OpLineTo, X: p1.X, Y: p1.Y})
		return
	}
	p01 := mid(p0, c1)
	p12 := mid(c1, c2)
	p23 := mid(c2, p1)
	p012 := mid(p01, p12)
	p123 := mid(p12, p23)
	m := mid(p012, p123)
	flattenCubic(p0, p01, p012, m, tol, depth+1, out)
	flattenCubic(m, p123, p23, p1, tol, depth+1, out)
}

func mid(a, b Point) Point {
	return Point{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
}

// distPointSegSq is the squared distance from p to the line through a and b
// (the de Casteljau flatness test: a control point near the chord is flat enough).
func distPointSegSq(p, a, b Point) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		ex := p.X - a.X
		ey := p.Y - a.Y
		return ex*ex + ey*ey
	}
	cross := (p.X-a.X)*dy - (p.Y-a.Y)*dx
	return cross * cross / l2
}
