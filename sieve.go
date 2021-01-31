package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"

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
	columns  map[string]interface{}
	geometry geom.Polygon
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

type column struct {
	cid       int
	name      string
	ctype     string
	notnull   int
	dfltValue *int
	pk        int
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
	log.Println(query)
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

func initTargetGeopackage(src *gpkg.Handle, trgt *gpkg.Handle) ([]string, error) {
	tables := filterGpkgGeometryColumns(src, `POLYGON`)
	var tablesOutput []string
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
		tablesOutput = append(tablesOutput, table.tableName)
	}
	return tablesOutput, nil
}

func readFeatures(h *gpkg.Handle, preSieve chan vlaklocaties, table string) {
	// TODO -> loop over available tables with (multi)polygons

	columns := getTableColumns(h, table)
	var cnames []string
	for _, column := range columns {
		cnames = append(cnames, column.name)
	}

	var rows *sql.Rows
	// TODO -> dynamic query based on given table
	query := `SELECT ` + strings.Join(cnames, `, `) + ` FROM ` + table + `;`
	log.Println(query)
	rows, err := h.Query(query)
	defer rows.Close()
	if err != nil {
		log.Printf("err during closing rows: %v - %v", query, err)
	}

	for rows.Next() {
		// TODO -> fix this!!
		var f int
		var i string
		var g interface{}

		err := rows.Scan(&f, &g, &i)
		if err != nil {
			log.Fatal(err)
		}

		wkbgeom, err := gpkg.DecodeGeometry(g.([]byte))
		if err != nil {
			log.Fatal(err)
		}

		var p geom.Polygon
		p = wkbgeom.Geometry.(geom.Polygon)

		row := vlaklocaties{fid: f, identificatie: i, geometry: p}

		preSieve <- row

	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("done with reading")
	close(preSieve)
}

func sieveFeatures(preSieve chan vlaklocaties, postSieve chan vlaklocaties, resolution float64) {
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
					postSieve <- vlaklocaties{fid: feature.fid, identificatie: feature.identificatie, geometry: newPolygon}
				} else {
					postSieve <- feature
				}
			}
		}
	}
	log.Println("done with sieving")
	close(postSieve)
}

func writeFeatures(postSieve chan vlaklocaties, kill chan bool, h *gpkg.Handle, table string) {
	var ext *geom.Extent

	stmt, err := h.Prepare(`INSERT INTO ` + table + `(fid, identificatie, geom) VALUES(?,?,?)`)
	if err != nil {
		log.Println("err:", err)
		return
	}

	for {
		feature, hasMore := <-postSieve
		if !hasMore {
			break
		} else {
			sb, err := gpkg.NewBinary(28992, feature.geometry)
			if err != nil {
				log.Println("err:", err)
				continue
			}
			_, err = stmt.Exec(feature.fid, feature.identificatie, sb)
			if err != nil {
				log.Println("err:", err)
				continue
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
	h.UpdateGeometryExtent(table, ext)

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

	for _, table := range tables {
		preSieve := make(chan vlaklocaties)
		postSieve := make(chan vlaklocaties)
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
