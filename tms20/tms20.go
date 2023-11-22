// Package tms20 implements the OGC Tile Matrix Set standard (v2.0) as a slippy.Grid
// See https://www.ogc.org/standard/tms/
package tms20

import (
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/go-spatial/geom"

	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"

	"github.com/go-spatial/geom/slippy"
	"github.com/perimeterx/marshmallow"
)

const (
	CoordPrecision                 = 5
	StandardizedRenderingPixelSize = 0.00028
)

var (
	//go:embed tilematrixsets/*.json
	embeddedTileMatrixSetsJSONFS embed.FS
	embeddedTileMatrixSetsCache  = make(map[string]*TileMatrixSet)
	crsURIRegexURL               = regexp.MustCompile(`https?://.+/def/crs/(?P<authority>[^/]+)/(?P<version>[^/]*)/(?P<code>[^/]+)$`)
	crsURIRegexURN               = regexp.MustCompile(`^urn:ogc:def:crs:(?P<authority>[^:]+):(?P<version>[^:]*):(?P<code>[^:]+)$`)
	latLonOrderedAxesRegex       = regexp.MustCompile(`^(e,n|x,y|lon,lat|e\(x\),n\(y\))`)
	lonLatOrderedAxesRegex       = regexp.MustCompile(`^(n,e|y,x|lat|lon)`)
)

func LoadJSONTileMatrixSet(path string) (TileMatrixSet, error) {
	var tms TileMatrixSet
	tmsJSON, err := os.ReadFile(path)
	if err != nil {
		return tms, err
	}
	err = json.Unmarshal(tmsJSON, &tms)
	if err != nil {
		return tms, err
	}
	return tms, nil
}

func LoadEmbeddedTileMatrixSet(id string) (TileMatrixSet, error) {
	var tms TileMatrixSet
	cached, ok := embeddedTileMatrixSetsCache[id]
	if ok {
		return *cached, nil
	}
	tmsJSON, err := embeddedTileMatrixSetsJSONFS.ReadFile(path.Join("tilematrixsets", id+extJSON))
	if err != nil {
		return tms, err
	}
	err = json.Unmarshal(tmsJSON, &tms)
	if err != nil {
		return tms, err
	}
	embeddedTileMatrixSetsCache[id] = &tms
	return tms, nil
}

// TileMatrixSet is a definition of a tile matrix set following the Tile Matrix Set standard.
// For tileset metadata, such a description (in `TileMatrixSet` property) is only required for offline use,
// as an alternative to a link with a `http://www.opengis.net/def/rel/ogc/1.0/tiling-scheme` relation type.
type TileMatrixSet struct {
	// Tile matrix set identifier. Implementation of 'identifier'
	ID string `json:"id,omitempty"`
	// Title of this tile matrix set, normally used for display to a human
	Title string `json:"title,omitempty"`
	// Brief narrative description of this tile matrix set, normally available for display to a human
	Description string `json:"description,omitempty"`
	// Unordered list of one or more commonly used or formalized word(s) or phrase(s) used to describe this tile matrix set
	Keywords []string `json:"keywords,omitempty"`
	// Reference to an official source for this TileMatrixSet
	URI string `validate:"omitempty,uri" json:"uri,omitempty"`
	// (This is informative, the CRS is authoritative)
	OrderedAxes []string `validate:"omitnil,min=1" json:"orderedAxes"`
	// Coordinate Reference System (CRS)
	CRS CRS `validate:"required" json:"-"`
	// Reference to a well-known scale set
	WellKnownScaleSet string `validate:"omitempty,uri" json:"wellKnownScaleSet,omitempty"`
	// Minimum bounding rectangle surrounding the tile matrix set, in the supported CRS
	BoundingBox *TwoDBoundingBox `json:"boundingBox,omitempty"`
	// Describes scale levels and its tile matrices
	TileMatrices map[int]TileMatrix `validate:"required,min=1" json:"-"`
}

func (tms *TileMatrixSet) MarshalJSON() ([]byte, error) {
	var tileMatrices []*TileMatrix
	for i := range tms.TileMatrices {
		tm := tms.TileMatrices[i]
		tileMatrices = append(tileMatrices, &(tm))
	}
	sort.Slice(tileMatrices, func(i, j int) bool {
		iID, _ := strconv.ParseInt(tileMatrices[i].ID, 10, 64)
		jID, _ := strconv.ParseInt(tileMatrices[j].ID, 10, 64)
		return iID < jID
	})
	return json.Marshal(struct {
		TileMatrixSet                     // not a pointer, because it would cause recursion to this function
		SpecialCRS          *CRS          `json:"crs"` // pointer, because crs' structs' MarshalJSON funcs are on pointer
		SpecialTileMatrices []*TileMatrix `json:"tileMatrices"`
	}{
		TileMatrixSet:       *tms,
		SpecialCRS:          &tms.CRS,
		SpecialTileMatrices: tileMatrices,
	})
}

func (tms *TileMatrixSet) UnmarshalJSON(data []byte) error {
	err := defaults.Set(tms)
	if err != nil {
		return err
	}

	specials, err := marshmallow.Unmarshal(data, tms, marshmallow.WithExcludeKnownFieldsFromMap(true))
	if err != nil {
		return err
	}

	// CRS
	rawCrs, ok := specials["crs"]
	if !ok {
		return fmt.Errorf(`missing key "crs"`)
	}
	tms.CRS, err = unmarshalCRS(rawCrs)
	if err != nil {
		return err
	}

	// TileMatrices
	rawTileMatrices, ok := specials["tileMatrices"]
	if !ok {
		return fmt.Errorf(`missing key "tileMatrices"`)
	}
	tms.TileMatrices, err = unmarshalTileMatrices(rawTileMatrices)
	if err != nil {
		return err
	}

	validate := validator.New(validator.WithRequiredStructEnabled())
	return validate.Struct(tms)
}

func unmarshalTileMatrices(rawTileMatrices interface{}) (map[int]TileMatrix, error) {
	rawTileMatricesList, ok := rawTileMatrices.([]interface{})
	if !ok {
		return nil, fmt.Errorf(`"tileMatrices" should be an array`)
	}
	tileMatrices := make(map[int]TileMatrix, len(rawTileMatricesList))
	for _, rawTileMatrix := range rawTileMatricesList {
		rawTileMatrixMap, ok := rawTileMatrix.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf(`"tileMatrices" should be objects`)
		}
		var tileMatrix TileMatrix
		err := tileMatrix.UnmarshalJSONFromMap(rawTileMatrixMap)
		if err != nil {
			return nil, err
		}
		tileMatrixID, err := strconv.ParseInt(tileMatrix.ID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("only integer-like ids are supported for tile matrices: %w", err)
		}
		tileMatrices[int(tileMatrixID)] = tileMatrix
	}
	return tileMatrices, nil
}

// unmarshalCRS tries 4 different CRS types (oneOf)
// TODO maybe there is already a library that can parse this, something like gdal/ogr
func unmarshalCRS(rawCrs interface{}) (CRS, error) {
	var rawCrsMap map[string]interface{}
	rawCrsString, asString := rawCrs.(string)
	if asString {
		rawCrsMap = map[string]interface{}{"uri": rawCrsString}
	} else {
		var ok bool
		rawCrsMap, ok = rawCrs.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf(`wrong type key "crs": %T`, rawCrs)
		}
	}
	var errors []error

	var uriCrs URICRS
	err := uriCrs.UnmarshalJSONFromMap(rawCrsMap)
	if err == nil {
		uriCrs.asString = asString
		return &uriCrs, nil
	}
	errors = append(errors, err)

	var wktCrs WKTCRS
	err = wktCrs.UnmarshalJSONFromMap(rawCrsMap)
	if err == nil {
		return &wktCrs, nil
	}
	errors = append(errors, err)

	var referenceSystemCrs ReferenceSystemCRS
	err = referenceSystemCrs.UnmarshalJSONFromMap(rawCrsMap)
	if err == nil {
		return &referenceSystemCrs, nil
	}
	errors = append(errors, err)

	return nil, fmt.Errorf(`could not unmarshal crs into any CRS type. errors: %v`, errors)
}

type CRS interface {
	Description() string
	Authority() string
	Version() string
	Code() string
}

type URICRS struct {
	description string
	// Reference to one coordinate reference system (CRS)
	uri       string `validate:"required,uri"`
	authority string `validate:"required"`
	version   string
	code      string `validate:"required"`
	// Whether it should be marshalled as just a string
	asString bool
}

func (crs *URICRS) MarshalJSON() ([]byte, error) {
	if crs.asString {
		return json.Marshal(crs.uri)
	}
	return json.Marshal(struct {
		Description string `json:"description,omitempty"`
		URI         string `json:"uri"`
	}{
		Description: crs.description,
		URI:         crs.uri,
	})
}

func (crs *URICRS) UnmarshalJSON(data []byte) error {
	return UnmarshalJSONMapUsingUnmarshalJSONFromMap(crs, data)
}

func (crs *URICRS) UnmarshalJSONFromMap(data interface{}) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf(`data is not a map but a %T`, data)
	}

	rawDescription, ok := dataMap["description"]
	if ok {
		crs.description, ok = rawDescription.(string)
		if !ok {
			return fmt.Errorf(`description property is not a string but a %T`, rawDescription)
		}
	}

	rawURI, ok := dataMap["uri"]
	if !ok {
		return fmt.Errorf(`uri property not found`)
	}
	crs.uri, ok = rawURI.(string)
	if !ok {
		return fmt.Errorf(`uri property is not a string but a %T`, rawURI)
	}

	uriParts := crsURIRegexURL.FindStringSubmatch(crs.uri)
	if uriParts == nil {
		uriParts = crsURIRegexURN.FindStringSubmatch(crs.uri)
	}
	if uriParts == nil {
		return fmt.Errorf(`could not parse crs uri "%v"`, crs.uri)
	}
	crs.authority = uriParts[1]
	crs.version = uriParts[2]
	crs.code = uriParts[3]

	validate := validator.New(validator.WithRequiredStructEnabled())
	return validate.Struct(crs)
}

func (crs *URICRS) Description() string {
	return crs.description
}

func (crs *URICRS) Authority() string {
	return crs.authority
}

func (crs *URICRS) Version() string {
	return crs.version
}

func (crs *URICRS) Code() string {
	return crs.code
}

type WKTCRS struct {
	description string
	// An object defining the CRS using the JSON encoding for Well-known text representation of coordinate reference systems 2.0
	wkt         ProjJSON
	originalWKT map[string]interface{}
}

// TODO expand the ProjJSON type
type ProjJSON struct {
	ID ProjJSONID `validate:"required" json:"id"`
}

type ProjJSONID struct {
	AuthorityName string `validate:"required" json:"authority"`
	AuthorityCode string `validate:"required" json:"code"` // TODO can be int cq number
}

func (crs *WKTCRS) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Description string                 `json:"description,omitempty"`
		WKT         map[string]interface{} `json:"wkt"`
	}{
		Description: crs.description,
		WKT:         crs.originalWKT,
	})
}

func (crs *WKTCRS) UnmarshalJSON(data []byte) error {
	return UnmarshalJSONMapUsingUnmarshalJSONFromMap(crs, data)
}

func (crs *WKTCRS) UnmarshalJSONFromMap(data interface{}) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf(`data is not a map but a %T`, data)
	}

	rawDescription, ok := dataMap["description"]
	if ok {
		crs.description, ok = rawDescription.(string)
		if !ok {
			return fmt.Errorf(`description property is not a string but a %T`, rawDescription)
		}
	}

	rawWKT, ok := dataMap["wkt"]
	if !ok {
		return fmt.Errorf(`wkt property not found`)
	}
	crs.originalWKT, ok = rawWKT.(map[string]interface{})
	if !ok {
		return fmt.Errorf(`wkt property is not an object but a %T`, rawWKT)
	}

	var wkt ProjJSON
	_, err := marshmallow.UnmarshalFromJSONMap(crs.originalWKT, &wkt)
	if err != nil {
		return fmt.Errorf(`could not parse wkt as ProjJSON "%v"`, crs.originalWKT)
	}
	crs.wkt = wkt

	validate := validator.New(validator.WithRequiredStructEnabled())
	return validate.Struct(crs)
}

func (crs *WKTCRS) Description() string {
	return crs.description
}

func (crs *WKTCRS) Authority() string {
	return crs.wkt.ID.AuthorityName
}

func (crs *WKTCRS) Version() string {
	return ""
}

func (crs *WKTCRS) Code() string {
	return crs.wkt.ID.AuthorityCode
}

type ReferenceSystemCRS struct {
	description string
	// A reference system data structure as defined in the MD_ReferenceSystem of the ISO 19115
	referenceSystem map[string]interface{} `validate:"required"`
}

func (crs *ReferenceSystemCRS) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Description     string                 `json:"description,omitempty"`
		ReferenceSystem map[string]interface{} `json:"wkt"`
	}{
		Description:     crs.description,
		ReferenceSystem: crs.referenceSystem,
	})
}

func (crs *ReferenceSystemCRS) UnmarshalJSON(data []byte) error {
	return UnmarshalJSONMapUsingUnmarshalJSONFromMap(crs, data)
}

func (crs *ReferenceSystemCRS) UnmarshalJSONFromMap(data interface{}) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf(`data is not a map but a %T`, data)
	}

	rawDescription, ok := dataMap["description"]
	if ok {
		crs.description, ok = rawDescription.(string)
		if !ok {
			return fmt.Errorf(`description property is not a string but a %T`, rawDescription)
		}
	}

	rawReferenceSystem, ok := dataMap["referenceSystem"]
	if !ok {
		return fmt.Errorf(`referenceSystem property not found`)
	}
	crs.referenceSystem, ok = rawReferenceSystem.(map[string]interface{})
	if !ok {
		return fmt.Errorf(`referenceSystem property is not an object but a %T`, rawReferenceSystem)
	}

	validate := validator.New(validator.WithRequiredStructEnabled())
	return validate.Struct(crs)
}

func (crs *ReferenceSystemCRS) Description() string {
	return crs.description
}

func (crs *ReferenceSystemCRS) Authority() string {
	panic("not implemented") // TODO implement ReferenceSystemCRS.Authority()
}

func (crs *ReferenceSystemCRS) Version() string {
	panic("not implemented") // TODO implement ReferenceSystemCRS.Version()
}

func (crs *ReferenceSystemCRS) Code() string {
	panic("not implemented") // TODO implement ReferenceSystemCRS.Code()
}

// Minimum bounding rectangle surrounding a 2D resource in the CRS indicated elsewhere
type TwoDBoundingBox struct {
	LowerLeft   *TwoDPoint `validate:"required" json:"lowerLeft"`
	UpperRight  *TwoDPoint `validate:"required" json:"upperRight"`
	CRS         CRS        `json:"-"`
	OrderedAxes []string   `validate:"omitempty,len=2" json:"orderedAxes,omitempty"`
}

func (bb *TwoDBoundingBox) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		TwoDBoundingBox      // not a pointer, because it would cause recursion to this function
		SpecialCRS      *CRS `json:"crs"` // pointer, because crs' structs' MarshalJSON funcs are on pointer
	}{
		TwoDBoundingBox: *bb,
		SpecialCRS:      &bb.CRS,
	})
}

func (bb *TwoDBoundingBox) UnmarshalJSON(data []byte) error {
	err := defaults.Set(bb)
	if err != nil {
		return err
	}

	specials, err := marshmallow.Unmarshal(data, bb, marshmallow.WithExcludeKnownFieldsFromMap(true))
	if err != nil {
		return err
	}

	// CRS
	rawCrs, ok := specials["crs"]
	if !ok {
		return fmt.Errorf(`missing key "crs"`)
	}
	bb.CRS, err = unmarshalCRS(rawCrs)
	if err != nil {
		return err
	}

	validate := validator.New(validator.WithRequiredStructEnabled())
	return validate.Struct(bb)
}

// A 2D Point in the CRS indicated elsewhere
type TwoDPoint [2]float64

func IsLatLon(crs CRS) (bool, error) {
	authority := crs.Authority()
	version := crs.Version()
	code := crs.Code()
	// :urn:ogc:def:crs:OGC:1.3:CRS84
	if authority == "OGC" && version == "1.3" && code == "CRS84" {
		return false, nil
	}
	if strings.ToLower(authority) != "epsg" {
		return false, fmt.Errorf(`could not determine axis order for unknown crs authority "%v"`, authority)
	}
	iCode, err := strconv.ParseUint(code, 10, 64)
	if err != nil {
		return false, fmt.Errorf(`could not parse uri authority code "%w"`, err)
	}
	isLatLon, known := epsgAxesAreLatLon[uint(iCode)]
	if !known {
		return false, fmt.Errorf(`unknown axis order for epsg:%d`, iCode)
	}
	return isLatLon, nil
}

// ToXYPoint ensures that the coordinates in a point are in XY order
func ToXYPoint(tms *TileMatrixSet, point [2]float64) (geom.Point, error) {
	isLatLon, err := IsLatLon(tms.CRS)
	if err != nil {
		// Fall back to (informative) OrderedAxes property
		isLatLon, err = axisOrderIsLatLon(tms.OrderedAxes)
	}
	switch {
	case err != nil:
		return [2]float64{}, err
	case isLatLon:
		return [2]float64{point[1], point[0]}, nil
	default:
		return [2]float64{point[0], point[1]}, nil
	}
}

func axisOrderIsLatLon(orderedAxes []string) (bool, error) {
	if len(orderedAxes) < 2 {
		return false, fmt.Errorf(`could not determine if (empty or single) ordered axes are in lat/lon order`)
	}
	orderedAxesStr := []byte(strings.ToLower(fmt.Sprintf(`%s,%s`, orderedAxes[0], orderedAxes[1])))
	if latLonOrderedAxesRegex.Match(orderedAxesStr) {
		return true, nil
	} else if lonLatOrderedAxesRegex.Match(orderedAxesStr) {
		return false, nil
	}
	return false, fmt.Errorf(`could not determine if ordered axes are in lat/lon order`)
}

// A tile matrix, usually corresponding to a particular zoom level of a TileMatrixSet.
type TileMatrix struct {
	// Identifier selecting one of the scales defined in the TileMatrixSet and representing the scaleDenominator the tile.
	// Implementation of 'identifier'
	ID string `validate:"required" json:"id"`
	// Title of this tile matrix, normally used for display to a human
	Title string `json:"title,omitempty"`
	// Brief narrative description of this tile matrix set, normally available for display to a human
	Description string `json:"description,omitempty"`
	// Unordered list of one or more commonly used or formalized word(s) or phrase(s) used to describe this dataset
	Keywords []string `json:"keywords,omitempty"`
	// Scale denominator of this tile matrix
	ScaleDenominator float64 `validate:"required,gt=0" json:"scaleDenominator"`
	// Cell size of this tile matrix
	CellSize float64 `validate:"required,gt=0" json:"cellSize"`
	// The corner of the tile matrix (_topLeft_ or _bottomLeft_) used as the origin for numbering tile rows and columns.
	// This corner is also a corner of the (0, 0) tile.
	CornerOfOrigin CornerOfOrigin `validate:"omitempty,oneof=topLeft bottomLeft" json:"cornerOfOrigin,omitempty"`
	// Precise position in CRS coordinates of the corner of origin (e.g. the top-left corner) for this tile matrix. This position is also a corner of the (0, 0) tile. In previous version, this was 'topLeftCorner' and 'cornerOfOrigin' did not exist.
	PointOfOrigin *TwoDPoint `validate:"required" json:"pointOfOrigin"`
	// Width of each tile of this tile matrix in pixels
	TileWidth uint `validate:"required,min=1" json:"tileWidth"`
	// Height of each tile of this tile matrix in pixels
	TileHeight uint `validate:"required,min=1" json:"tileHeight"`
	// Width of the matrix (number of tiles in width)
	MatrixWidth uint `validate:"required,min=1" json:"matrixWidth"`
	// Height of the matrix (number of tiles in height)
	MatrixHeight uint `validate:"required,min=1" json:"matrixHeight"`
	// Describes the rows that have variable matrix width
	VariableMatrixWidths []VariableMatrixWidth `json:"variableMatrixWidths,omitempty"`
}

func (tm *TileMatrix) UnmarshalJSON(data []byte) error {
	return UnmarshalJSONMapUsingUnmarshalJSONFromMap(tm, data)
}

func (tm *TileMatrix) UnmarshalJSONFromMap(data interface{}) error {
	err := defaults.Set(tm)
	if err != nil {
		return err
	}

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf(`data is not a map but a %T`, data)
	}

	_, err = marshmallow.UnmarshalFromJSONMap(dataMap, tm, marshmallow.WithExcludeKnownFieldsFromMap(true))
	if err != nil {
		return err
	}

	validate := validator.New(validator.WithRequiredStructEnabled())
	return validate.Struct(tm)
}

type CornerOfOrigin string

const (
	TopLeft    CornerOfOrigin = "topLeft"
	BottomLeft CornerOfOrigin = "bottomLeft"
	extJSON                   = ".json"
)

func (c *CornerOfOrigin) UnmarshalJSONFromMap(data interface{}) error {
	dataString, ok := data.(string)
	if !ok {
		return fmt.Errorf(`CornerOfOrigin data is not a string but a %T`, data)
	}
	switch dataString {
	case "":
		fallthrough
	case string(TopLeft):
		*c = TopLeft
	case string(BottomLeft):
		*c = BottomLeft
	default:
		return fmt.Errorf(`unknown CornerOfOrigin: %v`, data)
	}
	return nil
}

// Variable Matrix Width data structure
type VariableMatrixWidth struct {
	// Number of tiles in width that coalesce in a single tile for these rows
	Coalesce uint `validate:"required,min=2" json:"coalesce"`
	// First tile row where the coalescence factor applies for this tilematrix
	MinTileRow uint `validate:"required,min=0" json:"minTileRow"`
	// Last tile row where the coalescence factor applies for this tilematrix
	MaxTileRow uint `validate:"required,min=0" json:"maxTileRow"`
}

func (tms *TileMatrixSet) SRID() uint {
	code, err := strconv.ParseUint(tms.CRS.Code(), 10, 64)
	if err != nil {
		panic(fmt.Errorf(`could not parse uri authority code "%w"`, err))
	}
	return uint(code)
}

func (tms *TileMatrixSet) Size(zoom uint) (*slippy.Tile, bool) {
	tm, ok := tms.TileMatrices[int(zoom)]
	if !ok {
		return nil, false
	}
	return slippy.NewTile(zoom, tm.MatrixWidth, tm.MatrixHeight), true
}

func (tms *TileMatrixSet) FromNative(zoom uint, pt geom.Point) (*slippy.Tile, bool) {
	tm, ok := tms.TileMatrices[int(zoom)]
	if !ok {
		return nil, false
	}

	if tm.VariableMatrixWidths != nil {
		panic("variable matrix widths not supported") // TODO support VariableMatrixWidths
	}

	pointOfOriginXY, err := ToXYPoint(tms, *tm.PointOfOrigin)
	if err != nil {
		panic(fmt.Errorf(`could not get pointOfOrigin coordinates: %w`, err))
	}

	// TODO use big decimals to prevent floating point rounding errors
	tileSizeX := float64(tm.TileWidth) * tm.CellSize
	minX := pointOfOriginXY[0]
	x := (pt.X() - minX) / tileSizeX
	if x < 0 {
		return nil, false
	}
	ux := uint(x)
	if ux >= tm.MatrixWidth {
		return nil, false
	}

	tileSizeY := float64(tm.TileHeight) * tm.CellSize
	var y float64
	switch tm.CornerOfOrigin {
	default:
		fallthrough
	case TopLeft:
		maxY := pointOfOriginXY[1]
		y = (maxY - pt.Y()) / tileSizeY
	case BottomLeft:
		minY := pointOfOriginXY[1]
		y = (pt.Y() - minY) / tileSizeY
	}
	if y < 0 {
		return nil, false
	}
	uy := uint(y)
	if uy >= tm.MatrixHeight {
		return nil, false
	}

	return slippy.NewTile(zoom, ux, uy), true
}

func (tms *TileMatrixSet) ToNative(tile *slippy.Tile) (geom.Point, bool) {
	topLeftPt := geom.Point{}
	tm, ok := tms.TileMatrices[int(tile.Z)]
	if !ok {
		return topLeftPt, false
	}
	if tile.X > tm.MatrixWidth || tile.Y > tm.MatrixHeight {
		// >, not >= because "should be able to take tiles with x and y values 1 higher than the max"
		return topLeftPt, false
	}

	pointOfOriginXY, err := ToXYPoint(tms, *tm.PointOfOrigin)
	if err != nil {
		panic(fmt.Errorf(`could not get pointOfOrigin coordinates: %w`, err))
	}

	tileSizeX := float64(tm.TileWidth) * tm.CellSize
	minX := pointOfOriginXY[0]
	topLeftPt[0] = roundFloat(minX+float64(tile.X)*tileSizeX, CoordPrecision)

	tileSizeY := float64(tm.TileHeight) * tm.CellSize
	switch tm.CornerOfOrigin {
	default:
		fallthrough
	case TopLeft:
		maxY := pointOfOriginXY[1]
		topLeftPt[1] = roundFloat(maxY-float64(tile.Y)*tileSizeY, CoordPrecision)
	case BottomLeft:
		minY := pointOfOriginXY[1]
		topLeftPt[1] = roundFloat(minY+float64(tile.Y+1)*tileSizeY, CoordPrecision)
	}

	return topLeftPt, true
}

func UnmarshalJSONMapUsingUnmarshalJSONFromMap(target marshmallow.UnmarshalerFromJSONMap, data []byte) error {
	var dataMap map[string]interface{}
	err := json.Unmarshal(data, &dataMap)
	if err != nil {
		return err
	}
	return target.UnmarshalJSONFromMap(dataMap)
}

func roundFloat(f float64, p uint) float64 {
	r := math.Pow(10, float64(p))
	return math.Round(f*r) / r
}
