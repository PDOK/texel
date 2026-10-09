package gpkg

// Specializes the gpkg.go functionality for the `texel snap` command.

import (
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/gpkg"
	"github.com/pdok/texel/processing"
)

// Abstraction of a source. Table is a field that represents the table that is
// currently being processed.
type SourceGeopackage struct {
	Table Table
	geopackageHandle
}

// Abstraction of target geopackage. Table represents the table that is
// currently being processed.
type TargetGeopackage struct {
	Table    Table
	pagesize int
	geopackageHandle
	EncodeTiles bool
}

// Overrides geopackageHandle.Init
func (target *TargetGeopackage) Init(file string, pagesize int) {
	target.pagesize = pagesize
	target.geopackageHandle.Init(file)
}

type featureGPKG struct {
	columns  []any
	geometry geom.Geometry
}

func (f featureGPKG) Columns() []any {
	return f.columns
}

func (f featureGPKG) Geometry() geom.Geometry {
	return f.geometry
}

//nolint:funlen,cyclop
func (source SourceGeopackage) ReadFeatures(features chan<- processing.Feature) {
	rows, err := source.handle.Query(source.Table.selectSQL())
	if err != nil {
		log.Fatalf("err during closing rows: %s", err)
	}

	cols, err := rows.Columns()
	if err != nil {
		log.Fatalf("error reading the columns: %s", err)
	}

	for rows.Next() {
		vals := make([]any, len(cols))
		valPtrs := make([]any, len(cols))
		for i := range cols {
			valPtrs[i] = &vals[i]
		}

		if err = rows.Scan(valPtrs...); err != nil {
			log.Fatalf("err reading row values: %v", err)
		}
		var f featureGPKG
		var c []any

		for i, colName := range cols {
			switch colName {
			case source.Table.gcolumn:
				wkbgeom, err := gpkg.DecodeGeometry(vals[i].([]byte))
				if err != nil {
					log.Fatalf("error decoding the geometry: %s", err)
				}
				f.geometry = wkbgeom.Geometry
			default:
				switch v := vals[i].(type) {
				case []uint8:
					asBytes := make([]byte, len(v))
					copy(asBytes, v)
					c = append(c, string(asBytes))
				case int64:
					c = append(c, v)
				case float64:
					c = append(c, v)
				case time.Time:
					c = append(c, v.Format(time.RFC3339))
				case string:
					c = append(c, v)
				case nil:
					c = append(c, v)
				default:
					log.Fatalf("unexpected type for sqlite column data: %v: %T", cols[i], v)
				}
			}
			f.columns = c
		}
		ff := &f
		features <- ff
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
	close(features)
	defer rows.Close()
}

// selectSQL build a SELECT statement based on the table and columns
// used for reading the source features
func (t Table) selectSQL() string {
	var csql []string //nolint:prealloc
	for _, c := range t.columns {
		csql = append(csql, c.name)
	}
	query := `SELECT ` + strings.Join(csql, `,`) + ` FROM "` + t.Name + `";`
	return query
}

// insertSQL used for writing the features
// build the INSERT statement based on the table and columns
func (t Table) insertSQL() string {
	var csql, vsql []string
	for _, c := range t.columns {
		if c.name != t.gcolumn {
			csql = append(csql, c.name)
			vsql = append(vsql, `?`)
		}
	}
	csql = append(csql, t.gcolumn)
	vsql = append(vsql, `?`)
	query := `INSERT INTO "` + t.Name + `"(` + strings.Join(csql, `,`) + `) VALUES(` + strings.Join(vsql, `,`) + `)`
	return query
}

// insertSQLEncoded used for writing the encoded geometries when -enc is on
func (t Table) insertSQLEncoded() string {
	return `INSERT INTO "` + t.EncodedName() + `"` +
		` (tile_x, tile_y, tile_z, feature_id, geometry_type, data)` +
		` VALUES (?, ?, ?, ?, ?, ?)`
}

func serializeToBytes(intslice []uint32) []byte {
	bytes := make([]byte, len(intslice)*4)

	for i, x := range intslice {
		binary.LittleEndian.PutUint32(bytes[i*4:], x)
	}

	return bytes
}

// Create tables in target. Defers to buildTable and buildEncodedTable
func (target *TargetGeopackage) CreateTables(tables []Table) error {
	for _, table := range tables {
		err := target.handle.UpdateSRS(table.srs)
		if err != nil {
			return err
		}

		err = buildTable(target.handle, table)
		if err != nil {
			return err
		}

		if target.EncodeTiles {
			err = buildEncodedTable(target.handle, table)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Write the features to geopackage. Defers to writeFeatures and writeEncodedFeatures
func (target *TargetGeopackage) WriteFeatures(inFeatures <-chan processing.FeatureForTileMatrix) {
	var features []processing.FeatureForTileMatrix

	for {
		feature, hasMore := <-inFeatures
		if !hasMore {
			target.writeFeatures(features)
			if target.EncodeTiles {
				target.writeEncodedFeatures(features)
			}
			break
		}
		features = append(features, feature)

		if len(features)%target.pagesize == 0 {
			target.writeFeatures(features)
			if target.EncodeTiles {
				target.writeEncodedFeatures(features)
			}
			features = nil
		}
	}
}

// Write snapped features to table
func (target *TargetGeopackage) writeFeatures(features []processing.FeatureForTileMatrix) {
	tx, err := target.handle.Begin()
	if err != nil {
		log.Fatalf("Could not start a transaction: %s", err)
	}

	stmt, err := tx.Prepare(target.Table.insertSQL())
	if err != nil {
		log.Fatalf("Could not prepare a statement: %s", err)
	}

	var ext *geom.Extent

	for _, f := range features {
		//nolint:gosec // G115
		sb, err := gpkg.NewBinary(int32(target.Table.srs.ID), f.Geometry())
		if err != nil {
			log.Fatalf("Could not create a binary geometry: %s", err)
		}

		data := f.Columns()
		data = append(data, sb)

		_, err = stmt.Exec(data...)
		if err != nil {
			var fid any = "unknown"
			if len(data) > 0 {
				fid = data[0]
			}
			log.Fatalf("Could not get a result summary from the prepared statement for fid %s: %s", fid, err)
		}

		if ext == nil {
			ext, err = geom.NewExtentFromGeometry(f.Geometry())
			if err != nil {
				ext = nil
				log.Println("Failed to create new extent:", err)
				continue
			}
		} else {
			_ = ext.AddGeometry(f.Geometry())
		}
	}
	stmt.Close()
	_ = tx.Commit()

	err = target.handle.UpdateGeometryExtent(target.Table.Name, ext)
	if err != nil {
		log.Fatalln("Failed to update new extent:", err)
	}
}

// Write encoded geometries to corresponding tables
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
			_, err := stmt.Exec(eg.XTile, eg.YTile, uint(ef.TileMatrixID()), fid, eg.GeometryType, bytes) //nolint:gosec // G115 integers < 40
			if err != nil {
				log.Fatalf("Could not get a result summary from the prepared statement for fid %s: %s", fid, err)
			}
		}

	}

	stmt.Close()
	_ = tx.Commit()
}

// createSQL creates a CREATE statement on the given table and column information
// used for creating feature tables in the target Geopackage
func (t Table) createSQL() string {
	create := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%v"`, t.Name)
	var columnparts []string //nolint:prealloc
	for _, column := range t.columns {
		columnpart := column.name + ` ` + column.ctype
		if column.notnull == 1 {
			columnpart += ` NOT NULL`
		}
		if column.pk == 1 {
			columnpart += ` PRIMARY KEY`
		}

		columnparts = append(columnparts, columnpart)
	}

	query := create + `(` + strings.Join(columnparts, `, `) + `);`
	return query
}

// buildTable creates a given destination table with the necessary gpkg_ information
func buildTable(h *gpkg.Handle, t Table) error {
	query := t.createSQL()
	_, err := h.Exec(query)
	if err != nil {
		log.Fatalf("error building table in target GeoPackage: %s", err)
	}

	err = h.AddGeometryTable(gpkg.TableDescription{
		Name:          t.Name,
		ShortName:     t.Name,
		Description:   t.Name,
		GeometryField: t.gcolumn,
		GeometryType:  t.gtype,
		//nolint:gosec // G115
		SRS: int32(t.srs.ID),
		//
		Z: gpkg.Prohibited,
		M: gpkg.Prohibited,
	})
	if err != nil {
		log.Println("error adding geometry table in target GeoPackage:", err)
		return err
	}
	return nil
}

// create table for storing encoded geometries
func buildEncodedTable(h *gpkg.Handle, t Table) error {
	db := h.DB

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY,
			tile_x INTEGER NOT NULL,
			tile_y INTEGER NOT NULL,
			tile_z INTEGER NOT NULL,
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
