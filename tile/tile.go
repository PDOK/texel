package tile

import (
	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/mvt"
	"github.com/pdok/texel/pointindex"
	"github.com/pdok/texel/processing"
)

const precision = 4096

func MvtEncodeGeometry(q pointindex.Quadrant, g geom.Geometry) processing.EncodedGeometry {
	ext := q.Extent()
	preparedGeo := mvt.PrepareGeo(g, &ext, float64(precision))

	// This should not be necessary. 
//	sg, err := convert.ToTegola(preparedGeo)
//	tegolaGeo, err := validate.CleanGeometry(context.TODO(), sg, &ext)
//	validatedGeo := convert.ToGeom(tegolaGeo)

	encgeom, geomtype, err := EncodeGeometry(preparedGeo)
	if err != nil {
		panic(err)
	}
	

	xTile, yTile := q.Coords()

	return processing.EncodedGeometry{
		Encoding: encgeom,
		GeometryType: int32(geomtype),
		XTile: xTile,
		YTile: yTile,
	}
}
