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

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/go-spatial/geom/planar"
)

const (
	debug     = false
	Precision = 9
)

type Ord = int64

// ToGeomOrd turns an ordinate represented as an integer back into a floating point
func ToGeomOrd(o Ord) float64 {
	if o == 0 {
		return 0.0
	}
	return float64(o) / math.Pow(10, Precision)
}

// FromGeomOrd turns a floating point ordinate into a representation by an integer
func FromGeomOrd(o float64) Ord {
	return int64(o * math.Pow(10, Precision))
}

// SegmentIntersect will find the intersection point (x,y) between two lines if
// there is one. Ok will be true if it found an intersection point and if the
// point is on both lines.
// ref: https://en.wikipedia.org/wiki/Line%E2%80%93line_intersection#Given_two_points_on_each_line
// TODO implement this with integers
func SegmentIntersect(l1, l2 Line) ([2]int64, bool) {
	intersection, intersects := planar.SegmentIntersect(l1.ToGeomLine(), l2.ToGeomLine())
	intIntersection := [2]int64{FromGeomOrd(intersection[0]), FromGeomOrd(intersection[0])}
	return intIntersection, intersects
}

func PrintWithDecimals(o Ord, n uint) string {
	s := fmt.Sprintf("%0"+strconv.Itoa(Precision+1)+"d", o)
	l := len(s)
	m := s[l-Precision : l]
	if n < Precision {
		m = m[0:n]
	} else {
		m = m + strings.Repeat("0", int(n-Precision))
	}
	c := s[0 : l-Precision]
	return c + "." + m
}
