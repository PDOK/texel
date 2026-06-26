package processing

import (
	"github.com/go-spatial/geom"
)

type Feature interface {
	Columns() []any
	Geometry() geom.Geometry
}

type FeatureForTileMatrix interface {
	Feature
	TileMatrixID() int
}

type Source interface {
	ReadFeatures(ch chan<- Feature)
}

type Target interface {
	WriteFeatures(ch <-chan Feature)
}
