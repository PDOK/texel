package processing

import (
	"github.com/go-spatial/geom"
	"github.com/pdok/texel/intgeom"
	"github.com/pdok/texel/pointindex"
	"github.com/pdok/texel/tile"
	"github.com/pdok/texel/tms20"
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
	Tiles []tile.Tile
}

type TileCoord struct {
	X, Y, Z uint
}

type PIndex interface {
	InsertGeometry(geometry geom.Geometry) error
	DetectTilesViaLineTrace(geometry geom.Geometry, tmsID tms20.TMID, buffer uint) []tile.Tile
	GetQBBoxWithBuffer(tmsID tms20.TMID, buffer uint) []tile.Tile
	InternalPixelLevelFromTmsID(tmsID tms20.TMID) pointindex.Level
	SnapClosestPoints(line geom.Line, levelMap map[pointindex.Level]any, ringID int) map[pointindex.Level][][2]float64
	GetHitMultiple(level pointindex.Level) map[intgeom.Point][]int
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
	ListTiles(zoomlevel uint, tileSet map[TileCoord]bool) error
	GetFeaturesForTile(tileX, tileY, tileZ uint) ([]tile.EncodedFeatureRow, error)
	GetAttributesForFeatures(featureIDs []int64) (tile.InternalAttributeTable, error)
	AttributeColumnNames() []string
}

type MVTTarget interface {
	WriteTile(x, y, z uint, data []byte) error
}
