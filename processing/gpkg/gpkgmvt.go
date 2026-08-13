package gpkg

// Specializes the gpkg.go functionaltity for the `texel mvt` command

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pdok/texel/processing"
	"github.com/pdok/texel/tile"
)

// Batch value for reading features from the geopackage table
const maxSQLiteBatchParams = 500

type MVTSourceGeopackage struct {
	Table Table
	geopackageHandle
}

// GetAttributesForFeatures fetches the attributes (i.e. all non-geometry, non-fid columns)
// for the given feature IDs, batching the query to respect SQLite's parameter limits.
// The fid is assumed to be the table's first column.
func (source MVTSourceGeopackage) GetAttributesForFeatures(featureIDs []int64) (tile.InternalAttributeTable, error) {
	attributes := make(tile.InternalAttributeTable, len(featureIDs))
	fidColumn := source.Table.columns[0].name

	for start := 0; start < len(featureIDs); start += maxSQLiteBatchParams {
		batch := featureIDs[start:min(start+maxSQLiteBatchParams, len(featureIDs))]

		batchAsAny := make([]any, len(batch))
		for i, fid := range batch {
			batchAsAny[i] = fid
		}

		if err := source.queryAttributesBatch(fidColumn, batchAsAny, attributes); err != nil {
			return nil, err
		}
	}

	return attributes, nil
}

// selectByFIDsSQL builds a SELECT of all the table's columns for a batch of feature IDs.
// The fid is assumed to be the table's first column.
func (t Table) selectByFIDsSQL(batchSize int) string {
	var csql []string //nolint:prealloc
	for _, c := range t.columns {
		csql = append(csql, c.name)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", batchSize), ",")
	fidColumn := t.columns[0].name
	return `SELECT ` + strings.Join(csql, `,`) + ` FROM "` + t.Name + `" WHERE ` + fidColumn + ` IN (` + placeholders + `)`
}

func cleanData(v any) (any, bool) {
	switch v := v.(type) {
	case []uint8:
		asBytes := make([]byte, len(v))
		copy(asBytes, v)
		return v, true
	case int64:
		return v, true
	case float64:
		return v, true
	case time.Time:
		return v.Format(time.RFC3339), true
	case string:
		return v, true
	case nil:
		return nil, true
	default:
		return nil, false
	}
}

// attributeColumnNames lists all columns except fid and geometry column.
// Assumptions: fid column has index 0
func (t Table) attributeColumnNames() []string {
	names := make([]string, 0, len(t.columns))
	for i, c := range t.columns {
		if i == 0 || c.name == t.gcolumn {
			continue // skip the fid and geometry columns, they are not attributes
		}
		names = append(names, c.name)
	}
	return names
}

// AttributeColumnNames satisfies processing.MVTSource by delegating to the
// current Table.
func (source MVTSourceGeopackage) AttributeColumnNames() []string {
	return source.Table.attributeColumnNames()
}

// selectEncodedSQL builds the SELECT for fetching the encoded features of a single tile
func (t Table) selectEncodedSQL() string {
	return `SELECT feature_id, geometry_type, data FROM "` + t.EncodedName() + `" WHERE tile_x = ? AND tile_y = ? AND tile_z = ?`
}

// selectDistinctTilesSQL builds the SELECT for enumerating every tile that has at
// least one encoded feature.
func (t Table) selectDistinctTilesSQL() string {
	return `SELECT DISTINCT tile_x, tile_y FROM "` + t.EncodedName() + `" WHERE tile_z = ? ORDER BY tile_x, tile_y`
}

// GetFeaturesForTile returns every encoded feature stored for the given tile,
// i.e. the equivalent of the tile-features table's rows for (tileX, tileY).
func (source MVTSourceGeopackage) GetFeaturesForTile(tileX, tileY, tileZ uint) ([]tile.EncodedFeatureRow, error) {
	rows, err := source.handle.Query(source.Table.selectEncodedSQL(), tileX, tileY, tileZ)
	if err != nil {
		return nil, fmt.Errorf("querying encoded features for tile (%d,%d,%d): %w", tileX, tileY, tileZ, err)
	}
	defer rows.Close()

	var features []tile.EncodedFeatureRow
	for rows.Next() {
		var featureID int64
		var geometryType int32
		var data []byte
		if err := rows.Scan(&featureID, &geometryType, &data); err != nil {
			return nil, fmt.Errorf("scanning encoded feature row for tile (%d,%d): %w", tileX, tileY, err)
		}
		features = append(features, tile.EncodedFeatureRow{
			FeatureID: featureID,
			Geom: tile.EncodedGeometry{
				Encoding:     deserializeFromBytes(data),
				GeometryType: geometryType,
				XTile:        tileX,
				YTile:        tileY,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating encoded feature rows for tile (%d,%d): %w", tileX, tileY, err)
	}
	return features, nil
}

// deserializeFromBytes is the inverse of serializeToBytes: it turns a BLOB of
// little-endian uint32s back into the encoded geometry command/coordinate stream.
func deserializeFromBytes(data []byte) []uint32 {
	intslice := make([]uint32, len(data)/4)
	for i := range intslice {
		intslice[i] = binary.LittleEndian.Uint32(data[i*4:])
	}
	return intslice
}

// ListTiles returns every distinct tile present in the "<table>_encoded" table.
func (source MVTSourceGeopackage) ListTiles(z uint, tileSet map[processing.TileCoord]bool) error {
	rows, err := source.handle.Query(source.Table.selectDistinctTilesSQL(), z)
	if err != nil {
		return fmt.Errorf("listing tiles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var x, y int64
		if err := rows.Scan(&x, &y); err != nil {
			return fmt.Errorf("scanning tile row: %w", err)
		}
		tileSet[processing.TileCoord{X: uint(x), Y: uint(y), Z: z}] = true //nolint:gosec // G115 Tile coord fit within uint
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating tile rows: %w", err)
	}
	return nil
}

// Read from source.Table, store rows as attributes[fid][columnName] = value for all fids
func (source MVTSourceGeopackage) queryAttributesBatch(fidColumn string, fids []any, attributes tile.InternalAttributeTable) error {
	// Read relevant rows
	rows, err := source.handle.Query(source.Table.selectByFIDsSQL(len(fids)), fids...)
	if err != nil {
		return fmt.Errorf("querying attributes for %d feature(s): %w", len(fids), err)
	}
	defer rows.Close()

	cols := source.getColNames()

	// Process rows
	for rows.Next() {
		// Read data with helper vars
		vals := make([]any, len(cols))
		valPtrs := make([]any, len(cols))
		for i := range cols {
			valPtrs[i] = &vals[i]
		}
		if err := rows.Scan(valPtrs...); err != nil {
			return fmt.Errorf("scanning attribute row: %w", err)
		}

		// Interpret data
		row := make(map[string]any, len(cols))
		var fid int64
		var fidFound bool
		for i, colName := range cols {
			switch colName {
			case fidColumn:
				// Read fid
				id, ok := vals[i].(int64)
				if !ok {
					return fmt.Errorf("expected int64 fid for column %s, got %T", colName, vals[i])
				}
				fid, fidFound = id, true
			case source.Table.gcolumn:
				// Skip geometry column
			default:
				// Interpret data
				v, ok := cleanData(vals[i])
				if !ok {
					return fmt.Errorf("unexpected type for sqlite column data: %v: %T", colName, v)
				}
				row[colName] = v
			}
		}
		if !fidFound {
			return fmt.Errorf("row did not contain fid column %s", fidColumn)
		}
		attributes[fid] = row
	}
	return rows.Err()
}

func (source MVTSourceGeopackage) getColNames() []string {
	cols := source.Table.columns
	colNames := make([]string, len(cols))
	for i, col := range cols {
		colNames[i] = col.name
	}
	return colNames
}

// MVTFileTarget writes built MVT tiles to <OutDir>/<tileX>/<tileY>.mvt.
type MVTFileTarget struct {
	OutDir string
}

// WriteTile writes one tile's serialized bytes to <OutDir>/<tileX>/<tileY>.mvt,
// creating directories as needed.
func (t *MVTFileTarget) WriteTile(x, y, z uint, data []byte) error {
	dir := filepath.Join(t.OutDir, strconv.FormatUint(uint64(z), 10), strconv.FormatUint(uint64(x), 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating tile directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, strconv.FormatUint(uint64(y), 10)+".mvt")
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // G306 tile output does not need restrictive permissions
		return fmt.Errorf("writing tile file %s: %w", path, err)
	}
	return nil
}
