package snap

import (
	"fmt"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/processing"
)

func snapPolygon(polygon *geom.Polygon, tileMatrix TileMatrix) *geom.Polygon {
	ix := NewPointIndexFromTileMatrix(tileMatrix)
	ix.InsertPolygon(polygon)
	newPolygon := addPointsAndSnap(ix, polygon)

	return newPolygon
}

//nolint:cyclop
func addPointsAndSnap(ix *PointIndex, polygon *geom.Polygon) *geom.Polygon {
	newPolygon := make([][][2]float64, 0, len(*polygon))
	// Could use polygon.AsSegments(), but it skips rings with <3 segments and starts with the last segment.
	for ringI, ring := range polygon.LinearRings() {
		ringLen := len(ring)
		newRing := make([][2]float64, 0, ringLen*2) // TODO better estimation of new amount of points
		for vertexI, vertex := range ring {
			// LinearRings(): "The last point in the linear ring will not match the first point."
			// So also including that one.
			nextVertexI := (vertexI + 1) % ringLen
			segment := geom.Line{vertex, ring[nextVertexI]}
			newVertices := ix.SnapClosestPoints(segment)
			newVerticesCount := len(newVertices)
			if newVerticesCount > 0 {
				// 0 if len is 1, 1 otherwise
				minus := min(newVerticesCount-1, 1)
				// remove last vertex if there is more than 1 vertex, as the first vertex in the next segment will be the same
				newVertices = newVertices[:newVerticesCount-minus]
				// remove first element if it is equal to the last element added to newRing
				if len(newRing) > 0 && newVertices[0] == newRing[len(newRing)-1] {
					newVertices = newVertices[1:]
				}
				newRing = append(newRing, newVertices...)
			} else {
				panic(fmt.Sprintf("no points found for %v", segment))
			}
		}
		// LinearRings(): "The last point in the linear ring will not match the first point."
		if len(newRing) > 1 && newRing[0] == newRing[len(newRing)-1] {
			newRing = newRing[:len(newRing)-1]
		}
		switch len(newRing) {
		case 0:
			if ringI == 0 {
				// outer ring has become too small
				return nil
			}
		case 1, 2:
			if ringI == 0 {
				// keep outer ring as point or line
				newPolygon = append(newPolygon, newRing)
			}
		default:
			// deduplicate points in the ring, then add to the polygon
			newPolygon = append(newPolygon, deduplicateRing(newRing))
		}
	}
	return (*geom.Polygon)(&newPolygon)
}

//nolint:cyclop,nestif
func deduplicateRing(newRing [][2]float64) [][2]float64 {
	dedupedRing := make([][2]float64, 0, len(newRing))
	skip := 0
	skipped := false
	for newVertexI, newVertex := range newRing {
		if skip > 0 {
			skip--
			skipped = true
		} else {
			// if vertex at i is equal to vertex at i+2, and vertex at i+1 is equal to vertex at i+3,
			// then we're just traversing the same line three times --> skip two points to turn it into a single line
			newVertexIplus1 := (newVertexI + 1) % len(newRing)
			newVertexIplus2 := (newVertexI + 2) % len(newRing)
			newVertexIplus3 := (newVertexI + 3) % len(newRing)
			if newVertexI != newVertexIplus2 && newVertexIplus1 != newVertexIplus3 &&
				newVertex == newRing[newVertexIplus2] && newRing[newVertexIplus1] == newRing[newVertexIplus3] {
				skip = 2
			}
			// if we just skipped points, there may still be a duplicate traversal from the previous point, check that in the same manner
			if skipped {
				newVertexIminus1 := (newVertexI - 1) % len(newRing)
				if newVertexIminus1 != newVertexIplus1 && newVertexI != newVertexIplus2 &&
					newRing[newVertexIminus1] == newRing[newVertexIplus1] && newVertex == newRing[newVertexIplus2] {
					skip = 2
				} else {
					skipped = false
				}
			}
			dedupedRing = append(dedupedRing, newRing[newVertexI])
		}
	}
	if len(newRing) != len(dedupedRing) && len(dedupedRing) > 0 {
		return dedupedRing
	}
	return newRing
}

// SnapToPointCloud snaps polygons' points to a tile's internal pixel grid
// and adds points to lines to prevent intersections.
func SnapToPointCloud(source processing.Source, target processing.Target, tileMatrix TileMatrix) {
	processing.ProcessFeatures(source, target, func(p geom.Polygon) *geom.Polygon {
		return snapPolygon(&p, tileMatrix)
	})
}
