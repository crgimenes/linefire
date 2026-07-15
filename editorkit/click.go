package editorkit

// DoubleClickGap is the maximum number of ticks between two clicks for them to
// count as a double-click (about a third of a second at 60 TPS).
const DoubleClickGap int64 = 20

// doubleClickDistSq is the squared screen-pixel distance the two clicks must
// stay within.
const doubleClickDistSq = 36.0

// DoubleClick reports whether a click at the current tick closely follows the
// previous one in both time and screen position. dx/dy are the screen-space
// offsets from the previous click.
func DoubleClick(cur, last int64, dx, dy float64) bool {
	dt := cur - last
	return dt > 0 && dt <= DoubleClickGap && dx*dx+dy*dy <= doubleClickDistSq
}
