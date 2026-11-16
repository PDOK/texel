// Package intgeom resembles github.com/go-spatial/geom but uses int64s internally
// to avoid floating point errors when performing arithmetic with the coords.
//
// The idea is that an int64's range (math.MaxInt64) is enough for (most) (earthly) geom operations.
// See https://www.explainxkcd.com/wiki/index.php/2170:_Coordinate_Precision.
// Using the last 10 digits as decimals should be enough for identifying
// the location of a grain of sand (in degrees).
// More importantly, 10 digits are necessary to minimize the rounding error
// when the span of a matrix is divided by its size (in pixels) on deeper levels.
//
// That leaves 9 digits for the whole units of measurement in your SRS.
// If that unit is degrees, it's more than enough (a circle only has 360).
// If that unit is feet, earth's circumference (131 482 560) also still fits.
// This is not intended to cover everything. go-spatial/geom has much more functionality.
// You are not required to use this.
package intgeom

import (
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"strings"

	"github.com/go-spatial/geom/planar"
)

const (
	debug     = false
	Precision = 10
	Half      = 5000000000
	One       = 10000000000
)

// M is short for measure.
// Used to indicate that a distance or ordinate is saved as an int64 and needs division by Precision (eventually).
// Shortened for readability in long lines (in other packages it will also be prefixed with intgeom.)
type M = int64

// ToGeomOrd turns an ordinate represented as an integer back into a floating point
func ToGeomOrd(o M) float64 {
	if o == 0 {
		return 0.0
	}
	return float64(o) / math.Pow(10, Precision)
}

// FromGeomOrd turns a floating point ordinate into a representation by an integer
func FromGeomOrd(o float64) M {
	return int64(o * math.Pow(10, Precision))
}

// SegmentIntersect will find the intersection point (x,y) between two lines if
// there is one. Ok will be true if it found an intersection point and if the
// point is on both lines.
// ref: https://en.wikipedia.org/wiki/Line%E2%80%93line_intersection#Given_two_points_on_each_line
// TODO implement this with integers
func SegmentIntersect(l1, l2 Line) ([2]int64, bool) {
	intersection, intersects := planar.SegmentIntersect(l1.ToGeomLine(), l2.ToGeomLine())
	intIntersection := [2]int64{FromGeomOrd(intersection[0]), FromGeomOrd(intersection[1])}
	return intIntersection, intersects
}

func PrintWithDecimals(o M, n uint) string {
	s := fmt.Sprintf("%0"+strconv.Itoa(Precision+1)+"d", o)
	l := len(s)
	m := s[l-Precision : l]
	if n < Precision {
		m = m[0:n]
	} else {
		//nolint:gosec // G115
		m += strings.Repeat("0", int(n-Precision))
	}
	c := s[0 : l-Precision]
	return c + "." + m
}

////////////////////////////
// Liang-Barsky intersect //
////////////////////////////


// We use a variant of the Liang-Barsky algorithm (https://en.wikipedia.org/wiki/Liang%E2%80%93Barsky_algorithm)
// to determine whether a line intersects an extent. The idea is to abuse the
// fact that an extent is a square, which allows us to solve a problem in the 
// x- and y- coordinate frame separately.
//
// To be specific: Suppose we have a line (P0, P1). Parameterize this line as
// P0 + t * (P1 - P0) . Here t goes from 0 to 1 (which lets the point g from P0
// to P1). Then it is not very hard to compute the values of t for which the x-
// coordinate of P0 + t * (P1 - P0) lies between the MinX and MaxX of the
// extent. Similarly for the y-coordinate. The line intersects the extent if
// and only if there is a value of t for which this happens simultaneously.
//
// In fact, we can say slightly more. The values of t for which the x-
// coordinate of the line lies between MinX and MaxX is an interval: those
// values of t lie between a minimal and maximal value. You can even choose to
// in- or exclude the endpoints based on whether the corresponding MinX or MaxX
// is in- or excluded.
//
// This requires some infrastructure. The Interval type models the minimal and
// maximal values of t alluded to above. The "intersect" function models the
// idea that conditions have to be true at the same time. Aside from the x- and
// y-coordinate conditions, we have a condition that t needs to lie between 0
// and 1, i.e., the point needs to lie on the interior of the line. We also 
// have a Rational type, which is needed since we divide integers and require
// exactness. Here, 128-bit arithmetic is involved: our coordinates are large
// enough that multiplying them does not fit in an int64.
//
// The reason this algorithm is relatively efficient in terms of speed and 
// intersection logic is that we do not compute the intersection point, we just
// check if one exists.

// Struct to represent a rational.
// The denominator must be positive, the numerator can be negative.
type Rational struct {
	numerator, denominator int64
}

func abs64(n int64) uint64 {
	if n < 0 {
		// Unintuitive formula handles edge case n = MinInt64
		return uint64(-(n + 1)) + 1 //nolint: GoSec //G115
	}
	return uint64(n)
}

// Compare with usual convention:
// cmp < 0 means a < b
// cmp = 0 means a = b
// cmp > 0 means a > b
// Use 128 bits; our numbers are so large that this is necessary.
// Works with negative rationals (negative numerator)
func cmp(a, b Rational) int64 {
	if a.numerator < 0 && b.numerator >= 0 {
		return -1
	}
	if a.numerator >= 0 && b.numerator < 0 {
		return 1
	}

	ahi, alo := bits.Mul64(abs64(a.numerator), uint64(b.denominator)) //nolint: GoSec // G115
	bhi, blo := bits.Mul64(abs64(b.numerator), uint64(a.denominator)) //nolint: GoSec // G115

	var result int64
	switch {
	case ahi < bhi:
		result = -1
	case ahi > bhi:
		result = 1
	case alo < blo:
		result = -1
	case alo > blo:
		result = 1
	default:
		result = 0
	}

	if a.numerator < 0 {
		return -result
	}
	return result
}

type Interval struct {
	bottom, top             Rational
	bottomStrict, topStrict bool
}

var unitInterval = Interval{
	bottom:       Rational{0, 1},
	top:          Rational{1, 1},
	bottomStrict: false,
	topStrict:    false,
}

var emptyInterval = Interval{
	bottom:       Rational{1, 1},
	top:          Rational{0, 1},
	bottomStrict: true,
	topStrict:    true,
}

func (i *Interval) isEmpty() bool {
	c := cmp(i.bottom, i.top)
	return c > 0 || (c == 0 && (i.bottomStrict || i.topStrict))
}

func (i *Interval) intersect(j Interval) {
	bottomCmp := cmp(i.bottom, j.bottom)
	switch {
	case bottomCmp < 0:
		i.bottom = j.bottom
		i.bottomStrict = j.bottomStrict
	case bottomCmp == 0:
		i.bottomStrict = i.bottomStrict || j.bottomStrict
	case bottomCmp > 0:
	}

	topCmp := cmp(i.top, j.top)
	switch {
	case topCmp < 0:
	case topCmp == 0:
		i.topStrict = i.topStrict || j.topStrict
	case topCmp > 0:
		i.top = j.top
		i.topStrict = j.topStrict
	}
}

// This is the delta = 0 case of projectInterval
// In the context of Liang-Barsky it corresponds to a vertical or horizontal line
func constInterval(p, low, high int64, lowStrict, highStrict bool) Interval {
	switch {
	case p < low:
		return emptyInterval
	case p == low && lowStrict:
		return emptyInterval
	case p > high:
		return emptyInterval
	case p == high && highStrict:
		return emptyInterval
	default:
		return unitInterval
	}
}

// This creates the interval of t for which low <= p + t*delta <= high
// The inequalities can also be strict
func projectInterval(p, delta, low, high int64, lowStrict, highStrict bool) Interval {
	if delta == 0 {
		return constInterval(p, low, high, lowStrict, highStrict)
	}

	p2, delta2, low2, high2 := p, delta, low, high
	lowStrict2, highStrict2 := lowStrict, highStrict

	// If delta is negative, flip all signs. This is due to the equivalence of the equations:
	//  L <  p + t* delta <  H
	// -L > -p + t*-delta > -H
	// This interchanges high and low, which also affects the strictness
	// So after this we can assume we are in the normal situation where delta is positive
	if delta < 0 {
		p2, delta2, low2, high2 = -p, -delta, -high, -low
		lowStrict2 = highStrict
		highStrict2 = lowStrict
	}

	return Interval{
		bottom:       Rational{low2 - p2, delta2},
		bottomStrict: lowStrict2,
		top:          Rational{high2 - p2, delta2},
		topStrict:    highStrict2,
	}

}

func (e Extent) IntersectsLine(l Line) bool {
	interval := unitInterval
	interval.intersect(
		projectInterval(
			l.Point1().X(), l.Point2().X()-l.Point1().X(),
			e.MinX(), e.MaxX(),
			false, true)) // This reflects that the right edge does not belong to the extent

	interval.intersect(
		projectInterval(
			l.Point1().Y(), l.Point2().Y()-l.Point1().Y(),
			e.MinY(), e.MaxY(),
			false, true)) // This reflects that the top edge does not belong to the extent
	return !interval.isEmpty()
}
