package tile

import (
	"fmt"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/mvt"
	oldproto "github.com/golang/protobuf/proto" //nolint:staticcheck // needed: matches the reflection-based marshaling vector_tile.pb.go relies on
	"github.com/pdok/texel/pointindex"
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

// Transform geometry to tile extent, then encode. We assume the geometry is
// snapped to the proposed grid, in which case makevalid operations should not
// be necessary.
func MvtEncodeGeometry(q pointindex.Quadrant, g geom.Geometry) EncodedGeometry {
	ext := q.Extent()
	preparedGeo := mvt.PrepareGeo(g, &ext, float64(precision))

	// This should not be necessary.
	//	sg, err := convert.ToTegola(preparedGeo)
	//	tegolaGeo, err := validate.CleanGeometry(context.TODO(), sg, &ext)
	//	validatedGeo := convert.ToGeom(tegolaGeo)

	encgeom, geomtype, err := EncodeGeometry(preparedGeo)
	if err != nil {
		panic(err)
	}

	xTile, yTile := q.Coords()

	return EncodedGeometry{
		Encoding:     encgeom,
		GeometryType: int32(geomtype),
		XTile:        xTile,
		YTile:        yTile,
	}
}

// Serialze tile into protobuf format
func MarshalTile(t *GSTile) ([]byte, error) {
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

// Convert index map to slice. Assumes all indices 0, 1, ..., len(index)-1 are
// used exactly once. Necessary since iteration over maps is in arbitrary order.
// This is a generic function since we need it with K = string and K = any.
func orderedByIndex[K comparable](index map[K]uint32) []K {
	ordered := make([]K, len(index))
	for k, i := range index {
		ordered[i] = k
	}
	return ordered
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
func BuildFeature(featureID int64, geom EncodedGeometry, attributes map[string]any, keyIndex map[string]uint32, valueIndex map[any]uint32) (*GSTileFeature, error) {
	tags, err := buildTags(attributes, keyIndex, valueIndex)
	if err != nil {
		return nil, fmt.Errorf("feature %d: %w", featureID, err)
	}

	//nolint:gosec // G115 feature ids are expected to be non-negative
	id := uint64(featureID)
	gtype := GSTileGeomType(geom.GeometryType)

	return &GSTileFeature{
		Id:       &id,
		Tags:     tags,
		Type:     &gtype,
		Geometry: geom.Encoding,
	}, nil
}

// Construct layer according to protobuf format
func BuildLayer(name string, features []*GSTileFeature, keyIndex map[string]uint32, valueIndex map[any]uint32) *GSTileLayer {
	keys := orderedByIndex(keyIndex)

	rawValues := orderedByIndex(valueIndex)
	values := make([]*GSTileValue, len(rawValues))
	for i, v := range rawValues {
		values[i] = toGSTileValue(v)
	}

	version := uint32(mvtVersion)
	extent := uint32(precision)
	layerName := name

	return &GSTileLayer{
		Version:  &version,
		Name:     &layerName,
		Features: features,
		Keys:     keys,
		Values:   values,
		Extent:   &extent,
	}
}

// Construct tile according to protobuf format
func BuildTile(layers ...*GSTileLayer) *GSTile {
	return &GSTile{Layers: layers}
}

// Convert value to GSTileValue. We do not support XXX_unrecognized and panic
// if an unknown type is encountered. This should never happen.
func toGSTileValue(v any) *GSTileValue {
	tv := new(GSTileValue)
	switch t := v.(type) {
	case string:
		tv.StringValue = &t
	case fmt.Stringer:
		str := t.String()
		tv.StringValue = &str
	case bool:
		tv.BoolValue = &t
	case int8:
		iv := int64(t)
		tv.SintValue = &iv
	case int16:
		iv := int64(t)
		tv.SintValue = &iv
	case int32:
		iv := int64(t)
		tv.SintValue = &iv
	case int64:
		tv.IntValue = &t
	case uint8:
		iv := int64(t)
		tv.SintValue = &iv
	case uint16:
		iv := int64(t)
		tv.SintValue = &iv
	case uint32:
		iv := int64(t)
		tv.SintValue = &iv
	case uint64:
		tv.UintValue = &t
	case float32:
		tv.FloatValue = &t
	case float64:
		tv.DoubleValue = &t
	default:
		// We should never encounter this.
		panic("Unknown type for GSTypeValue.")
	}
	return tv
}
