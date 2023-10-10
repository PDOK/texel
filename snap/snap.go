package snap

import (
	"math"

	"github.com/go-spatial/geom"
	"github.com/pdok/sieve/processing"
)

const (
	InternalPixelSize = 16
	DefaultTileSize   = 256
)

// TileMatrix contains the parameters to create a PointIndex and resembles a TileMatrix from OGC TMS
// TODO use proper and full TileMatrixSet support
type TileMatrix struct {
	MinX      float64 `yaml:"MinX"`
	MaxY      float64 `yaml:"MaxY"`
	PixelSize uint    `yaml:"PixelSize"` // defaults to 16
	TileSize  uint    `yaml:"TileSize"`  // defaults to 256
	Level     uint    `yaml:"Level"`     // determines the number of tiles
	CellSize  float64 `yaml:"CellSize"`
}

func snapPolygon(polygon *geom.Polygon, tileMatrix TileMatrix) *geom.Polygon {
	ix := pointIndexForGrid(tileMatrix)
	ix.InsertPolygon(polygon)
	newPolygon := addPointsAndSnap(ix, polygon)

	return newPolygon
}

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
			// TODO dedupe points
			if len(newVertices) > 0 {
				minus := 1
				if len(newVertices) == 1 {
					minus = 0
				}
				newRing = append(newRing, newVertices[:len(newVertices)-minus]...)
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

func pointIndexForGrid(tm TileMatrix) *PointIndex {
	// TODO support actual TMS or slippy grid
	pixelSize := tm.PixelSize
	if pixelSize == 0 {
		pixelSize = InternalPixelSize
	}
	tileSize := tm.TileSize
	if tileSize == 0 {
		tileSize = DefaultTileSize
	}
	gridSize := float64(pow2(tm.Level)) * float64(tileSize) * tm.CellSize
	maxDepth := int(float64(tm.Level) + math.Log2(float64(tileSize)) + math.Log2(float64(pixelSize)))
	ix := PointIndex{
		level:    0, // TODO maybe adjust for actual matrixId
		x:        0,
		y:        0,
		maxDepth: uint(maxDepth),
		extent: geom.Extent{
			tm.MinX, tm.MaxY - gridSize, tm.MinX + gridSize, tm.MaxY,
		},
	}
	return &ix
}

// SnapToPointCloud snaps polygons' points to a tile's internal pixel grid
// and adds points to lines to prevent intersections.
func SnapToPointCloud(source processing.Source, target processing.Target, tileMatrix TileMatrix) {
	processing.ProcessFeatures(source, target, func(p geom.Polygon) *geom.Polygon {
		return snapPolygon(&p, tileMatrix)
	})
}
