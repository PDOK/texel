package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/gpkg"
)

// TODO -> make dynamic based on the columns available
type vlaklocaties struct {
	fid           int
	identificatie string
	geometry      geom.Polygon
}

type feature struct {
	//columns  map[string]interface{}
	columns  []interface{}
	geometry geom.Polygon
}

type sourceTableInfo struct {
	name       string
	columns    []column
	geomcolumn string
	srs        int
}

type column struct {
	cid       int
	name      string
	ctype     string
	notnull   int
	dfltValue *int
	pk        int
}

type gpkgGeometryColumns struct {
	tableName        string
	columnName       string
	geometryTypeName string
	srsID            int
}

func filterGpkgGeometryColumns(h *gpkg.Handle, geometrytype string) []gpkgGeometryColumns {
	var matches []gpkgGeometryColumns

	query := `SELECT table_name, column_name, geometry_type_name, srs_id FROM gpkg_geometry_columns WHERE upper(geometry_type_name) = upper('%v');`
	rows, err := h.Query(fmt.Sprintf(query, geometrytype))
	defer rows.Close()
	if err != nil {
		log.Printf("err during closing rows: %v - %v", query, err)
	}

	for rows.Next() {
		var ggc gpkgGeometryColumns
		err := rows.Scan(&ggc.tableName, &ggc.columnName, &ggc.geometryTypeName, &ggc.srsID)
		if err != nil {
			log.Fatal(err)
		}
		matches = append(matches, ggc)
	}

	return matches
}

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

func buildCreateTableQuery(tablename string, columns []column) string {
	create := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %v`, tablename)
	var columnparts []string
	for _, column := range columns {
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

func buildTable(h *gpkg.Handle, table gpkgGeometryColumns, columns []column) error {

	query := buildCreateTableQuery(table.tableName, columns)
	_, err := h.Exec(query)
	if err != nil {
		log.Println("err:", err)
		return err
	}

	err = h.AddGeometryTable(gpkg.TableDescription{
		Name:          table.tableName,
		ShortName:     table.tableName,
		Description:   table.tableName,
		GeometryField: table.columnName,
		GeometryType:  gpkg.Polygon,
		SRS:           int32(table.srsID),
		Z:             gpkg.Prohibited,
		M:             gpkg.Prohibited,
	})
	if err != nil {
		log.Println("err:", err)
		return err
	}
	return nil
}

func initTargetGeopackage(src *gpkg.Handle, trgt *gpkg.Handle) ([]sourceTableInfo, error) {
	tables := filterGpkgGeometryColumns(src, `POLYGON`)
	var tablesOutput []sourceTableInfo
	for _, table := range tables {
		srs := getSpatialReferenceSystem(src, table.srsID)
		err := trgt.UpdateSRS(srs)
		if err != nil {
			return tablesOutput, err
		}

		columns := getTableColumns(src, table.tableName)
		err = buildTable(trgt, table, columns)
		if err != nil {
			return tablesOutput, err
		}
		tablesOutput = append(tablesOutput, sourceTableInfo{name: table.tableName, geomcolumn: table.columnName, columns: columns, srs: table.srsID})
	}
	return tablesOutput, nil
}

func readFeatures(h *gpkg.Handle, preSieve chan feature, table sourceTableInfo) {
	// TODO -> loop over available tables with (multi)polygons

	var cnames []string
	for _, column := range table.columns {
		cnames = append(cnames, column.name)
	}

	var rows *sql.Rows
	// TODO -> dynamic query based on given table
	query := `SELECT ` + strings.Join(cnames, `, `) + ` FROM ` + table.name + `;`
	log.Println(query)
	rows, err := h.Query(query)
	defer rows.Close()
	if err != nil {
		log.Printf("err during closing rows: %v - %v", query, err)
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
			case table.geomcolumn:
				wkbgeom, err := gpkg.DecodeGeometry(vals[i].([]byte))
				if err != nil {
					log.Fatal(err)
				}
				var p geom.Polygon
				p = wkbgeom.Geometry.(geom.Polygon)
				f.geometry = p
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
	log.Println("done with reading")
	close(preSieve)
}

func sieveFeatures(preSieve chan feature, postSieve chan feature, resolution float64) {
	minArea := resolution * resolution
	for {
		feature, hasMore := <-preSieve
		if !hasMore {
			break
		} else {
			if area(feature.geometry) > minArea {
				if len(feature.geometry) > 1 {
					var newPolygon geom.Polygon
					newPolygon = append(newPolygon, feature.geometry[0])
					for _, interior := range feature.geometry[1:] {
						if shoelace(interior) > minArea {
							newPolygon = append(newPolygon, interior)
						}
					}
					feature.geometry = newPolygon
					postSieve <- feature
				} else {
					postSieve <- feature
				}
			}
		}
	}
	log.Println("done with sieving")
	close(postSieve)
}

func writeFeatures(postSieve chan feature, kill chan bool, h *gpkg.Handle, table sourceTableInfo) {
	var ext *geom.Extent

	var columns []string
	for _, column := range table.columns {
		if column.ctype != `POLYGON` {
			columns = append(columns, column.name)
		}
	}
	columns = append(columns, table.geomcolumn)

	log.Println(strings.Join(columns, `,`))
	stmt, err := h.Prepare(`INSERT INTO ` + table.name + `(` + strings.Join(columns, `,`) + `) VALUES(?,?,?)`)
	if err != nil {
		log.Println("err:", err)
		return
	}

	for {
		feature, hasMore := <-postSieve
		if !hasMore {
			break
		} else {
			sb, err := gpkg.NewBinary(int32(table.srs), feature.geometry)
			if err != nil {
				log.Println("err:", err)
				continue
			}

			data := feature.columns
			data = append(data, sb)

			_, err = stmt.Exec(data...)
			if err != nil {
				log.Fatalln("stmt err:", err)
				//continue
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
	h.UpdateGeometryExtent(table.name, ext)

	log.Println("killing")
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

	tables, err := initTargetGeopackage(srcHandle, trgHandle)
	if err != nil {
		log.Println("err:", err)
		return
	}

	// Proces the tables sequential
	for _, table := range tables {
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
	}

	log.Println("done")
}

func area(geom [][][2]float64) float64 {
	interior := .0
	if len(geom) > 1 {
		for _, i := range geom[1:] {
			interior = interior + shoelace(i)
		}
	}
	return (shoelace(geom[0]) * -1) - interior
}

func shoelace(pts [][2]float64) float64 {
	sum := 0.
	p0 := pts[len(pts)-1]
	for _, p1 := range pts {
		sum += p0[1]*p1[0] - p0[0]*p1[1]
		p0 = p1
	}
	return sum / 2
}
