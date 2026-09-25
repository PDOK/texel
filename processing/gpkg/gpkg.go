package gpkg

// This file defines general geopackage-related infrastructure shared by both
// the `snap` and `mvt` flows.

import (
	"fmt"
	"log"
	"strings"

	"github.com/go-spatial/geom/encoding/gpkg"
)

type column struct {
	cid       int
	name      string
	ctype     string
	notnull   int
	dfltValue *int
	pk        int
}

// Abstraction of geometry table in a gpkg
type Table struct {
	Name    string
	columns []column
	gcolumn string
	gtype   gpkg.GeometryType
	srs     gpkg.SpatialReferenceSystem
}

// Name of associated table of a geometry table.
func (t Table) EncodedName() string {
	return t.Name + "_encoded"
}

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

// Custom wrapper for gpkg.Handle. To be embedded in other structs
type geopackageHandle struct{ handle *gpkg.Handle }

func (gH *geopackageHandle) Init(file string) {
	gH.handle = openGeopackage(file)
}

func (gH *geopackageHandle) Close() {
	gH.handle.Close()
}

// Generate []Table slice for looping over.
func (gH *geopackageHandle) GetTableInfo() []Table {
	query := `SELECT table_name, column_name, geometry_type_name, srs_id FROM gpkg_geometry_columns;`
	rows, err := gH.handle.Query(query)
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
			log.Fatalf("error retrieving the source table information: %s", err)
		}

		t.columns = gH.getTableColumns(t.Name)
		t.gtype = geometryTypeFromString(gtype)
		t.srs = gH.getSpatialReferenceSystem(srsID)

		tables = append(tables, t)
	}
	defer rows.Close()
	return tables
}

func openGeopackage(file string) *gpkg.Handle {
	handle, err := gpkg.Open(file)
	if err != nil {
		log.Fatalf("error opening GeoPackage: %s", err)
	}
	return handle
}

// getSpatialReferenceSystem extracts this based on the given SRS id
func (gH *geopackageHandle) getSpatialReferenceSystem(id int) gpkg.SpatialReferenceSystem {
	var srs gpkg.SpatialReferenceSystem
	query := `SELECT srs_name, srs_id, organization, organization_coordsys_id, definition, description FROM gpkg_spatial_ref_sys WHERE srs_id = %v;`

	row := gH.handle.QueryRow(fmt.Sprintf(query, id))
	var description *string
	_ = row.Scan(&srs.Name, &srs.ID, &srs.Organization, &srs.OrganizationCoordsysID, &srs.Definition, &description)
	if description != nil {
		srs.Description = *description
	}

	return srs
}

// getTableColumns collects the column information of a given table
func (gH *geopackageHandle) getTableColumns(table string) []column {
	var columns []column
	query := `PRAGMA table_info('%v');`
	rows, err := gH.handle.Query(fmt.Sprintf(query, table))
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
