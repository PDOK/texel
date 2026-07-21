package tile

import "fmt"

const mvtVersion = 2

// Convert tag map to slice. Assumes all indices 0, 1, ..., len(index)-1 are
// exactly used once. Necessary since iteration over maps is in arbitrary order.
// This is a generic function since we need it with K = string and K = any.
func orderedByIndex[K comparable](index map[K]uint32) []K {
	ordered := make([]K, len(index))
	for k, i := range index {
		ordered[i] = k
	}
	return ordered
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

// Assemble layer struct
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

// Assemble layers into GSTile. This struct can be marshalled by protobuf.
func BuildTile(layers ...*GSTileLayer) *GSTile {
	return &GSTile{Layers: layers}
}
