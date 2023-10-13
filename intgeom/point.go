package intgeom

import (
	"github.com/go-spatial/geom"
)

// Point describes a simple 2D point
type Point [2]int64

func (p Point) ToGeomPoint() geom.Point {
	return geom.Point{
		ToGeomOrd(p[0]),
		ToGeomOrd(p[1]),
	}
}

func FromGeomPoint(p geom.Point) Point {
	return Point{
		FromGeomOrd(p[0]),
		FromGeomOrd(p[1]),
	}
}

// XY returns an array of 2D coordinates
func (p Point) XY() [2]int64 {
	return p
}

// SetXY sets a pair of coordinates
func (p Point) SetXY(xy [2]int64) {
	p[0] = xy[0]
	p[1] = xy[1]
}

// X is the x coordinate of a point in the projection
func (p Point) X() int64 { return p[0] }

// Y is the y coordinate of a point in the projection
func (p Point) Y() int64 { return p[1] }
