package gpkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pdok/texel/tile"
)

// TileCoord identifies a tile by its column/row in the tile matrix.
type TileCoord struct {
	X, Y uint
}

// selectDistinctTilesSQL builds the SELECT for enumerating every tile that has at
// least one encoded feature.
func (t Table) selectDistinctTilesSQL() string {
	return `SELECT DISTINCT tile_x, tile_y FROM "` + t.EncodedName() + `" ORDER BY tile_x, tile_y`
}

// ListTiles returns every distinct tile present in the "<table>_encoded" table.
func (source SourceGeopackage) ListTiles() ([]TileCoord, error) {
	rows, err := source.handle.Query(source.Table.selectDistinctTilesSQL())
	if err != nil {
		return nil, fmt.Errorf("listing tiles: %w", err)
	}
	defer rows.Close()

	var tiles []TileCoord
	for rows.Next() {
		var x, y int64
		if err := rows.Scan(&x, &y); err != nil {
			return nil, fmt.Errorf("scanning tile row: %w", err)
		}
		tiles = append(tiles, TileCoord{X: uint(x), Y: uint(y)}) //nolint:gosec // G115 Tile coords fit within uint
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tile rows: %w", err)
	}
	return tiles, nil
}

// BuildMVTTile builds and serializes the MVT tile for (tileX, tileY): it fetches the
// tile's encoded features and their attributes, builds the per-tile value dictionary,
// assembles a single layer named layerName using the given (already built once,
// fixed-schema) key dictionary, and marshals the result to MVT protobuf bytes. A tile
// with no encoded features still yields a valid, empty layer rather than an error.
func (source SourceGeopackage) BuildMVTTile(layerName string, keyIndex map[string]uint32, tileX, tileY uint) ([]byte, error) {
	rows, err := source.GetFeaturesForTile(tileX, tileY)
	if err != nil {
		return nil, fmt.Errorf("getting features for tile (%d,%d): %w", tileX, tileY, err)
	}

	featureIDs := make([]int64, len(rows))
	for i, row := range rows {
		featureIDs[i] = row.FeatureID
	}

	attributes, err := source.GetAttributesForFeatures(featureIDs)
	if err != nil {
		return nil, fmt.Errorf("getting attributes for tile (%d,%d): %w", tileX, tileY, err)
	}
	valueIndex := tile.BuildValueDictionary(attributes)

	features := make([]*tile.GSTileFeature, 0, len(rows))
	for _, row := range rows {
		attrs := AttributesForFeature(attributes, row.FeatureID)
		feature, err := tile.BuildFeature(row.FeatureID, row.Geom, attrs, keyIndex, valueIndex)
		if err != nil {
			return nil, fmt.Errorf("building tile (%d,%d): %w", tileX, tileY, err)
		}
		features = append(features, feature)
	}

	layer := tile.BuildLayer(layerName, features, keyIndex, valueIndex)

	data, err := tile.MarshalTile(tile.BuildTile(layer))
	if err != nil {
		return nil, fmt.Errorf("marshaling tile (%d,%d): %w", tileX, tileY, err)
	}
	return data, nil
}

// WriteTileFile writes one tile's serialized bytes to <baseDir>/<tileX>/<tileY>.mvt,
// creating directories as needed.
func WriteTileFile(baseDir string, tileX, tileY uint, data []byte) error {
	dir := filepath.Join(baseDir, strconv.FormatUint(uint64(tileX), 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating tile directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, strconv.FormatUint(uint64(tileY), 10)+".mvt")
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // G306 tile output does not need restrictive permissions
		return fmt.Errorf("writing tile file %s: %w", path, err)
	}
	return nil
}

// ProcessTilesToMVT enumerates every tile with encoded features for source.Table,
// builds each tile's MVT bytes (see BuildMVTTile) and writes it to outDir via
// WriteTileFile. The key dictionary is built once, up front, since the attribute
// schema is fixed for the whole table (see tile.BuildKeyDictionary).
func (source SourceGeopackage) ProcessTilesToMVT(outDir string) error {
	keyIndex := tile.BuildKeyDictionary(source.Table.AttributeColumnNames())

	tiles, err := source.ListTiles()
	if err != nil {
		return err
	}

	for _, tc := range tiles {
		data, err := source.BuildMVTTile(source.Table.Name, keyIndex, tc.X, tc.Y)
		if err != nil {
			return fmt.Errorf("processing tile (%d,%d): %w", tc.X, tc.Y, err)
		}
		if err := WriteTileFile(outDir, tc.X, tc.Y, data); err != nil {
			return fmt.Errorf("processing tile (%d,%d): %w", tc.X, tc.Y, err)
		}
	}
	return nil
}
