package intgeom

import (
	"log"

	"github.com/go-spatial/geom"
)

// Line has exactly two points
type Line [2][2]int64

func (l Line) ToGeomLine() geom.Line {
	return geom.Line{
		{ToGeomOrd(l[0][0]), ToGeomOrd(l[0][1])},
		{ToGeomOrd(l[1][0]), ToGeomOrd(l[1][1])},
	}
}

func FromGeomLine(l geom.Line) Line {
	return Line{
		{FromGeomOrd(l[0][0]), FromGeomOrd(l[0][1])},
		{FromGeomOrd(l[1][0]), FromGeomOrd(l[1][1])},
	}
}

// IsVertical returns true if the `y` elements of the points that make up the line (l) are equal.
func (l Line) IsVertical() bool { return l[0][0] == l[1][0] }

// IsHorizontal returns true if the `x` elements of the points that make the line (l) are equal.
func (l Line) IsHorizontal() bool { return l[0][1] == l[1][1] }

// Point1 returns a new copy of the first point in the line.
func (l Line) Point1() *Point { return (*Point)(&l[0]) }

// Point2 returns a new copy of the second point in the line.
func (l Line) Point2() *Point { return (*Point)(&l[1]) }

func (l Line) Vertices() [][2]int64 { return l[:] }

// ContainsPoint checks to see if the given pont lines on the line segment. (Including the end points.)
func (l Line) ContainsPoint(pt [2]int64) bool {
	minx, maxx := l[0][0], l[1][0]
	if minx > maxx {
		minx, maxx = maxx, minx
	}
	miny, maxy := l[0][1], l[1][1]
	if miny > maxy {
		miny, maxy = maxy, miny
	}
	if debug {
		log.Printf("pt.x %v is between %v and %v: %v && %v", pt[0], minx, maxx, minx <= pt[0], pt[0] <= maxx)
		log.Printf("pt.y %v is between %v and %v: %v && %v", pt[1], miny, maxy, miny <= pt[1], pt[1] <= maxy)
	}

	return minx <= pt[0] && pt[0] <= maxx && miny <= pt[1] && pt[1] <= maxy
}
