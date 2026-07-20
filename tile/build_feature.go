package tile

import "fmt"

// Given attribute map and indices for keys and values, build tag list.
// Panic if key or value is not present.
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

// Build go-spatial compatible feature that can be marshalled.
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
