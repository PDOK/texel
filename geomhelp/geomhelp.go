package geomhelp

import (
	"math"

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
//nolint:cyclop,nestif
func RayIntersect(pt, start, end [2]float64) (intersects, on bool) {
	if start[0] > end[0] {
		start, end = end, start
	}

	if pt[0] == start[0] {
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
	} else if pt[0] == end[0] {
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
	for i := range lines {
		s += wktMustEncodeTruncated(lines[i], maxLen)
	}
	for i := range points {
		s += wktMustEncodeTruncated(points[i], maxLen)
	}
	return s
}

func WktMustEncodeSlice(geoms []geom.Polygon, maxLen uint) string {
	s := ""
	for i := range geoms {
		s += WktMustEncode(geoms[i], maxLen) + "\n"
	}
	return s
}

func wktMustEncodeTruncated(geom geom.Geometry, width uint) string {
	if width == 0 {
		return wkt.MustEncode(geom)
	}
	return truncate.StringWithTail(wkt.MustEncode(geom), width, "...")
}
