package processing

import (
	"github.com/go-spatial/geom"
	"github.com/pdok/texel/pointindex"
)

type Feature interface {
	Columns() []any
	Geometry() geom.Geometry
}

type FeatureForTileMatrix interface {
	Feature
	TileMatrixID() int
}

type SnapResult struct {
	Geometry geom.Geometry
	Tiles []pointindex.Quadrant
}

type Source interface {
	ReadFeatures(ch chan<- Feature)
}

type Target interface {
	WriteFeatures(ch <-chan Feature)
}
