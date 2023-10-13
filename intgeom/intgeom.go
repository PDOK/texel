// Package intgeom resembles github.com/go-spatial/geom but uses int64s internally
// to avoid floating point errors when performing arithmetic with the coords.
// The idea is that an int64's range (math.MaxInt64) is enough for (most) (earthly) geom operations.
// See https://www.explainxkcd.com/wiki/index.php/2170:_Coordinate_Precision.
// Using the last 9 digits as decimals should be enough for identifying
// the location of a grain of sand (in degrees).
// That leaves 10 digits for the whole units of measurement in your SRS.
// If that unit is degrees, it's more than enough (a circle only has 360).
// If that unit is feet, earth's circumference (131 482 560) also still fits.
// This is not intended to cover everything. go-spatial/geom has much more functionality.
// You are not required to use this.
package intgeom

const (
	debug     = false
	Precision = 1e09
)

// ToGeomOrd turns an ordinate represented as an integer back into a floating point
func ToGeomOrd(o int64) float64 {
	if o == 0 {
		return 0.0
	}
	return float64(o) / Precision
}

// FromGeomOrd turns a floating point ordinate into a representation by an integer
func FromGeomOrd(o float64) int64 {
	return int64(o * Precision)
}

// SegmentIntersect will find the intersection point (x,y) between two lines if
// there is one. Ok will be true if it found an intersection point and if the
// point is on both lines.
// ref: https://en.wikipedia.org/wiki/Line%E2%80%93line_intersection#Given_two_points_on_each_line
// TODO this should be in an "intplanar" package
func SegmentIntersect(l1, l2 Line) (pt [2]int64, ok bool) {
	x1, y1 := l1.Point1().X(), l1.Point1().Y()
	x2, y2 := l1.Point2().X(), l1.Point2().Y()
	x3, y3 := l2.Point1().X(), l2.Point1().Y()
	x4, y4 := l2.Point2().X(), l2.Point2().Y()

	deltaX12 := x1 - x2
	deltaX13 := x1 - x3
	deltaX34 := x3 - x4
	deltaY12 := y1 - y2
	deltaY13 := y1 - y3
	deltaY34 := y3 - y4
	denom := (deltaX12 * deltaY34) - (deltaY12 * deltaX34)

	// The lines are parallel or they overlap. No single point.
	if denom == 0 {
		return pt, false
	}

	xnom := (((x1 * y2) - (y1 * x2)) * deltaX34) - (deltaX12 * ((x3 * y4) - (y3 * x4)))
	ynom := (((x1 * y2) - (y1 * x2)) * deltaY34) - (deltaY12 * ((x3 * y4) - (y3 * x4)))
	bx := xnom / denom
	by := ynom / denom
	if bx == -0 {
		bx = 0
	}
	if by == -0 {
		by = 0
	}

	t := ((deltaX13 * deltaY34) - (deltaY13 * deltaX34)) / denom
	u := -((deltaX12 * deltaY13) - (deltaY12 * deltaX13)) / denom

	intersects := u >= 0.0 && u <= 1.0 && t >= 0.0 && t <= 1.0
	return [2]int64{bx, by}, intersects
}
