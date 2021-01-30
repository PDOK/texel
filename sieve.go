package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/gpkg"
)

// TODO -> make dynamic based on the columns available
type vlaklocaties struct {
	fid           int
	identificatie string
	geometry      geom.Polygon
}

func readFeatures(h *gpkg.Handle, preSieve chan vlaklocaties) {
	// TODO -> loop over available tables with (multi)polygons

	var rows *sql.Rows
	// TODO -> dynamic query based on given table
	query := `SELECT fid, identificatie, geom from vlaklocaties;`
	rows, err := h.Query(query)

	defer rows.Close()
	if err != nil {
		log.Printf("err during closing rows: %v - %v", query, err)
	}

	for rows.Next() {
		var f int
		var i string
		var g interface{}

		err := rows.Scan(&f, &i, &g)
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

func writeFeatures(postSieve chan vlaklocaties, kill chan bool, targetGeopackage string) {

	h, err := gpkg.Open(targetGeopackage)
	if err != nil {
		log.Println("Open err:", err)
		return
	}
	defer h.Close()

	rd := gpkg.SpatialReferenceSystem{
		Name:                   `epsg:28992`,
		ID:                     28992,
		Organization:           `epsg`,
		OrganizationCoordsysID: 28992,
		Definition: `PROJCS["Amersfoort / RD New", 
		GEOGCS["Amersfoort", 
		  DATUM["Amersfoort", 
			SPHEROID["Bessel 1841", 6377397.155, 299.1528128, AUTHORITY["EPSG","7004"]], 
			TOWGS84[565.2369, 50.0087, 465.658, -0.4068573303223975, -0.3507326765425626, 1.8703473836067956, 4.0812], 
			AUTHORITY["EPSG","6289"]], 
		  PRIMEM["Greenwich", 0.0, AUTHORITY["EPSG","8901"]], 
		  UNIT["degree", 0.017453292519943295], 
		  AXIS["Geodetic longitude", EAST], 
		  AXIS["Geodetic latitude", NORTH], 
		  AUTHORITY["EPSG","4289"]], 
		PROJECTION["Oblique_Stereographic", AUTHORITY["EPSG","9809"]], 
		PARAMETER["central_meridian", 5.387638888888891], 
		PARAMETER["latitude_of_origin", 52.15616055555556], 
		PARAMETER["scale_factor", 0.9999079], 
		PARAMETER["false_easting", 155000.0], 
		PARAMETER["false_northing", 463000.0], 
		UNIT["m", 1.0], 
		AXIS["Easting", EAST], 
		AXIS["Northing", NORTH], 
		AUTHORITY["EPSG","28992"]]`,
		Description: `epsg:28992`,
	}
	err = h.UpdateSRS(rd)
	if err != nil {
		log.Println("updatesrs err:", err)
	}

	c := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS vlaklocaties (fid INTEGER NOT NULL PRIMARY KEY, identificatie TEXT, geometry %v);`, gpkg.Polygon.String())
	_, err = h.Exec(c)
	if err != nil {
		log.Println("create err:", err)
		return
	}

	err = h.AddGeometryTable(gpkg.TableDescription{
		Name:          "vlaklocaties",
		ShortName:     "vlaklocaties",
		Description:   "vlaklocaties",
		GeometryField: "geometry",
		GeometryType:  gpkg.Polygon,
		SRS:           28992,
		Z:             gpkg.Prohibited,
		M:             gpkg.Prohibited,
	})
	if err != nil {
		log.Println("err:", err)
	}

	var ext *geom.Extent

	stmt, err := h.Prepare(`INSERT INTO vlaklocaties(fid, identificatie, geometry) VALUES(?,?,?)`)
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
	h.UpdateGeometryExtent("vlaklocaties", ext)

	log.Println("killing")
	kill <- true
}

func main() {
	log.Println("start")
	sourceGeopackage := flag.String("s", "empty", "source geopackage")
	targetGeopackage := flag.String("t", "empty", "target geopackage")
	resolution := flag.Float64("r", 0.0, "resolution for sieving")
	flag.Parse()

	h, err := gpkg.Open(*sourceGeopackage)
	if err != nil {
		log.Println("err:", err)
		return
	}
	defer h.Close()

	preSieve := make(chan vlaklocaties)
	postSieve := make(chan vlaklocaties)
	kill := make(chan bool)

	go writeFeatures(postSieve, kill, *targetGeopackage)
	go sieveFeatures(preSieve, postSieve, *resolution)
	go readFeatures(h, preSieve)

	for {
		if <-kill {
			break
		}
	}

	close(kill)
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
