package tile

// InternalAttributeTable maps feature ID to a map of attribute name to value, as
// produced by reading a source table's attribute columns for a set of features.
type InternalAttributeTable map[int64]map[string]any

// Convert slice to map for easy access of indices
func BuildKeyDictionary(attributeNames []string) map[string]uint32 {
	index := make(map[string]uint32, len(attributeNames))
	for i, name := range attributeNames {
		if i > int(^uint32(0)) {
			panic("Number of keys does not fit in uint32")
		}
		index[name] = uint32(i) //nolint:gosec // G115 this is guarded
	}
	return index
}

// BuildValueDictionary scans a tile's feature attributes and returns an index from
// value to a stable dictionary position. Values are compared by (dynamic type, value) -
// matching the MVT tag value semantics used by keyvalTagsMap - which comparable Go types
// (string, the int/uint/float variants, bool) support directly as map keys. nil values
// are skipped, matching the MVT spec's handling of absent tag values (see keyvalTagsMap).
func BuildValueDictionary(attributes InternalAttributeTable) map[any]uint32 {
	index := make(map[any]uint32)
	for _, attrs := range attributes {
		for _, v := range attrs {
			if v == nil {
				continue
			}
			if _, ok := index[v]; ok {
				continue
			}
			index[v] = uint32(len(index)) //nolint:gosec // G115 a tile cannot realistically hold more than 2^32 distinct values
		}
	}
	return index
}
