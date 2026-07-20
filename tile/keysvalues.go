package tile

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
