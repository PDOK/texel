package intgeom

import (
	"github.com/go-spatial/geom"
)

type Polygon [][][2]M

func (p Polygon) ToGeomPolygon() geom.Polygon {
	geompol := make([][][2]float64, len(p))
	for i, ring := range p {
		geompol[i] = make([][2]float64, len(ring))
		for j, point := range ring {
			geompol[i][j] = [2]float64{ToGeomOrd(point[0]), ToGeomOrd(point[1])}
		}
	}
	return geompol
}

func FromGeomPolygon(p geom.Polygon) Polygon {
	intpol := make([][][2]int64, len(p))
	for i, ring := range p {
		intpol[i] = make([][2]int64, len(ring))
		for j, point := range ring {
			intpol[i][j] = [2]int64{FromGeomOrd(point[0]), FromGeomOrd(point[1])}
		}
	}
	return intpol
}
