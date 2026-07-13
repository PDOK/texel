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
	Tiles    []pointindex.Quadrant
}

type EncodedGeometry struct {
	Encoding     []uint32
	GeometryType int32
	XTile        uint
	YTile        uint
}

type EncodedFeature struct {
	Feature      Feature
	EncodedGeoms []EncodedGeometry
}

type Source interface {
	ReadFeatures(ch chan<- Feature)
}

type Target interface {
	WriteFeatures(ch <-chan Feature)
}
