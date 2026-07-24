package tile_test

import (
	"reflect"
	"testing"

	vectorTile "github.com/go-spatial/geom/encoding/mvt/vector_tile"
	oldproto "github.com/golang/protobuf/proto" //nolint:staticcheck // needed: matches the reflection-based marshaling vector_tile.pb.go relies on
	"github.com/pdok/texel/tile"
)

// TestBuildLayerIsWireCompatibleWithVectorTile verifies that our hand-copied,
// GS-prefixed protobuf types (GSTile/GSTileLayer/GSTileValue/GSTileFeature in
// go_spatial_layer.go and go_spatial_feature.go) produce bytes on the wire that are
// indistinguishable from the real, code-generated go-spatial/geom vector_tile types.
//
// This matters because our types are not registered/generated protobuf messages: they
// only implement the minimal proto.Message interface (Reset/String/ProtoMessage) and
// rely on the same `protobuf:"..."` struct tags as the original vector_tile.pb.go for
// the classic reflection-based (github.com/golang/protobuf/proto) marshaler to encode
// them correctly. A typo in a tag (wrong field number/wire type) would silently produce
// an invalid or misinterpreted MVT tile, so this test marshals with our types and
// unmarshals with the real, generated ones to prove the two are wire-compatible.
func TestBuildLayerIsWireCompatibleWithVectorTile(t *testing.T) {
	keyIndex := tile.BuildKeyDictionary([]string{"name", "count"})
	attributes := tile.InternalAttributeTable{
		1: {"name": "foo", "count": int64(3)},
		2: {"name": "bar", "count": int64(3)}, // shares the "count" value with feature 1
	}
	valueIndex := tile.BuildValueDictionary(attributes)

	feature1, err := tile.BuildFeature(1, tile.EncodedGeometry{Encoding: []uint32{9, 2, 2}, GeometryType: 1}, attributes[1], keyIndex, valueIndex)
	if err != nil {
		t.Fatalf("BuildFeature(1): %v", err)
	}
	feature2, err := tile.BuildFeature(2, tile.EncodedGeometry{Encoding: []uint32{9, 4, 4}, GeometryType: 1}, attributes[2], keyIndex, valueIndex)
	if err != nil {
		t.Fatalf("BuildFeature(2): %v", err)
	}

	layer := tile.BuildLayer("mytable", []*vectorTile.Tile_Feature{feature1, feature2}, keyIndex, valueIndex)
	gsTile := tile.BuildTile(layer)

	b, err := oldproto.Marshal(gsTile)
	if err != nil {
		t.Fatalf("marshaling our GSTile: %v", err)
	}

	var vt vectorTile.Tile
	if err := oldproto.Unmarshal(b, &vt); err != nil {
		t.Fatalf("unmarshaling with the real vectorTile.Tile: %v", err)
	}

	if len(vt.GetLayers()) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(vt.GetLayers()))
	}
	l := vt.GetLayers()[0]

	if got := l.GetName(); got != "mytable" {
		t.Errorf("layer name = %q, want %q", got, "mytable")
	}
	if got := l.GetVersion(); got != 2 {
		t.Errorf("layer version = %d, want 2", got)
	}
	if got := l.GetExtent(); got != 4096 {
		t.Errorf("layer extent = %d, want 4096", got)
	}
	if want := []string{"name", "count"}; !equalStrings(l.GetKeys(), want) {
		t.Errorf("layer keys = %v, want %v", l.GetKeys(), want)
	}
	// "foo", "bar" and the shared int64(3) => 3 distinct values.
	if len(l.GetValues()) != 3 {
		t.Fatalf("expected 3 distinct values, got %d: %+v", len(l.GetValues()), l.GetValues())
	}
	if len(l.GetFeatures()) != 2 {
		t.Fatalf("expected 2 features, got %d", len(l.GetFeatures()))
	}

	for i, feat := range l.GetFeatures() {
		if feat.GetType() != vectorTile.Tile_POINT {
			t.Errorf("feature %d type = %v, want POINT", i, feat.GetType())
		}
		if len(feat.GetTags())%2 != 0 {
			t.Errorf("feature %d tags = %v, expected an even number of (key,value) indices", i, feat.GetTags())
		}
	}
	if feat0Geo, feat1Geo := l.GetFeatures()[0].GetGeometry(), l.GetFeatures()[1].GetGeometry(); len(feat0Geo) == 0 || len(feat1Geo) == 0 {
		t.Errorf("expected both features to keep their encoded geometry, got %v and %v", feat0Geo, feat1Geo)
	}
}

// TestBuildMVTTile verifies that BuildMVTTile's per-tile assembly (value dictionary
// construction, feature/attribute lookup, layer/tile building and marshaling)
// produces a tile whose features/attributes match expectations, by decoding the
// marshaled bytes back and resolving each feature's tags through the layer's
// Keys/Values dictionaries. This is compared structurally rather than byte-for-byte,
// since BuildValueDictionary/buildTags iterate Go maps, whose order (and thus the
// resulting dictionary/tag order) is not guaranteed across runs.
func TestBuildMVTTile(t *testing.T) {
	keys := []string{"ownerID", "function"}
	keyIndex := tile.BuildKeyDictionary(keys)
	fids := []uint64{2, 100, 101, 404}
	encGeometries := []tile.EncodedGeometry{
		{Encoding: []uint32{9, 2, 2}, GeometryType: int32(vectorTile.Tile_POINT)},
		{Encoding: []uint32{1, 2, 3}, GeometryType: int32(vectorTile.Tile_POLYGON)},
		{Encoding: []uint32{9, 4, 4}, GeometryType: int32(vectorTile.Tile_POINT)},
		{Encoding: []uint32{9, 6, 6}, GeometryType: int32(vectorTile.Tile_POINT)},
	}
	attributeTable := tile.InternalAttributeTable{
		2:   {"ownerID": int64(3), "function": "production"},
		100: {"ownerID": int64(4)},
		101: {"ownerID": int64(3), "function": "living"},
	}
	encRows := make([]tile.EncodedFeatureRow, len(fids))
	for i, fid := range fids {
		encRows[i] = tile.EncodedFeatureRow{FeatureID: int64(fid), Geom: encGeometries[i]} //nolint:gosec // G115 test fids fit within int64
	}

	layerName := "mylayer"

	got, err := tile.BuildMVTTile(layerName, keyIndex, encRows, attributeTable)
	if err != nil {
		t.Fatalf("BuildMVTTile: %v", err)
	}

	var gotTile vectorTile.Tile
	if err := oldproto.Unmarshal(got, &gotTile); err != nil {
		t.Fatalf("unmarshaling got tile: %v", err)
	}

	wantGeoms := map[uint64]tile.EncodedGeometry{
		fids[0]: encGeometries[0],
		fids[1]: encGeometries[1],
		fids[2]: encGeometries[2],
		fids[3]: encGeometries[3],
	}
	wantAttrs := map[uint64]map[string]any{
		fids[0]: {"ownerID": int64(3), "function": "production"},
		fids[1]: {"ownerID": int64(4)},
		fids[2]: {"ownerID": int64(3), "function": "living"},
		fids[3]: {},
	}

	assertMVTLayer(t, gotTile, layerName, keys, fids, wantGeoms, wantAttrs)
}

// assertMVTLayer checks that tl contains a single layer named layerName, with the
// given keys, and one feature per fid (in that order) whose geometry matches
// wantGeoms[fid] and whose tags, resolved through the layer's Keys/Values
// dictionaries, match wantAttrs[fid]. Resolving through the dictionaries makes the
// comparison independent of the (unspecified) order BuildValueDictionary assigns.
func assertMVTLayer(t *testing.T, tl vectorTile.Tile, layerName string, keys []string, fids []uint64, wantGeoms map[uint64]tile.EncodedGeometry, wantAttrs map[uint64]map[string]any) {
	t.Helper()
	if len(tl.GetLayers()) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(tl.GetLayers()))
	}
	l := tl.GetLayers()[0]
	if l.Name == nil || *l.Name != layerName { //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
		t.Errorf("layer name = %v, want %q", l.Name, layerName)
	}
	if l.Version == nil || *l.Version != 2 { //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
		t.Errorf("layer version = %v, want 2", l.Version)
	}
	if l.Extent == nil || *l.Extent != 4096 { //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
		t.Errorf("layer extent = %v, want 4096", l.Extent)
	}
	if !reflect.DeepEqual(l.GetKeys(), keys) {
		t.Errorf("layer keys = %v, want %v", l.GetKeys(), keys)
	}
	if len(l.GetFeatures()) != len(fids) {
		t.Fatalf("expected %d features, got %d", len(fids), len(l.GetFeatures()))
	}

	for i, fid := range fids {
		feat := l.GetFeatures()[i]
		if feat.Id == nil || *feat.Id != fid { //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
			t.Errorf("feature %d id = %v, want %d", i, feat.Id, fid)
			continue
		}
		wantGeom := wantGeoms[fid]
		if feat.Type == nil || *feat.Type != vectorTile.Tile_GeomType(wantGeom.GeometryType) { //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
			t.Errorf("feature %d (fid %d) type = %v, want %v", i, fid, feat.Type, wantGeom.GeometryType)
		}
		if !reflect.DeepEqual(feat.GetGeometry(), wantGeom.Encoding) {
			t.Errorf("feature %d (fid %d) geometry = %v, want %v", i, fid, feat.GetGeometry(), wantGeom.Encoding)
		}

		gotAttrs := resolveTags(t, feat.GetTags(), l.GetKeys(), l.GetValues())
		if !reflect.DeepEqual(gotAttrs, wantAttrs[fid]) {
			t.Errorf("feature %d (fid %d) attrs = %v, want %v", i, fid, gotAttrs, wantAttrs[fid])
		}
	}
}

// resolveTags decodes a feature's (key index, value index) tag pairs back into a
// plain map[string]any, using the layer's key/value dictionaries.
func resolveTags(t *testing.T, tags []uint32, keys []string, values []*vectorTile.Tile_Value) map[string]any {
	t.Helper()
	attrs := make(map[string]any, len(tags)/2)
	for i := 0; i+1 < len(tags); i += 2 {
		kidx, vidx := tags[i], tags[i+1]
		if int(kidx) >= len(keys) || int(vidx) >= len(values) {
			t.Fatalf("tag pair (%d, %d) out of range (keys=%d, values=%d)", kidx, vidx, len(keys), len(values))
		}
		attrs[keys[kidx]] = decodeGSValue(values[vidx])
	}
	return attrs
}

// decodeGSValue extracts the single set field of a GSTileValue as a plain Go value.
func decodeGSValue(v *vectorTile.Tile_Value) any {
	switch {
	case v.StringValue != nil:
		return *v.StringValue //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
	case v.IntValue != nil:
		return *v.IntValue //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
	case v.SintValue != nil:
		return *v.SintValue //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
	case v.UintValue != nil:
		return *v.UintValue //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
	case v.FloatValue != nil:
		return *v.FloatValue //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
	case v.DoubleValue != nil:
		return *v.DoubleValue //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
	case v.BoolValue != nil:
		return *v.BoolValue //nolint:protogetter // GSTile* are hand-copied plain structs with no generated getters
	default:
		return nil
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
