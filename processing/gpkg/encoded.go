package gpkg

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/go-spatial/geom/encoding/gpkg"
	"github.com/pdok/texel/processing"
	"github.com/pdok/texel/tile"
)

func (t Table) EncodedName() string {
	return t.Name + "_encoded"
}

// List all columns except fid and geometry column
// Assumptions: fid column has index 0
func (t Table) AttributeColumnNames() []string {
	names := make([]string, 0, len(t.columns))
	for i, c := range t.columns {
		if i == 0 || c.name == t.gcolumn {
			continue // skip the fid and geometry columns, they are not attributes
		}
		names = append(names, c.name)
	}
	return names
}

func (t Table) insertSQLEncoded() string {
	return `INSERT INTO "` + t.EncodedName() + `"` +
		` (tile_x, tile_y, feature_id, geometry_type, data)` +
		` VALUES (?, ?, ?, ?, ?)`
}

// selectEncodedSQL builds the SELECT for fetching the encoded features of a single tile
func (t Table) selectEncodedSQL() string {
	return `SELECT feature_id, geometry_type, data FROM "` + t.EncodedName() + `" WHERE tile_x = ? AND tile_y = ?`
}

func serializeToBytes(intslice []uint32) []byte {
	bytes := make([]byte, len(intslice)*4)

	for i, x := range intslice {
		binary.LittleEndian.PutUint32(bytes[i*4:], x)
	}

	return bytes
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

// EncodedFeatureRow pairs a feature's identifier with its encoded, tile-relative geometry
// as read back from the "<table>_encoded" table.
type EncodedFeatureRow struct {
	FeatureID int64
	Geom      tile.EncodedGeometry
}

// GetFeaturesForTile returns every encoded feature stored for the given tile,
// i.e. the equivalent of the tile-features table's rows for (tileX, tileY).
func (source SourceGeopackage) GetFeaturesForTile(tileX, tileY uint) ([]EncodedFeatureRow, error) {
	rows, err := source.handle.Query(source.Table.selectEncodedSQL(), tileX, tileY)
	if err != nil {
		return nil, fmt.Errorf("querying encoded features for tile (%d,%d): %w", tileX, tileY, err)
	}
	defer rows.Close()

	var features []EncodedFeatureRow
	for rows.Next() {
		var featureID int64
		var geometryType int32
		var data []byte
		if err := rows.Scan(&featureID, &geometryType, &data); err != nil {
			return nil, fmt.Errorf("scanning encoded feature row for tile (%d,%d): %w", tileX, tileY, err)
		}
		features = append(features, EncodedFeatureRow{
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

func (target *TargetGeopackage) writeEncodedFeatures(encFeature []processing.FeatureForTileMatrix) {
	tx, err := target.handle.Begin()
	if err != nil {
		log.Fatalf("Could not start a transaction: %s", err)
	}

	stmt, err := tx.Prepare(target.Table.insertSQLEncoded())
	if err != nil {
		log.Fatalf("Could not prepare a statement: %s", err)
	}

	for _, ef := range encFeature {
		cols := ef.Columns()
		if len(cols) == 0 {
			log.Fatalf("Processing feature without data")
		}
		fid := cols[0]
		for _, eg := range ef.EncodedGeoms() {
			bytes := serializeToBytes(eg.Encoding)
			_, err := stmt.Exec(eg.XTile, eg.YTile, fid, eg.GeometryType, bytes)
			if err != nil {
				log.Fatalf("Could not get a result summary from the prepared statement for fid %s: %s", fid, err)
			}
		}

	}

	stmt.Close()
	_ = tx.Commit()
}

func buildEncodedTable(h *gpkg.Handle, t Table) error {
	db := h.DB

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY,
			tile_x INTEGER NOT NULL,
			tile_y INTEGER NOT NULL,
			feature_id INTEGER NOT NULL,
			geometry_type INTEGER NOT NULL,
			data BLOB
		)
		`, t.EncodedName())
	_, err := db.Exec(query)
	if err != nil {
		log.Println("error adding encoded table in target GeoPackage:", err)
		return err
	}
	return nil
}
