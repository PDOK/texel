package tile

import (
	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/mvt"
	"github.com/pdok/texel/pointindex"
)

const precision = 4096

func EncodePolygon(q pointindex.Quadrant, p geom.Polygon) ([]uint32, int32) {
	ext := q.Extent()
	preparedGeo := mvt.PrepareGeo(p, &ext, float64(precision))

	// This should not be necessary. 
//	sg, err := convert.ToTegola(preparedGeo)
//	tegolaGeo, err := validate.CleanGeometry(context.TODO(), sg, &ext)
//	validatedGeo := convert.ToGeom(tegolaGeo)

	encgeom, geomtype, err := EncodeGeometry(preparedGeo)
	if err != nil {
		panic(err)
	}

	return encgeom, int32(geomtype)
}
