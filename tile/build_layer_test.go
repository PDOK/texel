package tile_test

import (
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

	layer := tile.BuildLayer("mytable", []*tile.GSTileFeature{feature1, feature2}, keyIndex, valueIndex)
	gsTile := tile.BuildTile(layer)

	b, err := oldproto.Marshal(gsTile)
	if err != nil {
		t.Fatalf("marshaling our GSTile: %v", err)
	}

	var vt vectorTile.Tile
	if err := oldproto.Unmarshal(b, &vt); err != nil {
		t.Fatalf("unmarshaling with the real vectorTile.Tile: %v", err)
	}

	if len(vt.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(vt.Layers))
	}
	l := vt.Layers[0]

	if got := l.GetName(); got != "mytable" {
		t.Errorf("layer name = %q, want %q", got, "mytable")
	}
	if got := l.GetVersion(); got != 2 {
		t.Errorf("layer version = %d, want 2", got)
	}
	if got := l.GetExtent(); got != 4096 {
		t.Errorf("layer extent = %d, want 4096", got)
	}
	if want := []string{"name", "count"}; !equalStrings(l.Keys, want) {
		t.Errorf("layer keys = %v, want %v", l.Keys, want)
	}
	// "foo", "bar" and the shared int64(3) => 3 distinct values.
	if len(l.Values) != 3 {
		t.Fatalf("expected 3 distinct values, got %d: %+v", len(l.Values), l.Values)
	}
	if len(l.Features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(l.Features))
	}

	for i, feat := range l.Features {
		if feat.GetType() != vectorTile.Tile_POINT {
			t.Errorf("feature %d type = %v, want POINT", i, feat.GetType())
		}
		if len(feat.Tags)%2 != 0 {
			t.Errorf("feature %d tags = %v, expected an even number of (key,value) indices", i, feat.Tags)
		}
	}
	if feat0Geo, feat1Geo := l.Features[0].Geometry, l.Features[1].Geometry; len(feat0Geo) == 0 || len(feat1Geo) == 0 {
		t.Errorf("expected both features to keep their encoded geometry, got %v and %v", feat0Geo, feat1Geo)
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
