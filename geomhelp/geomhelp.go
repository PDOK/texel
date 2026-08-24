package geomhelp

import (
	"fmt"
	"math"
	"strings"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/wkt"
	"github.com/muesli/reflow/truncate"
)

// https://en.wikipedia.org/wiki/Shoelace_formula
func Shoelace(pts [][2]float64) float64 {
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

// from paulmach/orb
// Original implementation: http://rosettacode.org/wiki/Ray-casting_algorithm#Go
//
//nolint:cyclop
func RayIntersect(pt, start, end [2]float64) (intersects, on bool) {
	if start[0] > end[0] {
		start, end = end, start
	}

	switch pt[0] {
	case start[0]:
		if pt[1] == start[1] {
			// pt == start
			return false, true
		} else if start[0] == end[0] {
			// vertical segment (start -> end)
			// return true if within the line, check to see if start or end is greater.
			if start[1] > end[1] && start[1] >= pt[1] && pt[1] >= end[1] {
				return false, true
			}

			if end[1] > start[1] && end[1] >= pt[1] && pt[1] >= start[1] {
				return false, true
			}
		}

		// Move the y coordinate to deal with degenerate case
		pt[0] = math.Nextafter(pt[0], math.Inf(1))
	case end[0]:
		if pt[1] == end[1] {
			// matching the end point
			return false, true
		}

		pt[0] = math.Nextafter(pt[0], math.Inf(1))
	}

	if pt[0] < start[0] || pt[0] > end[0] {
		return false, false
	}

	if start[1] > end[1] {
		if pt[1] > start[1] {
			return false, false
		} else if pt[1] < end[1] {
			return true, false
		}
	} else {
		if pt[1] > end[1] {
			return false, false
		} else if pt[1] < start[1] {
			return true, false
		}
	}

	rs := (pt[1] - start[1]) / (pt[0] - start[0])
	ds := (end[1] - start[1]) / (end[0] - start[0])

	if rs == ds {
		return false, true
	}

	return rs <= ds, false
}

func FloatPolygonToGeomPolygon(floater [][][2]float64) geom.Polygon {
	return floater
}

func FloatPolygonsToGeomPolygons(floaters [][][][2]float64) []geom.Polygon {
	geoms := make([]geom.Polygon, len(floaters))
	for i := range floaters {
		geoms[i] = floaters[i]
	}
	return geoms
}

func FloatPolygonsToGeomPolygonsForAllKeys[K comparable](floatersPerKey map[K][][][][2]float64) map[K][]geom.Polygon {
	geomsPerKey := make(map[K][]geom.Polygon, len(floatersPerKey))
	for k := range floatersPerKey {
		geomsPerKey[k] = FloatPolygonsToGeomPolygons(floatersPerKey[k])
	}
	return geomsPerKey
}

func WktMustEncode(g geom.Geometry, maxLen uint) (s string) {
	p, isPoly := g.(geom.Polygon)
	if !isPoly {
		return wktMustEncodeTruncated(g, maxLen)
	}

	var lines []geom.LineString
	var points []geom.Point
	pp := make(geom.Polygon, len(p))
	copy(pp, p)
	for r := 0; r < len(pp); r++ {
		switch len(pp[r]) {
		default:
			continue
		case 1:
			points = append(points, pp[r][0])
		case 2:
			lines = append(lines, pp[r])
		}
		pp = append(pp[:r], pp[r+1:]...)
		r--
	}

	if len(pp) > 0 {
		s = wktMustEncodeTruncated(pp, maxLen)
	}
	var builder strings.Builder
	for i := range lines {
		builder.WriteString(wktMustEncodeTruncated(lines[i], maxLen))
	}
	for i := range points {
		builder.WriteString(wktMustEncodeTruncated(points[i], maxLen))
	}
	s += builder.String()
	return s
}

func WktMustEncodeSlice(geoms []geom.Geometry, maxLen uint) string {
	s := ""
	var builder strings.Builder
	for i := range geoms {
		builder.WriteString(WktMustEncode(geoms[i], maxLen) + "\n")
	}
	s += builder.String()
	return s
}

func wktMustEncodeTruncated(geom geom.Geometry, width uint) string {
	if width == 0 {
		return wkt.MustEncode(geom)
	}
	return truncate.StringWithTail(wkt.MustEncode(geom), width, "...")
}

func GeometrySliceToGeom(geometries []geom.Geometry) geom.Geometry {
	if len(geometries) == 0 {
		panic("Multipolygon with zero polygons encountered")
	}
	if len(geometries) == 1 {
		return geometries[0]
	}
	return geometries
}

func PolygonSliceToGeom(polygons []geom.Polygon) geom.Geometry {
	if len(polygons) == 0 {
		panic("Multipolygon with zero polygons encountered")
	}
	if len(polygons) == 1 {
		return polygons[0]
	}
	multipolygon := make(geom.MultiPolygon, len(polygons))
	for i, p := range polygons {
		multipolygon[i] = p
	}
	return multipolygon
}

// Convert geometry to slice of points. Does not accept multi-geometries
func PointSlice(g geom.Geometry) []geom.Point {
	switch g := g.(type) {
	case geom.Point:
		return []geom.Point{g}
	case geom.Polygon:
		rings := g.LinearRings()
		pointsCount := 0
		for _, ring := range rings {
			pointsCount += len(ring)
		}
		points := make([]geom.Point, 0, pointsCount)
		for _, ring := range rings {
			for _, point := range ring {
				points = append(points, point)
			}
		}
		return points
	case geom.LineString:
		vertices := g.Vertices()
		points := make([]geom.Point, 0, len(vertices))
		for _, vert := range vertices {
			points = append(points, vert)
		}
		return points
	default:
		err := fmt.Errorf("PointSlice called incompatible geometry type: %s", g)
		panic(err)
	}
}

// Essentially merge two lists
func mergeFamily[S any, E any, M ~[]E](
	g, h geom.Geometry, wrap func(S) M, errMsg string,
) geom.Geometry {
	switch g := g.(type) {
	case S:
		switch h := h.(type) {
		case S:
			return append(wrap(g), wrap(h)...)
		case M:
			return append(wrap(g), h...)
		default:
			panic(errMsg)
		}
	case M:
		switch h := h.(type) {
		case S:
			return append(g, wrap(h)...)
		case M:
			return append(g, h...)
		default:
			panic(errMsg)
		}
	default:
		panic(errMsg)
	}
}

func MergeGeometries(g, h geom.Geometry) geom.Geometry {
	if g == nil {
		return h
	}
	if h == nil {
		return g
	}

	switch g.(type) {
	case geom.Point, geom.MultiPoint:
		return mergeFamily(g, h, func(p geom.Point) geom.MultiPoint { return geom.MultiPoint{p} },
			"Trying to merge point with non-point geometry")
	case geom.LineString, geom.MultiLineString:
		return mergeFamily(g, h, func(l geom.LineString) geom.MultiLineString { return geom.MultiLineString{l} },
			"Trying to merge linestring with non-linestring geometry")
	case geom.Polygon, geom.MultiPolygon:
		return mergeFamily(g, h, func(p geom.Polygon) geom.MultiPolygon { return geom.MultiPolygon{p} },
			"Trying to merge a polygon with a non-polygon geometry.")
	default:
		panic("Trying to merge unknown geometry types.")
	}
}

// A rather unfortunate function that needs to exist to deal with golang interfaces.
func MultiGeometryToSlice(multiGeom geom.Geometry) []geom.Geometry {
	switch geometry := multiGeom.(type) {
	case geom.MultiPolygon:
		geoms := make([]geom.Geometry, 0, len(geometry))
		for _, polygon := range geometry {
			geoms = append(geoms, geom.Polygon(polygon))
		}
		return geoms
	case geom.MultiLineString:
		geoms := make([]geom.Geometry, 0, len(geometry))
		for _, line := range geometry {
			geoms = append(geoms, geom.LineString(line))
		}
		return geoms
	case geom.MultiPoint:
		geoms := make([]geom.Geometry, 0, len(geometry))
		for _, point := range geometry {
			geoms = append(geoms, geom.Point(point))
		}
		return geoms
	default:
		panic("Cannot convert to slice: not a multigeometry.")
	}
}
