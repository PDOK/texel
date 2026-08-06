package tile

import (
	"sort"
	"testing"

	vectorTile "github.com/go-spatial/geom/encoding/mvt/vector_tile"
	oldproto "github.com/golang/protobuf/proto" //nolint:staticcheck // matches MarshalTile's use of the reflection-based marshaler
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Quick test BuildKeyDictionary
func TestBuildKeyDictionary(t *testing.T) {
	tests := []struct {
		name        string
		columnNames []string
		want        map[string]uint32
	}{
		{
			name:        "empty slice yields empty map",
			columnNames: []string{},
			want:        map[string]uint32{},
		},
		{
			name:        "single column",
			columnNames: []string{"a"},
			want:        map[string]uint32{"a": 0},
		},
		{
			name:        "multiple columns keep their position as index",
			columnNames: []string{"a", "b", "c"},
			want:        map[string]uint32{"a": 0, "b": 1, "c": 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildKeyDictionary(tt.columnNames)
			assert.Equal(t, tt.want, got)
		})
	}
}

// indexedKeys returns the keys of index sorted by their assigned uint32,
// i.e. it undoes the (arbitrary, since built from a map) assignment of
// indices so tests can assert on order-independent content.
func indexedKeys(index map[any]uint32) []any {
	keys := make([]any, len(index))
	for k, i := range index {
		keys[i] = k
	}
	return keys
}

func TestBuildValueDictionary(t *testing.T) {
	tests := []struct {
		name                 string
		attributesPerFeature InternalAttributeTable
		wantValues           []any // expected distinct, non-nil values (order-independent)
	}{
		{
			name:                 "empty table yields empty dictionary",
			attributesPerFeature: InternalAttributeTable{},
			wantValues:           []any{},
		},
		{
			name: "single feature, single attribute",
			attributesPerFeature: InternalAttributeTable{
				1: {"name": "foo"},
			},
			wantValues: []any{"foo"},
		},
		{
			name: "shared values across features are only counted once",
			attributesPerFeature: InternalAttributeTable{
				1: {"name": "foo"},
				2: {"name": "foo"},
				3: {"name": "bar"},
			},
			wantValues: []any{"foo", "bar"},
		},
		{
			name: "nil values are skipped",
			attributesPerFeature: InternalAttributeTable{
				1: {"name": "foo", "optional": nil},
			},
			wantValues: []any{"foo"},
		},
		{
			name: "mixed value types are all indexed",
			attributesPerFeature: InternalAttributeTable{
				1: {"name": "foo", "count": int64(3), "active": true},
			},
			wantValues: []any{"foo", int64(3), true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildValueDictionary(tt.attributesPerFeature)

			// Every index from 0..len(got)-1 must be used exactly once.
			assert.Len(t, got, len(tt.wantValues))
			gotValues := indexedKeys(got)
			assert.ElementsMatch(t, tt.wantValues, gotValues)
		})
	}
}

func TestBuildTags(t *testing.T) {
	keyIndex := map[string]uint32{"name": 0, "count": 1}
	valueIndex := map[any]uint32{"foo": 0, int64(3): 1}

	t.Run("empty attributes yield empty tags", func(t *testing.T) {
		tags, err := buildTags(map[string]any{}, keyIndex, valueIndex)
		require.NoError(t, err)
		assert.Empty(t, tags)
	})

	t.Run("single attribute produces a key/value index pair", func(t *testing.T) {
		tags, err := buildTags(map[string]any{"name": "foo"}, keyIndex, valueIndex)
		require.NoError(t, err)
		assert.Equal(t, []uint32{0, 0}, tags)
	})

	t.Run("nil-valued attribute is skipped", func(t *testing.T) {
		tags, err := buildTags(map[string]any{"name": "foo", "count": nil}, keyIndex, valueIndex)
		require.NoError(t, err)
		assert.Equal(t, []uint32{0, 0}, tags)
	})

	t.Run("multiple attributes produce all pairs", func(t *testing.T) {
		tags, err := buildTags(map[string]any{"name": "foo", "count": int64(3)}, keyIndex, valueIndex)
		require.NoError(t, err)

		// Tags is built from map iteration, so pair order is not
		// guaranteed; compare as a set of (key, value) pairs instead.
		require.Len(t, tags, 4)
		pairs := map[[2]uint32]bool{}
		for i := 0; i < len(tags); i += 2 {
			pairs[[2]uint32{tags[i], tags[i+1]}] = true
		}
		assert.True(t, pairs[[2]uint32{0, 0}])
		assert.True(t, pairs[[2]uint32{1, 1}])
	})

	t.Run("missing key returns an error", func(t *testing.T) {
		_, err := buildTags(map[string]any{"unknown": "foo"}, keyIndex, valueIndex)
		require.Error(t, err)
	})

	t.Run("missing value returns an error", func(t *testing.T) {
		_, err := buildTags(map[string]any{"name": "unindexed value"}, keyIndex, valueIndex)
		require.Error(t, err)
	})
}

func TestBuildLayer(t *testing.T) {
	t.Run("happy path building layer", func(t *testing.T) {
		keyIndex := map[string]uint32{"b": 1, "a": 0}
		valueIndex := map[any]uint32{"second": 1, "first": 0}
		features := []*vectorTile.Tile_Feature{{}, {}}

		layer := BuildLayer("mylayer", features, keyIndex, valueIndex)

		assert.Equal(t, "mylayer", layer.GetName())
		assert.Equal(t, uint32(mvtVersion), layer.GetVersion())
		assert.Equal(t, uint32(precision), layer.GetExtent())
		assert.Same(t, features[0], layer.GetFeatures()[0])
		assert.Same(t, features[1], layer.GetFeatures()[1])

		// Keys/values must be ordered according to their assigned index,
		// regardless of the (arbitrary) map iteration order used to build them.
		assert.Equal(t, []string{"a", "b"}, layer.GetKeys())
		require.Len(t, layer.GetValues(), 2)
		assert.Equal(t, "first", layer.GetValues()[0].GetStringValue())
		assert.Equal(t, "second", layer.GetValues()[1].GetStringValue())
	})
	t.Run("empty layer", func(t *testing.T) {
		layer := BuildLayer("empty", nil, map[string]uint32{}, map[any]uint32{})

		assert.Equal(t, "empty", layer.GetName())
		assert.Empty(t, layer.GetKeys())
		assert.Empty(t, layer.GetValues())
		assert.Empty(t, layer.GetFeatures())
	})
}

func TestBuildMVTTile(t *testing.T) {
	t.Run("happy path roundtrips features, tags and attribute values", func(t *testing.T) {
		keyIndex := BuildKeyDictionary([]string{"name"})
		attributes := InternalAttributeTable{
			1: {"name": "foo"},
			2: {"name": "bar"},
		}
		encFeatRows := []EncodedFeatureRow{
			{FeatureID: 1, Geom: EncodedGeometry{GeometryType: int32(vectorTile.Tile_POINT)}},
			{FeatureID: 2, Geom: EncodedGeometry{GeometryType: int32(vectorTile.Tile_POINT)}},
		}

		data, err := BuildMVTTile("mylayer", keyIndex, encFeatRows, attributes)
		require.NoError(t, err)
		require.NotEmpty(t, data)

		got := &vectorTile.Tile{}
		require.NoError(t, oldproto.Unmarshal(data, got))
		require.Len(t, got.GetLayers(), 1)

		layer := got.GetLayers()[0]
		assert.Equal(t, "mylayer", layer.GetName())
		require.Len(t, layer.GetFeatures(), 2)

		// Collect the string values assigned to each feature via its tags,
		// independent of feature/value ordering.
		gotNames := make([]string, 0, 2)
		for _, feature := range layer.GetFeatures() {
			require.Len(t, feature.GetTags(), 2)
			valueIdx := feature.GetTags()[1]
			gotNames = append(gotNames, layer.GetValues()[valueIdx].GetStringValue())
		}
		sort.Strings(gotNames)
		assert.Equal(t, []string{"bar", "foo"}, gotNames)
	})

	t.Run("error building a feature's tags propagates", func(t *testing.T) {
		keyIndex := map[string]uint32{"name": 0}
		attributes := InternalAttributeTable{
			1: {"unknown-key": "foo"},
		}
		encFeatRows := []EncodedFeatureRow{
			{FeatureID: 1, Geom: EncodedGeometry{}},
		}

		_, err := BuildMVTTile("mylayer", keyIndex, encFeatRows, attributes)
		require.Error(t, err)
	})
}
