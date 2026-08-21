package processing

import (
	"reflect"
	"testing"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/pointindex"
	"github.com/pdok/texel/tile"
	"github.com/pdok/texel/tms20"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcessGeometry covers processGeometry and processMultiPolygon
// Uses a stub processPolygonFunc
func TestProcessGeometry(t *testing.T) {
	polyA := geom.Polygon{{{0, 0}, {1, 0}, {1, 1}, {0, 0}}}
	polyB := geom.Polygon{{{2, 2}, {3, 2}, {3, 3}, {2, 2}}}
	tileA := tile.Tile{}
	tileB := tile.Tile{}

	tests := []struct {
		name string
		// geometry passed to processGeometry
		geometry geom.Geometry
		tmIDs    []tms20.TMID
		// callResults is a list of return values of the stub function in
		// call order.
		callResults []map[tms20.TMID]SnapResult
		want        map[tms20.TMID]SnapResult
	}{
		{
			name:     "single Polygon, single tmID: passthrough of f's result",
			geometry: polyA,
			tmIDs:    []tms20.TMID{1},
			callResults: []map[tms20.TMID]SnapResult{
				{1: {Geometry: polyA, Tiles: []tile.Tile{tileA}}},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: polyA, Tiles: []tile.Tile{tileA}},
			},
		},
		{
			name:        "unsupported geometry type (Point): default branch, f not called",
			geometry:    geom.Point{9, 9},
			tmIDs:       []tms20.TMID{1, 2},
			callResults: nil,
			want: map[tms20.TMID]SnapResult{
				1: {},
				2: {},
			},
		},
		{
			name:        "nil geometry value: default branch, f not called",
			geometry:    nil,
			tmIDs:       []tms20.TMID{1},
			callResults: nil,
			want: map[tms20.TMID]SnapResult{
				1: {},
			},
		},
		{
			name:        "empty MultiPolygon (len 0, non-nil): f never called, empty result map",
			geometry:    geom.MultiPolygon{},
			tmIDs:       []tms20.TMID{1, 2},
			callResults: nil,
			want:        map[tms20.TMID]SnapResult{},
		},
		{
			name:     "MultiPolygon with 1 polygon, single tmID: passthrough",
			geometry: geom.MultiPolygon{polyA},
			tmIDs:    []tms20.TMID{1},
			callResults: []map[tms20.TMID]SnapResult{
				{1: {Geometry: polyA, Tiles: []tile.Tile{tileA}}},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: polyA, Tiles: []tile.Tile{tileA}},
			},
		},
		{
			name:     "MultiPolygon with 2 polygons, single tmID: merged geometry and concatenated tiles",
			geometry: geom.MultiPolygon{polyA, polyB},
			tmIDs:    []tms20.TMID{1},
			callResults: []map[tms20.TMID]SnapResult{
				{1: {Geometry: polyA, Tiles: []tile.Tile{tileA}}},
				{1: {Geometry: polyB, Tiles: []tile.Tile{tileB}}},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: geom.MultiPolygon{polyA, polyB}, Tiles: []tile.Tile{tileA, tileB}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			f := func(_ geom.Polygon, _ []tms20.TMID) map[tms20.TMID]SnapResult {
				if callCount >= len(tt.callResults) {
					t.Fatalf("unexpected call %d to f", callCount+1)
				}
				result := tt.callResults[callCount]
				callCount++
				return result
			}

			got := processGeometry(tt.geometry, tt.tmIDs, f)

			if callCount != len(tt.callResults) {
				t.Errorf("f called %d times, want %d", callCount, len(tt.callResults))
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processGeometry() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// newTestIndexFactory builds an IndexFactory backed by the embedded
// NetherlandsRDNewQuad tile matrix set, deep enough to hold tmIDs up to
// deepestTMID without hitting OutsideGridError for coordinates that lie
// within its bounding box (roughly x: -285402..595402, y: 22598..903402).
func newTestIndexFactory(t *testing.T, deepestTMID tms20.TMID) IndexFactory {
	t.Helper()
	tms, err := tms20.LoadEmbeddedTileMatrixSet("NetherlandsRDNewQuad")
	require.NoError(t, err)
	return pointindex.Factory(tms, deepestTMID)
}

// identitySnapFunc is a stub SnapFunc that returns the input geometry
// unchanged for every requested tile matrix, so newProcessGeometry's
// wiring (type switch, single vs. multi geometry handling, geometry
// merging) can be tested independently of the real snapping algorithm.
func identitySnapFunc(_ *pointindex.PointIndex, g geom.Geometry, tmIDs []tms20.TMID, _ Config) map[tms20.TMID][]geom.Geometry {
	result := make(map[tms20.TMID][]geom.Geometry, len(tmIDs))
	for _, tmID := range tmIDs {
		result[tmID] = []geom.Geometry{g}
	}
	return result
}

// TestNewProcessGeometry covers newProcessGeometry, which replaces
// processGeometry, exercising ProcessSingleGeometry (for Polygon,
// LineString and Point) and ProcessMultiGeometry (for MultiPolygon,
// MultiLineString and MultiPoint) through the type switch in
// newProcessGeometry.
func TestNewProcessGeometry(t *testing.T) {
	polyA := geom.Polygon{{{100000, 100000}, {100010, 100000}, {100010, 100010}, {100000, 100000}}}
	polyB := geom.Polygon{{{200000, 200000}, {200010, 200000}, {200010, 200010}, {200000, 200000}}}
	lineA := geom.LineString{{100000, 100000}, {100010, 100010}}
	pointA := geom.Point{100000, 100000}

	factory := newTestIndexFactory(t, 1)
	tmIDs := []tms20.TMID{1}
	config := Config{}

	tests := []struct {
		name     string
		geometry geom.Geometry
		want     map[tms20.TMID]SnapResult
	}{
		{
			name:     "single Polygon: passthrough via ProcessSingleGeometry",
			geometry: polyA,
			want:     map[tms20.TMID]SnapResult{1: {Geometry: polyA, Tiles: nil}},
		},
		{
			name:     "single LineString (non-polygon): passthrough via ProcessSingleGeometry",
			geometry: lineA,
			want:     map[tms20.TMID]SnapResult{1: {Geometry: lineA, Tiles: nil}},
		},
		{
			name:     "single Point (non-polygon): passthrough via ProcessSingleGeometry",
			geometry: pointA,
			want:     map[tms20.TMID]SnapResult{1: {Geometry: pointA, Tiles: nil}},
		},
		{
			name:     "MultiPolygon with 2 polygons: merged via ProcessMultiGeometry",
			geometry: geom.MultiPolygon{polyA, polyB},
			want:     map[tms20.TMID]SnapResult{1: {Geometry: geom.MultiPolygon{polyA, polyB}, Tiles: nil}},
		},
		{
			name:     "MultiLineString with 1 line (non-polygon): passthrough via ProcessMultiGeometry",
			geometry: geom.MultiLineString{lineA},
			want:     map[tms20.TMID]SnapResult{1: {Geometry: lineA, Tiles: nil}},
		},
		{
			name:     "MultiPoint with 1 point (non-polygon): passthrough via ProcessMultiGeometry",
			geometry: geom.MultiPoint{pointA},
			want:     map[tms20.TMID]SnapResult{1: {Geometry: pointA, Tiles: nil}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newProcessGeometry(tt.geometry, tmIDs, config, identitySnapFunc, factory)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newProcessGeometry() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestNewProcessGeometry_MergingMultipleNonPolygons documents that
// mergeGeometries (used by ProcessMultiGeometry) only supports merging
// Polygon/MultiPolygon results; combining more than one non-polygon
// geometry (e.g. multiple LineStrings from a MultiLineString) panics.
func TestNewProcessGeometry_MergingMultipleNonPolygons(t *testing.T) {
	lineA := geom.LineString{{100000, 100000}, {100010, 100010}}
	lineB := geom.LineString{{200000, 200000}, {200010, 200010}}

	factory := newTestIndexFactory(t, 1)
	tmIDs := []tms20.TMID{1}
	config := Config{}

	assert.Panics(t, func() {
		newProcessGeometry(geom.MultiLineString{lineA, lineB}, tmIDs, config, identitySnapFunc, factory)
	})
}

// TestNewProcessGeometry_OutsideGrid covers the OutsideGridError path
// shared by ProcessSingleGeometry: when a geometry falls outside the
// index's grid/extent and Config.IgnoreOutsideGrid is set, an empty
// result map is returned instead of panicking.
func TestNewProcessGeometry_OutsideGrid(t *testing.T) {
	outsidePoint := geom.Point{10000000, 10000000}
	factory := newTestIndexFactory(t, 1)
	tmIDs := []tms20.TMID{1}

	t.Run("IgnoreOutsideGrid=true: empty result, no panic", func(t *testing.T) {
		config := Config{IgnoreOutsideGrid: true}
		got := newProcessGeometry(outsidePoint, tmIDs, config, identitySnapFunc, factory)
		assert.Empty(t, got)
	})

	t.Run("IgnoreOutsideGrid=false: panics", func(t *testing.T) {
		config := Config{IgnoreOutsideGrid: false}
		assert.Panics(t, func() {
			newProcessGeometry(outsidePoint, tmIDs, config, identitySnapFunc, factory)
		})
	})
}
