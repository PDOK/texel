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
	// EncodedGeoms is nil if encoding is not desired
	EncodedGeoms() []tile.EncodedGeometry
}

type SnapResult struct {
	Geometry geom.Geometry
	// Tiles is nil if encoding is not desired
	Tiles []pointindex.Quadrant
}

type TileCoord struct {
	X, Y uint
}

// Source and target for snapping
type Source interface {
	ReadFeatures(ch chan<- Feature)
}

type Target interface {
	WriteFeatures(ch <-chan FeatureForTileMatrix)
}

// Source and target for tile generation
type MVTSource interface {
	ListTiles() ([]TileCoord, error)
	GetFeaturesForTile(tileX, tileY uint) ([]tile.EncodedFeatureRow, error)
	GetAttributesForFeatures(featureIDs []int64) (tile.InternalAttributeTable, error)
	AttributeColumnNames() []string
}

type MVTTarget interface {
	WriteTile(x, y uint, data []byte) error
}
