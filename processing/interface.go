package processing

import (
	"github.com/go-spatial/geom"
)

type Feature interface {
	Columns() []interface{}
	Geometry() geom.Geometry
}

type FeatureForTileMatrix interface {
	Feature
	TileMatrixID() int
}

type Source interface {
	ReadFeatures(chan<- Feature)
}

type Target interface {
	WriteFeatures(<-chan Feature)
}
