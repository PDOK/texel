package gpkg

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/go-spatial/geom/encoding/gpkg"
	"github.com/pdok/texel/processing"
)

func (t Table) EncodedName() string {
	return t.Name + "_encoded"
}

func (t Table) insertSQLEncoded() string {
	return `INSERT INTO "` + t.EncodedName() + `"` +
		` (tile_x, tile_y, feature_id, geometry_type, data)` +
		` VALUES (?, ?, ?, ?, ?)`
}

func serializeToBytes(intslice []uint32) []byte {
	bytes := make([]byte, len(intslice)*4)

	for i, x := range intslice {
		binary.LittleEndian.PutUint32(bytes[i*4:], x)
	}

	return bytes
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
