package snap

import (
	"math"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/slippy"
	"github.com/pdok/sieve/processing"
)

func snapPolygon(polygon *geom.Polygon) *geom.Polygon {
	pi := pointIndexForGrid(nil, 0)
	pi.InsertPolygon(polygon)
	newPolygon := addPointsAndSnap(pi, polygon)

	return newPolygon
}

func addPointsAndSnap(pi *PointIndex, polygon *geom.Polygon) *geom.Polygon {
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
			newVertices := pi.SnapClosestPoints(segment)
			// TODO dedupe points
			if len(newVertices) > 0 {
				newRing = append(newRing, newVertices[:len(newVertices)-1]...)
			} // FIXME what if it is not? shouldn't happen? always line first and last are returned?
		}
		if len(newRing) > 2 {
			newPolygon = append(newPolygon, newRing)
		} else {
			// TODO keep as line or point instead of removing cq sieving
			if ringI == 0 {
				return nil // outer ring has become too small
			}
		}
	}
	return (*geom.Polygon)(&newPolygon)
}

func pointIndexForGrid(grid *slippy.Grid, matrixId uint) *PointIndex {
	// TODO replace hardcoded NetherlandsRDNewQuad with TMS or slippy grid
	minX := -285401.92
	maxY := 903401.92
	pixelSize := 16
	tileSize := 256
	matrixSize := 32   // <-- = 5 //  0 = 1
	cellSize := 107.52 // <-- = 5 //  0 = 3440.64
	gridSize := float64(matrixSize) * float64(tileSize) * cellSize
	maxDepth := int(math.Log2(float64(matrixSize)) + math.Log2(float64(tileSize)) + math.Log2(float64(pixelSize)))
	pi := PointIndex{
		level:    0, // TODO maybe adjust for actual matrixId
		x:        0,
		y:        0,
		maxDepth: uint(maxDepth),
		extent: geom.Extent{
			minX, maxY - gridSize, minX + gridSize, maxY,
		},
	}
	return &pi
}

// SnapToPointCloud snaps polygons' points to a tile's internal pixel grid
// and adds points to lines to prevent intersections.
func SnapToPointCloud(source processing.Source, target processing.Target) {
	processing.ProcessFeatures(source, target, func(p geom.Polygon) *geom.Polygon {
		return snapPolygon(&p)
	})
}
