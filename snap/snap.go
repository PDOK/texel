package snap

import (
	"fmt"
	"math"
	"os"

	"github.com/go-spatial/geom"
	"github.com/pdok/sieve/processing"
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
	ix.toWkt(os.Stdout)
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
			} else {
				panic(fmt.Sprintf("no points found for %v", segment))
			}
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
