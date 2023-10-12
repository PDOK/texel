package snap

import (
	"fmt"
	"math"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/processing"
)

const (
	precision = 5
)

// TileMatrix contains the parameters to create a PointIndex and resembles a TileMatrix from OGC TMS
// TODO use proper and full TileMatrixSet support
type TileMatrix struct {
	MinX      float64 `yaml:"MinX"`
	MaxY      float64 `yaml:"MaxY"`
	PixelSize uint    `default:"16" yaml:"PixelSize"`
	TileSize  uint    `default:"256" yaml:"TileSize"`
	Level     uint    `yaml:"Level"`    // a.k.a. ID. determines the number of tiles
	CellSize  float64 `yaml:"CellSize"` // the cell size at that Level
}

func (tm *TileMatrix) GridSize() float64 {
	return float64(pow2(tm.Level)) * float64(tm.TileSize) * tm.CellSize
}

func (tm *TileMatrix) MinY() float64 {
	return roundFloat(tm.MaxY-tm.GridSize(), precision) // FIXME is this the solution to all fp issues?
}

func (tm *TileMatrix) MaxX() float64 {
	return roundFloat(tm.MinX+tm.GridSize(), precision)
}

func snapPolygon(polygon *geom.Polygon, tileMatrix TileMatrix) *geom.Polygon {
	ix := NewPointIndexFromTileMatrix(tileMatrix)
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
			newVerticesCount := len(newVertices)
			if newVerticesCount > 0 {
				// 0 if len is 1, 1 otherwise
				minus := min(newVerticesCount-1, 1)
				// remove last vertex if there is more than 1 vertex, as the first vertex in the next segment will be the same
				newRing = append(newRing, newVertices[:newVerticesCount-minus]...)
			} else {
				panic(fmt.Sprintf("no points found for %v", segment))
			}
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

func deduplicateRing(newRing [][2]float64) [][2]float64 {
	dedupedRing := make([][2]float64, 0, len(newRing))
	skip := 0
	for newVertexI, newVertex := range newRing {
		if skip > 0 {
			skip--
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
			dedupedRing = append(dedupedRing, newVertex)
		}
	}
	return dedupedRing
}

// SnapToPointCloud snaps polygons' points to a tile's internal pixel grid
// and adds points to lines to prevent intersections.
func SnapToPointCloud(source processing.Source, target processing.Target, tileMatrix TileMatrix) {
	processing.ProcessFeatures(source, target, func(p geom.Polygon) *geom.Polygon {
		return snapPolygon(&p, tileMatrix)
	})
}

func roundFloat(val float64, precision uint) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
