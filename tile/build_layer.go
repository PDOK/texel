package tile

import "fmt"

// mvtVersion is the MVT spec version this codebase produces (matches go-spatial/geom's
// mvt.Version).
const mvtVersion = 2

// orderedByIndex inverts a dictionary (as built by BuildKeyDictionary/BuildValueDictionary)
// back into a slice, ordered by index. Position i in the result is dictionary index i,
// which is exactly what the Layer.Keys/Layer.Values fields require: a feature's Tags
// reference keys/values by that same position.
func orderedByIndex[K comparable](index map[K]uint32) []K {
	ordered := make([]K, len(index))
	for k, i := range index {
		ordered[i] = k
	}
	return ordered
}

// toGSTileValue converts a Go attribute value into the MVT Value oneof, mirroring
// go-spatial/geom/encoding/mvt's vectorTileValue: the exact numeric type determines which
// oneof field is set (e.g. a Go int32 becomes a zigzag-encoded "sint" value, matching the
// original library, even though our attribute values are currently only ever string,
// int64 or float64).
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
	}
	return tv
}

// BuildLayer assembles a GSTileLayer for one tile: the layer name (assumed to be the
// source table name), the already-built features (see BuildFeature), and the key/value
// dictionaries used to build those features' Tags. The dictionaries are inverted back
// into the ordered Keys/Values arrays the features' Tags indices refer to.
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

// BuildTile wraps one or more layers into the final GSTile, ready to be serialized with
// (github.com/golang/protobuf/proto).Marshal.
func BuildTile(layers ...*GSTileLayer) *GSTile {
	return &GSTile{Layers: layers}
}
