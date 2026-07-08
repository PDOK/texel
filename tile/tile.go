package tile

import (
	"github.com/go-spatial/geom"
	"github.com/pdok/texel/pointindex"
)

func translateToTileCoord(q pointindex.Quadrant, p geom.Polygon) geom.Polygon {
	translatedPol := make(geom.Polygon, len(p))
	basepoint := q.Centroid()

	for i, ring := range p.LinearRings() {
		translatedPol[i] = make([][2]float64, len(ring))
		for j, point := range ring {
			translatedPol[i][j] = [2]float64{point[0] - basepoint[0], point[1] - basepoint[1]}
		}
	}

	return p
}

func EncodePolygon(q pointindex.Quadrant, p geom.Polygon) ([]uint32, int32) {
	translatedPol := translateToTileCoord(q, p)
	encgeom, geomtype, err := EncodeGeometry(translatedPol)
	if err != nil {
		panic(err)
	}

	return encgeom, int32(geomtype)
}
