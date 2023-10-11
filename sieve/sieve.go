package sieve

import (
	"github.com/go-spatial/geom"
	"github.com/pdok/sieve/processing"
	"math"
)

// calculate the area of a polygon
func area(geom [][][2]float64) float64 {
	interior := .0
	if geom == nil {
		return 0.
	}
	if len(geom) > 1 {
		for _, i := range geom[1:] {
			interior += shoelace(i)
		}
	}
	return shoelace(geom[0]) - interior
}

// https://en.wikipedia.org/wiki/Shoelace_formula
func shoelace(pts [][2]float64) float64 {
	sum := 0.
	if len(pts) == 0 {
		return 0.
	}

	p0 := pts[len(pts)-1]
	for _, p1 := range pts {
		sum += p0[1]*p1[0] - p0[0]*p1[1]
		p0 = p1
	}
	return math.Abs(sum / 2)
}

// polygonSieve will sieve a given POLYGON
func polygonSieve(p geom.Polygon, resolution float64) geom.Polygon {
	minArea := resolution * resolution
	if area(p) > minArea {
		if len(p) > 1 {
			var sievedPolygon geom.Polygon
			sievedPolygon = append(sievedPolygon, p[0])
			for _, interior := range p[1:] {
				if shoelace(interior) > minArea {
					sievedPolygon = append(sievedPolygon, interior)
				}
			}
			return sievedPolygon
		}
		return p
	}
	return nil
}

func Sieve(source processing.Source, target processing.Target, resolution float64) {
	processing.ProcessFeatures(source, target, func(p geom.Polygon) geom.Polygon {
		return polygonSieve(p, resolution)
	})
}
