package main

import (
	"flag"
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
	create := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %v`, t.name)
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
	query := `SELECT ` + strings.Join(csql, `,`) + ` FROM ` + t.name + `;`
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
	query := `INSERT INTO ` + t.name + `(` + strings.Join(csql, `,`) + `) VALUES(` + strings.Join(vsql, `,`) + `)`
	return query
}

// getSourceTableInfo collects the source table information
func getSourceTableInfo(h *gpkg.Handle) []table {
	query := `SELECT table_name, column_name, geometry_type_name, srs_id FROM gpkg_geometry_columns;`
	rows, err := h.Query(query)
	defer rows.Close()
	if err != nil {
		log.Printf("err during closing rows: %v - %v", query, err)
	}
	var tables []table

	for rows.Next() {
		var t table
		var gtype string
		var srsID int
		err := rows.Scan(&t.name, &t.gcolumn, &gtype, &srsID)
		if err != nil {
			log.Fatal(err)
		}

		t.columns = getTableColumns(h, t.name)
		t.gtype = geometryTypeFromString(gtype)
		t.srs = getSpatialReferenceSystem(h, srsID)

		tables = append(tables, t)
	}
	return tables
}

// getSpatialReferenceSystem extracts this based on the given SRS id
func getSpatialReferenceSystem(h *gpkg.Handle, id int) gpkg.SpatialReferenceSystem {
	var srs gpkg.SpatialReferenceSystem
	query := `SELECT srs_name, srs_id, organization, organization_coordsys_id, definition, description FROM gpkg_spatial_ref_sys WHERE srs_id = %v;`
	rows, err := h.Query(fmt.Sprintf(query, id))
	defer rows.Close()
	if err != nil {
		log.Printf("err during closing rows: %v - %v", query, err)
	}

	for rows.Next() {
		var description *string
		err := rows.Scan(&srs.Name, &srs.ID, &srs.Organization, &srs.OrganizationCoordsysID, &srs.Definition, &description)
		if description != nil {
			srs.Description = *description
		}
		if err != nil {
			log.Fatal(err)
		}
		// On first hit (and only) return
		return srs
	}
	return srs
}

// getTableColumns collects the column information of a given table
func getTableColumns(h *gpkg.Handle, table string) []column {
	var columns []column
	query := `PRAGMA table_info('%v');`
	rows, err := h.Query(fmt.Sprintf(query, table))
	defer rows.Close()
	if err != nil {
		log.Printf("err during closing rows: %v - %v", query, err)
	}

	for rows.Next() {
		var column column
		err := rows.Scan(&column.cid, &column.name, &column.ctype, &column.notnull, &column.dfltValue, &column.pk)
		if err != nil {
			log.Fatal(err)
		}
		columns = append(columns, column)
	}
	return columns
}

// buildTable creates a given destination table with the necessary gpkg_ information
func buildTable(h *gpkg.Handle, t table) error {
	query := t.createSQL()
	_, err := h.Exec(query)
	if err != nil {
		log.Println("err:", err)
		return err
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
		log.Println("err:", err)
		return err
	}
	return nil
}

// initTargetGeopackage creates the destination Geopackage
func initTargetGeopackage(h *gpkg.Handle, tables []table) error {
	for _, table := range tables {
		err := h.UpdateSRS(table.srs)
		if err != nil {
			return err
		}

		err = buildTable(h, table)
		if err != nil {
			return err
		}
	}
	return nil
}

// readFeatures reads the features from the given Geopackage table
// and decodes the WKB geometry to a geom.Polygon
func readFeatures(h *gpkg.Handle, preSieve chan feature, t table) {
	rows, err := h.Query(t.selectSQL())
	defer rows.Close()
	if err != nil {
		log.Printf("err during closing rows: %v", err)
	}

	cols, err := rows.Columns()
	if err != nil {
		log.Println("err:", err)
	}

	for rows.Next() {
		vals := make([]interface{}, len(cols))
		valPtrs := make([]interface{}, len(cols))
		for i := 0; i < len(cols); i++ {
			valPtrs[i] = &vals[i]
		}

		if err = rows.Scan(valPtrs...); err != nil {
			log.Printf("err reading row values: %v", err)
			return
		}
		var f feature
		var c []interface{}

		for i, colName := range cols {
			if vals[i] == nil {
				continue
			}
			switch colName {
			case t.gcolumn:
				wkbgeom, err := gpkg.DecodeGeometry(vals[i].([]byte))
				if err != nil {
					log.Fatal(err)
				}
				f.geometry = wkbgeom.Geometry
			default:
				// Grab any non-nil, non-id, non-bounding box, & non-geometry column as a tag
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
				default:
					log.Printf("unexpected type for sqlite column data: %v: %T", cols[i], v)
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
}

// sieveFeatures sieves/filters the geometry against the given resolution
// the two steps that are done are:
// 1. filter features with a area smaller then the (resolution*resolution)
// 2. removes interior rings with a area smaller then the (resolution*resolution)
func sieveFeatures(preSieve chan feature, postSieve chan feature, resolution float64) {
	for {
		feature, hasMore := <-preSieve
		if !hasMore {
			break
		} else {
			switch gpkg.TypeForGeometry(feature.geometry) {
			case gpkg.Polygon:
				var p geom.Polygon
				p = feature.geometry.(geom.Polygon)
				if p := polygonSieve(p, resolution); p != nil {
					feature.geometry = p
					postSieve <- feature
				}
			case gpkg.MultiPolygon:
				var mp geom.MultiPolygon
				mp = feature.geometry.(geom.MultiPolygon)
				if mp := multiPolygonSieve(mp, resolution); mp != nil {
					feature.geometry = mp
					postSieve <- feature
				}
			default:
				postSieve <- feature
			}
		}
	}
	close(postSieve)
}

// multiPolygonSieve will split it self into the seperated polygons that will be sieved before building a new MULTIPOLYGON
func multiPolygonSieve(mp geom.MultiPolygon, resolution float64) geom.MultiPolygon {
	var sievedMultiPolygon geom.MultiPolygon
	for _, p := range mp {
		if sievedPolygon := polygonSieve(p, resolution); sievedPolygon != nil {
			sievedMultiPolygon = append(sievedMultiPolygon)
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

// writeFeatures writes the features processed by the sieveFeatures to the geopackages
func writeFeatures(postSieve chan feature, kill chan bool, h *gpkg.Handle, t table) {
	var ext *geom.Extent
	stmt, err := h.Prepare(t.insertSQL())
	if err != nil {
		log.Println("err:", err)
		return
	}

	for {
		feature, hasMore := <-postSieve
		if !hasMore {
			break
		} else {
			sb, err := gpkg.NewBinary(int32(t.srs.ID), feature.geometry)
			if err != nil {
				log.Fatalln("err:", err)
			}

			data := feature.columns
			data = append(data, sb)

			_, err = stmt.Exec(data...)
			if err != nil {
				log.Fatalln("stmt err:", err)
			}
		}

		if ext == nil {
			ext, err = geom.NewExtentFromGeometry(feature.geometry)
			if err != nil {
				ext = nil
				log.Println("err:", err)
				continue
			}
		} else {
			ext.AddGeometry(feature.geometry)
		}
	}
	h.UpdateGeometryExtent(t.name, ext)
	kill <- true
}

func main() {
	log.Println("start")
	sourceGeopackage := flag.String("s", "empty", "source geopackage")
	targetGeopackage := flag.String("t", "empty", "target geopackage")
	resolution := flag.Float64("r", 0.0, "resolution for sieving")
	flag.Parse()

	srcHandle, err := gpkg.Open(*sourceGeopackage)
	if err != nil {
		log.Println("err:", err)
		return
	}
	defer srcHandle.Close()

	trgHandle, err := gpkg.Open(*targetGeopackage)
	if err != nil {
		log.Println("Open err:", err)
		return
	}
	defer trgHandle.Close()

	tables := getSourceTableInfo(srcHandle)

	err = initTargetGeopackage(trgHandle, tables)
	if err != nil {
		log.Println("err:", err)
		return
	}

	// Process the tables sequential
	for _, table := range tables {
		log.Println(fmt.Sprintf(`processing: %s`, table.name))
		preSieve := make(chan feature)
		postSieve := make(chan feature)
		kill := make(chan bool)

		go writeFeatures(postSieve, kill, trgHandle, table)
		go sieveFeatures(preSieve, postSieve, *resolution)
		go readFeatures(srcHandle, preSieve, table)

		for {
			if <-kill {
				break
			}
		}
		close(kill)
		log.Println(fmt.Sprintf(`finished: %s`, table.name))
	}

	log.Println("stop")
}

// calculate the area of a polygon
func area(geom [][][2]float64) float64 {
	interior := .0
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
	p0 := pts[len(pts)-1]
	for _, p1 := range pts {
		sum += p0[1]*p1[0] - p0[0]*p1[1]
		p0 = p1
	}
	return math.Abs(sum / 2)
}
