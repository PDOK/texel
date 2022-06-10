package gpkg

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/gpkg"
	"github.com/pdok/sieve/pkg"
)

type featureGPKG struct {
	columns  []interface{}
	geometry geom.Geometry
}

func (f featureGPKG) Columns() []interface{} {
	return f.columns
}

func (f featureGPKG) Geometry() geom.Geometry {
	return f.geometry
}

func (f *featureGPKG) UpdateGeometry(geometry geom.Geometry) {
	f.geometry = geometry
}

type column struct {
	cid       int
	name      string
	ctype     string
	notnull   int
	dfltValue *int
	pk        int
}

type Table struct {
	Name    string
	columns []column
	gcolumn string
	gtype   gpkg.GeometryType
	srs     gpkg.SpatialReferenceSystem
}

// geometryTypeFromString returns the numeric value of a gometry string
func geometryTypeFromString(geometrytype string) gpkg.GeometryType {
	switch strings.ToUpper(geometrytype) {
	case "GEOMETRY":
		return gpkg.Geometry
	case "POINT":
		return gpkg.Point
	case "LINESTRING":
		return gpkg.Linestring
	case "POLYGON":
		return gpkg.Polygon
	case "MULTIPOINT":
		return gpkg.MultiPoint
	case "MULTILINESTRING":
		return gpkg.MultiLinestring
	case "MULTIPOLYGON":
		return gpkg.MultiPolygon
	case "GEOMETRYCOLLECTION":
		return gpkg.GeometryCollection
	default:
		return gpkg.Geometry
	}
}

type SourceGeopackage struct {
	Table  Table
	handle *gpkg.Handle
}

func (source *SourceGeopackage) Init(file string) {
	source.handle = openGeopackage(file)
}

func (source SourceGeopackage) Close() {
	source.handle.Close()
}

func (source SourceGeopackage) ReadFeatures(preSieve chan pkg.Feature) {

	rows, err := source.handle.Query(source.Table.selectSQL())
	if err != nil {
		log.Fatalf("err during closing rows: %s", err)
	}

	cols, err := rows.Columns()
	if err != nil {
		log.Fatalf("error reading the columns: %s", err)
	}

	for rows.Next() {
		vals := make([]interface{}, len(cols))
		valPtrs := make([]interface{}, len(cols))
		for i := 0; i < len(cols); i++ {
			valPtrs[i] = &vals[i]
		}

		if err = rows.Scan(valPtrs...); err != nil {
			log.Fatalf("err reading row values: %v", err)
		}
		var f featureGPKG
		var c []interface{}

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
					for j := 0; j < len(v); j++ {
						asBytes[j] = v[j]
					}
					c = append(c, string(asBytes))
				case int64:
					c = append(c, v)
				case float64:
					c = append(c, v)
				case time.Time:
					c = append(c, v)
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
		preSieve <- ff
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
	close(preSieve)
	defer rows.Close()
}

func (source SourceGeopackage) GetTableInfo() []Table {
	query := `SELECT table_name, column_name, geometry_type_name, srs_id FROM gpkg_geometry_columns;`
	rows, err := source.handle.Query(query)
	if err != nil {
		log.Fatalf("error during closing rows: %v - %v", query, err)
	}
	var tables []Table

	for rows.Next() {
		var t Table
		var gtype string
		var srsID int
		err := rows.Scan(&t.Name, &t.gcolumn, &gtype, &srsID)
		if err != nil {
			log.Fatalf("error ready the source table information: %s", err)
		}

		t.columns = getTableColumns(source.handle, t.Name)
		t.gtype = geometryTypeFromString(gtype)
		t.srs = getSpatialReferenceSystem(source.handle, srsID)

		tables = append(tables, t)
	}
	defer rows.Close()
	return tables
}

type TargetGeopackage struct {
	Table    Table
	pagesize int
	handle   *gpkg.Handle
}

func (target *TargetGeopackage) Init(file string, pagesize int) {
	target.pagesize = pagesize
	target.handle = openGeopackage(file)
}

func (target TargetGeopackage) Close() {
	target.handle.Close()
}

func (target TargetGeopackage) CreateTables(tables []Table) error {
	for _, table := range tables {
		err := target.handle.UpdateSRS(table.srs)
		if err != nil {
			return err
		}

		err = buildTable(target.handle, table)
		if err != nil {
			return err
		}
	}
	return nil
}

func (target TargetGeopackage) WriteFeatures(postSieve chan pkg.Feature) {
	var features []interface{}

	for {
		feature, hasMore := <-postSieve
		if !hasMore {
			target.writeFeatures(features)
			break
		} else {
			features = append(features, feature)

			if len(features)%target.pagesize == 0 {
				target.writeFeatures(features)
				features = nil
			}
		}
	}
}

func (target TargetGeopackage) writeFeatures(features []interface{}) {
	tx, err := target.handle.Begin()
	if err != nil {
		log.Fatalf("Could not start a transaction: %s", err)
	}

	stmt, err := tx.Prepare(target.Table.insertSQL())
	if err != nil {
		log.Fatalf("Could not prepare a statement: %s", err)
	}

	var ext *geom.Extent

	for _, feature := range features {
		f := feature.(*featureGPKG)
		sb, err := gpkg.NewBinary(int32(target.Table.srs.ID), f.geometry)
		if err != nil {
			log.Fatalf("Could not create a binary geometry: %s", err)
		}

		data := f.columns
		data = append(data, sb)

		_, err = stmt.Exec(data...)
		if err != nil {
			var fid interface{} = "unknown"
			if len(data) > 0 {
				fid = data[0]
			}
			log.Fatalf("Could not get a result summary from the prepared statement for fid %s: %s", fid, err)
		}

		if ext == nil {
			ext, err = geom.NewExtentFromGeometry(f.geometry)
			if err != nil {
				ext = nil
				log.Println("Failed to create new extent:", err)
				continue
			}
		} else {
			ext.AddGeometry(f.geometry)
		}
	}
	stmt.Close()
	tx.Commit()

	err = target.handle.UpdateGeometryExtent(target.Table.Name, ext)
	if err != nil {
		log.Fatalln("Failed to update new extent:", err)
	}

}

func openGeopackage(file string) *gpkg.Handle {
	handle, err := gpkg.Open(file)
	if err != nil {
		log.Fatalf("error opening GeoPackage: %s", err)
	}
	return handle
}

// createSQL creates a CREATE statement on the given table and column information
// used for creating feature tables in the target Geopackage
func (t Table) createSQL() string {
	create := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%v"`, t.Name)
	var columnparts []string
	for _, column := range t.columns {
		columnpart := column.name + ` ` + column.ctype
		if column.notnull == 1 {
			columnpart = columnpart + ` NOT NULL`
		}
		if column.pk == 1 {
			columnpart = columnpart + ` PRIMARY KEY`
		}

		columnparts = append(columnparts, columnpart)
	}

	query := create + `(` + strings.Join(columnparts, `, `) + `);`
	return query
}

// selectSQL build a SELECT statement based on the table and columns
// used for reading the source features
func (t Table) selectSQL() string {
	var csql []string
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

// getSpatialReferenceSystem extracts this based on the given SRS id
func getSpatialReferenceSystem(h *gpkg.Handle, id int) gpkg.SpatialReferenceSystem {
	var srs gpkg.SpatialReferenceSystem
	query := `SELECT srs_name, srs_id, organization, organization_coordsys_id, definition, description FROM gpkg_spatial_ref_sys WHERE srs_id = %v;`

	row := h.QueryRow(fmt.Sprintf(query, id))
	var description *string
	row.Scan(&srs.Name, &srs.ID, &srs.Organization, &srs.OrganizationCoordsysID, &srs.Definition, &description)
	if description != nil {
		srs.Description = *description
	}

	return srs
}

// getTableColumns collects the column information of a given table
func getTableColumns(h *gpkg.Handle, table string) []column {
	var columns []column
	query := `PRAGMA table_info('%v');`
	rows, err := h.Query(fmt.Sprintf(query, table))

	if err != nil {
		log.Fatalf("err during closing rows: %v - %v", query, err)
	}

	for rows.Next() {
		var column column
		err := rows.Scan(&column.cid, &column.name, &column.ctype, &column.notnull, &column.dfltValue, &column.pk)
		if err != nil {
			log.Fatalf("error getting the column information: %s", err)
		}
		columns = append(columns, column)
	}
	defer rows.Close()
	return columns
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
		SRS:           int32(t.srs.ID),
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
