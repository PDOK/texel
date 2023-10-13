package intgeom

import (
	"github.com/go-spatial/geom"
)

// Extenter represents an interface that returns a boundbox.
type Extenter interface {
	Extent() (extent [4]int64)
}

// MinMaxer is a wrapper for an Extent that gets min/max of the extent
type MinMaxer interface {
	MinX() int64
	MinY() int64
	MaxX() int64
	MaxY() int64
}

// Extent represents the minx, miny, maxx and maxy
// A nil extent represents the whole universe.
type Extent [4]int64

func (e Extent) ToGeomExtent() geom.Extent {
	return geom.Extent{
		ToGeomOrd(e[0]),
		ToGeomOrd(e[1]),
		ToGeomOrd(e[2]),
		ToGeomOrd(e[3]),
	}
}

func FromGeomExtent(e geom.Extent) Extent {
	return Extent{
		FromGeomOrd(e[0]),
		FromGeomOrd(e[1]),
		FromGeomOrd(e[2]),
		FromGeomOrd(e[3]),
	}
}

/* ========================= ATTRIBUTES ========================= */

// Vertices return the vertices of the Bounding Box. The vertices are ordered in the following manner.
// (minx,miny), (maxx,miny), (maxx,maxy), (minx,maxy)
func (e Extent) Vertices() [][2]int64 {
	return [][2]int64{
		{e.MinX(), e.MinY()},
		{e.MaxX(), e.MinY()},
		{e.MaxX(), e.MaxY()},
		{e.MinX(), e.MaxY()},
	}
}

// ClockwiseFunc returns weather the set of points should be considered clockwise or counterclockwise.
// The last point is not the same as the first point, and the function should connect these points as needed.
type ClockwiseFunc func(...[2]int64) bool

// Edges returns the clockwise order of the edges that make up the extent.
func (e Extent) Edges(cwfn ClockwiseFunc) [][2][2]int64 {
	v := e.Vertices()
	if cwfn != nil && !cwfn(v...) {
		v[0], v[1], v[2], v[3] = v[3], v[2], v[1], v[0]
	}
	return [][2][2]int64{
		{v[0], v[1]},
		{v[1], v[2]},
		{v[2], v[3]},
		{v[3], v[0]},
	}
}

// MaxX is the larger of the x values.
func (e Extent) MaxX() int64 {
	return e[2]
}

// MinX  is the smaller of the x values.
func (e Extent) MinX() int64 {
	return e[0]
}

// MaxY is the larger of the y values.
func (e Extent) MaxY() int64 {
	return e[3]
}

// MinY is the smaller of the y values.
func (e Extent) MinY() int64 {
	return e[1]
}

// XSpan is the distance of the Extent in X
func (e Extent) XSpan() int64 {
	return e[2] - e[0]
}

// YSpan is the distance of the Extent in Y
func (e Extent) YSpan() int64 {
	return e[3] - e[1]
}

// Extent returns back the min and max of the Extent
func (e Extent) Extent() [4]int64 {
	return [4]int64{e.MinX(), e.MinY(), e.MaxX(), e.MaxY()}
}
