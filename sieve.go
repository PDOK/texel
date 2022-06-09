package main

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/gpkg"
)

type feature struct {
	columns  []interface{}
	geometry geom.Geometry
}

type features [][]interface{}

type column struct {
	cid       int
	name      string
	ctype     string
	notnull   int
	dfltValue *int
	pk        int
}

type table struct {
	name    string
	columns []column
	gcolumn string
	gtype   gpkg.GeometryType
	srs     gpkg.SpatialReferenceSystem
}

type Source interface {
	ReadFeatures(table, chan feature)
	GetTableInfo() []table
}

type SourceGeopackage struct {
	handle *gpkg.Handle
}

func (source SourceGeopackage) ReadFeatures(t table, preSieve chan feature) {

	rows, err := source.handle.Query(t.selectSQL())
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
		var f feature
		var c []interface{}

		for i, colName := range cols {
			switch colName {
			case t.gcolumn:
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
		preSieve <- f
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
	close(preSieve)
	defer rows.Close()
}

func (source SourceGeopackage) GetTableInfo() []table {
	query := `SELECT table_name, column_name, geometry_type_name, srs_id FROM gpkg_geometry_columns;`
	rows, err := source.handle.Query(query)
	if err != nil {
		log.Fatalf("error during closing rows: %v - %v", query, err)
	}
	var tables []table

	for rows.Next() {
		var t table
		var gtype string
		var srsID int
		err := rows.Scan(&t.name, &t.gcolumn, &gtype, &srsID)
		if err != nil {
			log.Fatalf("error ready the source table information: %s", err)
		}

		t.columns = getTableColumns(source.handle, t.name)
		t.gtype = geometryTypeFromString(gtype)
		t.srs = getSpatialReferenceSystem(source.handle, srsID)

		tables = append(tables, t)
	}
	defer rows.Close()
	return tables
}

type Target interface {
	CreateTables([]table) error
	WriteFeatures(features, table)
	UpdateGeometryExtent(string, *geom.Extent)
}

type TargetGeopackage struct {
	handle *gpkg.Handle
}

func (target TargetGeopackage) CreateTables(tables []table) error {
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

func (target TargetGeopackage) WriteFeatures(features features, t table) {
	tx, err := target.handle.Begin()
	if err != nil {
		log.Fatalf("Could not start a transaction: %s", err)
	}

	stmt, err := tx.Prepare(t.insertSQL())
	if err != nil {
		log.Fatalf("Could not prepare a statement: %s", err)
	}

	for _, f := range features {
		_, err = stmt.Exec(f...)
		if err != nil {
			var fid interface{} = "unknown"
			if len(f) > 0 {
				fid = f[0]
			}
			log.Fatalf("Could not get a result summary from the prepared statement for fid %s: %s", fid, err)
		}
	}

	stmt.Close()
	tx.Commit()
}

func (target TargetGeopackage) UpdateGeometryExtent(table string, ext *geom.Extent) {
	target.handle.UpdateGeometryExtent(table, ext)
}

func openGeopackage(file string) *gpkg.Handle {
	handle, err := gpkg.Open(file)
	if err != nil {
		log.Fatalf("error opening GeoPackage: %s", err)
	}
	return handle
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

// createSQL creates a CREATE statement on the given table and column information
// used for creating feature tables in the target Geopackage
func (t table) createSQL() string {
	create := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%v"`, t.name)
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
func (t table) selectSQL() string {
	var csql []string
	for _, c := range t.columns {
		csql = append(csql, c.name)
	}
	query := `SELECT ` + strings.Join(csql, `,`) + ` FROM "` + t.name + `";`
	return query
}

// insertSQL used for writing the features
// build the INSERT statement based on the table and columns
func (t table) insertSQL() string {
	var csql, vsql []string
	for _, c := range t.columns {
		if c.name != t.gcolumn {
			csql = append(csql, c.name)
			vsql = append(vsql, `?`)
		}
	}
	csql = append(csql, t.gcolumn)
	vsql = append(vsql, `?`)
	query := `INSERT INTO "` + t.name + `"(` + strings.Join(csql, `,`) + `) VALUES(` + strings.Join(vsql, `,`) + `)`
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
func buildTable(h *gpkg.Handle, t table) error {
	query := t.createSQL()
	_, err := h.Exec(query)
	if err != nil {
		log.Fatalf("error building table in target GeoPackage: %s", err)
	}

	err = h.AddGeometryTable(gpkg.TableDescription{
		Name:          t.name,
		ShortName:     t.name,
		Description:   t.name,
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

// readFeatures reads the features from the given Geopackage table
// and decodes the WKB geometry to a geom.Polygon
func readFeaturesFromSource(source Source, preSieve chan feature, t table) {
	source.ReadFeatures(t, preSieve)
}

// sieveFeatures sieves/filters the geometry against the given resolution
// the two steps that are done are:
// 1. filter features with a area smaller then the (resolution*resolution)
// 2. removes interior rings with a area smaller then the (resolution*resolution)
func sieveFeatures(preSieve chan feature, postSieve chan feature, resolution float64) {
	var preSieveCount, postSieveCount, nonPolygonCount, multiPolygonCount uint64
	for {
		feature, hasMore := <-preSieve
		if !hasMore {
			break
		} else {
			preSieveCount++
			switch gpkg.TypeForGeometry(feature.geometry) {
			case gpkg.Polygon:
				var p geom.Polygon
				p = feature.geometry.(geom.Polygon)
				if p := polygonSieve(p, resolution); p != nil {
					feature.geometry = p
					postSieveCount++
					postSieve <- feature
				}
			case gpkg.MultiPolygon:
				var mp geom.MultiPolygon
				mp = feature.geometry.(geom.MultiPolygon)
				if mp := multiPolygonSieve(mp, resolution); mp != nil {
					feature.geometry = mp
					multiPolygonCount++
					postSieveCount++
					postSieve <- feature
				}
			default:
				postSieveCount++
				nonPolygonCount++
				postSieve <- feature
			}
		}
	}
	close(postSieve)

	log.Printf("    total features: %d", preSieveCount)
	log.Printf("      non-polygons: %d", nonPolygonCount)
	if preSieveCount != nonPolygonCount {
		log.Printf("     multipolygons: %d", multiPolygonCount)
	}
	log.Printf("              kept: %d", postSieveCount)
}

// writeFeatures collects the processed features by the sieveFeatures and
// creates a WKB binary from the geometry
// The collected feature array, based on the pagesize, is then passed to the writeFeaturesArray
func writeFeaturesToTarget(postSieve chan feature, kill chan bool, target Target, t table, p int) {
	var ext *geom.Extent
	var err error

	var features [][]interface{}

	for {
		feature, hasMore := <-postSieve
		if !hasMore {
			target.WriteFeatures(features, t)
			break
		} else {
			sb, err := gpkg.NewBinary(int32(t.srs.ID), feature.geometry)
			if err != nil {
				log.Fatalf("Could not create a binary geometry: %s", err)
			}

			data := feature.columns
			data = append(data, sb)
			features = append(features, data)

			if len(features)%p == 0 {
				target.WriteFeatures(features, t)
				features = nil
			}
		}

		if ext == nil {
			ext, err = geom.NewExtentFromGeometry(feature.geometry)
			if err != nil {
				ext = nil
				log.Println("Failed to create new extent:", err)
				continue
			}
		} else {
			ext.AddGeometry(feature.geometry)
		}
	}
	target.UpdateGeometryExtent(t.name, ext)
	kill <- true
}

// multiPolygonSieve will split it self into the separated polygons that will be sieved before building a new MULTIPOLYGON
func multiPolygonSieve(mp geom.MultiPolygon, resolution float64) geom.MultiPolygon {
	var sievedMultiPolygon geom.MultiPolygon
	for _, p := range mp {
		if sievedPolygon := polygonSieve(p, resolution); sievedPolygon != nil {
			sievedMultiPolygon = append(sievedMultiPolygon, sievedPolygon)
		}
	}
	return sievedMultiPolygon
}

// polygonSieve will sieve a given POLYGON
func polygonSieve(p geom.Polygon, resolution float64) geom.Polygon {
	minArea := resolution * resolution
	if area(p) > minArea {
		if len(p) > 1 {
			var sievedPolygon geom.Polygon
			sievedPolygon = append(sievedPolygon, p[0])
			for _, interior := range p[1:] {
				if shoelace(interior) > minArea {
					sievedPolygon = append(sievedPolygon, interior)
				}
			}
			return sievedPolygon
		}
		return p
	}
	return nil
}

// calculate the area of a polygon
func area(geom [][][2]float64) float64 {
	interior := .0
	if geom == nil {
		return 0.
	}
	if len(geom) > 1 {
		for _, i := range geom[1:] {
			interior = interior + shoelace(i)
		}
	}
	return shoelace(geom[0]) - interior
}

// https://en.wikipedia.org/wiki/Shoelace_formula
func shoelace(pts [][2]float64) float64 {
	sum := 0.
	if len(pts) == 0 {
		return 0.
	}

	p0 := pts[len(pts)-1]
	for _, p1 := range pts {
		sum += p0[1]*p1[0] - p0[0]*p1[1]
		p0 = p1
	}
	return math.Abs(sum / 2)
}
