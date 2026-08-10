package tile

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/mvt"
	vectorTile "github.com/go-spatial/geom/encoding/mvt/vector_tile"
	oldproto "github.com/golang/protobuf/proto" //nolint:staticcheck // needed: matches the reflection-based marshaling vector_tile.pb.go relies on
	"github.com/pdok/texel/intgeom"
	"github.com/pdok/texel/mapslicehelp"
)

const (
	precision  = 4096
	mvtVersion = 2
)

type EncodedGeometry struct {
	Encoding     []uint32
	GeometryType int32
	XTile        uint
	YTile        uint
}

// EncodedFeatureRow pairs a feature's identifier with its encoded, tile-relative geometry
// as read back from the "<table>_encoded" table.
type EncodedFeatureRow struct {
	FeatureID int64
	Geom      EncodedGeometry
}

type Tile struct {
	Extent      intgeom.Extent
	X           uint
	Y           uint
	IsContained bool
}

// DefaultEncoding is the pre-encoded geometry for a tile that is fully
// contained by a polygon, i.e. a square covering the tile's (buffered)
// extent. Since it only depends on the (constant) buffer, it can be
// computed once and reused for every contained tile.
type DefaultEncoding struct {
	Encoding []uint32
	GeomType vectorTile.Tile_GeomType
}

// NewDefaultEncoding precomputes the DefaultEncoding for the given buffer
// (in internal pixels). Compute once and reuse across calls to
// MvtEncodeGeometry, since the result only depends on the buffer.
func NewDefaultEncoding(buffer uint) (DefaultEncoding, error) {
	fbuffer := float64(buffer)
	defaultPolygon := geom.Polygon{{
		{-fbuffer, -fbuffer},
		{-fbuffer, precision + fbuffer},
		{precision + fbuffer, precision + fbuffer},
		{precision + fbuffer, -fbuffer},
	}}

	defaultExtent := geom.Extent{
		-fbuffer,
		-fbuffer,
		precision + fbuffer,
		precision + fbuffer,
	}
	preparedPolygon := mvt.PrepareGeo(defaultPolygon, &defaultExtent, precision)

	encoding, geomType, err := EncodeGeometry(preparedPolygon)
	return DefaultEncoding{Encoding: encoding, GeomType: geomType}, err
}

// Transform geometry to tile extent, then encode. We assume the geometry is
// snapped to the proposed grid, in which case makevalid operations should not
// be necessary. defaultEnc is the precomputed encoding (see NewDefaultEncoding)
// used for tiles fully contained by the polygon.
func MvtEncodeGeometry(t Tile, g geom.Geometry, defaultEnc DefaultEncoding) EncodedGeometry {
	var encgeom []uint32
	var geomtype vectorTile.Tile_GeomType
	var err error
	if t.IsContained {
		encgeom, geomtype = defaultEnc.Encoding, defaultEnc.GeomType
	} else {
		ext := t.Extent.ToGeomExtent()
		preparedGeo := mvt.PrepareGeo(g, &ext, float64(precision))

		// This should not be necessary.
		//	sg, err := convert.ToTegola(preparedGeo)
		//	tegolaGeo, err := validate.CleanGeometry(context.TODO(), sg, &ext)
		//	validatedGeo := convert.ToGeom(tegolaGeo)

		encgeom, geomtype, err = EncodeGeometry(preparedGeo)
	}
	if err != nil {
		panic(err)
	}

	return EncodedGeometry{
		Encoding:     encgeom,
		GeometryType: int32(geomtype),
		XTile:        t.X,
		YTile:        t.Y,
	}
}

// Serialze tile into protobuf format
func MarshalTile(t *vectorTile.Tile) ([]byte, error) {
	return oldproto.Marshal(t)
}

// Internal representation of attributes: read table[fid][columnName]=value.
type InternalAttributeTable map[int64]map[string]any

// Build indices from column names
func BuildKeyDictionary(columnNames []string) map[string]uint32 {
	index := make(map[string]uint32, len(columnNames))
	for i, name := range columnNames {
		if i > int(^uint32(0)) {
			panic("Number of keys does not fit in uint32")
		}
		index[name] = uint32(i) //nolint:gosec // G115
	}
	return index
}

// Build index table for values. This function guarantees that all indices from
// 0 to len(index)-1 all appear exactly once. Absent attributes are not recorded.
func BuildValueDictionary(attributesPerFeature InternalAttributeTable) map[any]uint32 {
	index := make(map[any]uint32)
	for _, attributes := range attributesPerFeature {
		for _, v := range attributes {
			if v == nil {
				continue
			}
			if _, ok := index[v]; ok {
				continue
			}
			l := len(index)
			if l > int(^uint32(0)) {
				panic("Number of values does not fit in uint32")
			}
			index[v] = uint32(l)
		}
	}
	return index
}

// Construct tags for a single feature. `keyIndex` and `valueIndex` are built
// by above functions by considering all features. `attributes` are the
// attributes of the current feature. Absent values are skipped. Panics if a
// key of value is not present.
func buildTags(attributes map[string]any, keyIndex map[string]uint32, valueIndex map[any]uint32) ([]uint32, error) {
	tags := make([]uint32, 0, 2*len(attributes))
	for key, val := range attributes {
		if val == nil {
			continue
		}

		kidx, ok := keyIndex[key]
		if !ok {
			return nil, fmt.Errorf("did not find key (%v) in key dictionary", key)
		}

		vidx, ok := valueIndex[val]
		if !ok {
			return nil, fmt.Errorf("did not find value (%v) in value dictionary", val)
		}

		tags = append(tags, kidx, vidx)
	}
	return tags, nil
}

// Construct feature according to protobuf format.
func BuildFeature(featureID int64, geom EncodedGeometry, attributes map[string]any, keyIndex map[string]uint32, valueIndex map[any]uint32) (*vectorTile.Tile_Feature, error) {
	tags, err := buildTags(attributes, keyIndex, valueIndex)
	if err != nil {
		return nil, fmt.Errorf("feature %d: %w", featureID, err)
	}

	//nolint:gosec // G115 feature ids are expected to be non-negative
	id := uint64(featureID)
	gtype := vectorTile.Tile_GeomType(geom.GeometryType)

	return &vectorTile.Tile_Feature{
		Id:       &id,
		Tags:     tags,
		Type:     &gtype,
		Geometry: geom.Encoding,
	}, nil
}

// Construct layer according to protobuf format
func BuildLayer(name string, features []*vectorTile.Tile_Feature, keyIndex map[string]uint32, valueIndex map[any]uint32) *vectorTile.Tile_Layer {
	keys := mapslicehelp.OrderedByIndex(keyIndex)

	rawValues := mapslicehelp.OrderedByIndex(valueIndex)
	values := make([]*vectorTile.Tile_Value, len(rawValues))
	for i, v := range rawValues {
		values[i] = vectorTileValue(v)
	}

	version := uint32(mvtVersion)
	extent := uint32(precision)
	layerName := name

	return &vectorTile.Tile_Layer{
		Version:  &version,
		Name:     &layerName,
		Features: features,
		Keys:     keys,
		Values:   values,
		Extent:   &extent,
	}
}

// Construct tile according to protobuf format
func BuildTile(layers ...*vectorTile.Tile_Layer) *vectorTile.Tile {
	return &vectorTile.Tile{Layers: layers}
}

// Main exported function that encodes a tile of a single layer. Assumes
// geometry and keyIndex have already been encoded. Returns marshalled byte
// string.
func BuildMVTTile(layerName string, keyIndex map[string]uint32, encFeatRows []EncodedFeatureRow, attributes InternalAttributeTable) ([]byte, error) {
	valueIndex := BuildValueDictionary(attributes)

	features := make([]*vectorTile.Tile_Feature, 0, len(encFeatRows))
	for _, encodedFeature := range encFeatRows {
		featureID := encodedFeature.FeatureID
		attrs := attributesForFeature(attributes, featureID)
		feature, err := BuildFeature(featureID, encodedFeature.Geom, attrs, keyIndex, valueIndex)
		if err != nil {
			return nil, err
		}
		features = append(features, feature)
	}

	layer := BuildLayer(layerName, features, keyIndex, valueIndex)

	data, err := MarshalTile(BuildTile(layer))
	if err != nil {
		return nil, fmt.Errorf("marshaling tile: %w", err)
	}
	return data, nil
}

// attributesForFeature extracts a single feature's attributes from the map construct.
func attributesForFeature(attributes InternalAttributeTable, featureID int64) map[string]any {
	if attrs, ok := attributes[featureID]; ok {
		return attrs
	}
	return map[string]any{}
}

// Convert value to Tile_Value. Copied from go-spatial/encoding/mvt/layer.go
func vectorTileValue(i any) *vectorTile.Tile_Value { //nolint:cyclop, funlen
	tv := new(vectorTile.Tile_Value)
	switch t := i.(type) {
	default:
		buff := new(bytes.Buffer)
		err := binary.Write(buff, binary.BigEndian, t)
		// We are going to ignore the value and return an empty TileValue
		if err == nil {
			tv.XXX_unrecognized = buff.Bytes()
		}

	case string:
		tv.StringValue = &t

	case fmt.Stringer:
		str := t.String()
		tv.StringValue = &str

	case bool:
		tv.BoolValue = &t

	case int8:
		intv := int64(t)
		tv.SintValue = &intv

	case int16:
		intv := int64(t)
		tv.SintValue = &intv

	case int32:
		intv := int64(t)
		tv.SintValue = &intv

	case int64:
		tv.IntValue = &t

	case uint8:
		intv := int64(t)
		tv.SintValue = &intv

	case uint16:
		intv := int64(t)
		tv.SintValue = &intv

	case uint32:
		intv := int64(t)
		tv.SintValue = &intv

	case uint64:
		tv.UintValue = &t

	case float32:
		tv.FloatValue = &t

	case float64:
		tv.DoubleValue = &t

	} // switch
	return tv
}
