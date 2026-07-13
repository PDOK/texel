package processing

import (
	"github.com/go-spatial/geom"
	"github.com/pdok/texel/pointindex"
	"github.com/pdok/texel/tile"
)

type Feature interface {
	Columns() []any
	Geometry() geom.Geometry
}

type FeatureForTileMatrix interface {
	Feature
	TileMatrixID() int
	EncodedGeoms() []tile.EncodedGeometry
}

type SnapResult struct {
	Geometry geom.Geometry
	Tiles    []pointindex.Quadrant
}

type EncodedFeature struct {
	Feature      Feature
	EncodedGeoms []tile.EncodedGeometry
}

type Source interface {
	ReadFeatures(ch chan<- Feature)
}

type Target interface {
	WriteFeatures(ch <-chan FeatureForTileMatrix)
}
