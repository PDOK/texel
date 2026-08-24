package processing

import (
	"reflect"
	"testing"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/pointindex"
	"github.com/pdok/texel/tms20"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestIndexFactory builds a raw point-index constructor backed by the
// embedded NetherlandsRDNewQuad tile matrix set, deep enough to hold tmIDs
// up to deepestTMID without hitting OutsideGridError for coordinates that
// lie within its bounding box (roughly x: -285402..595402, y: 22598..903402).
func newTestIndexFactory(t *testing.T, deepestTMID tms20.TMID) func() PIndex {
	t.Helper()
	tms, err := tms20.LoadEmbeddedTileMatrixSet("NetherlandsRDNewQuad")
	require.NoError(t, err)
	rawFactory := pointindex.Factory(tms, deepestTMID)
	return func() PIndex { return rawFactory() }
}

// identitySnapFunc is a stub SnapFunc that returns the input geometry
// unchanged for every requested tile matrix, so GeometryProcessor.Process's
// wiring (type switch, single vs. multi geometry handling, geometry
// merging) can be tested independently of the real snapping algorithm.
func identitySnapFunc(_ PIndex, g geom.Geometry, tmIDs []tms20.TMID, _ Config) map[tms20.TMID][]geom.Geometry {
	result := make(map[tms20.TMID][]geom.Geometry, len(tmIDs))
	for _, tmID := range tmIDs {
		result[tmID] = []geom.Geometry{g}
	}
	return result
}

// TestGeometryProcessorProcess covers GeometryProcessor.Process, which
// replaces processGeometry, exercising ProcessSingle (for Polygon,
// LineString and Point) and ProcessMulti (for MultiPolygon,
// MultiLineString and MultiPoint) through the type switch in Process.
func TestGeometryProcessorProcess(t *testing.T) {
	polyA := geom.Polygon{{{100000, 100000}, {100010, 100000}, {100010, 100010}, {100000, 100000}}}
	polyB := geom.Polygon{{{200000, 200000}, {200010, 200000}, {200010, 200010}, {200000, 200000}}}
	lineA := geom.LineString{{100000, 100000}, {100010, 100010}}
	pointA := geom.Point{100000, 100000}

	factory := newTestIndexFactory(t, 1)
	tmIDs := []tms20.TMID{1}
	config := Config{}
	processor := NewGeometryProcessor(tmIDs, config, identitySnapFunc, factory)

	tests := []struct {
		name     string
		geometry geom.Geometry
		want     map[tms20.TMID]SnapResult
	}{
		{
			name:     "single Polygon: passthrough via ProcessSingle",
			geometry: polyA,
			want:     map[tms20.TMID]SnapResult{1: {Geometry: polyA, Tiles: nil}},
		},
		{
			name:     "single LineString (non-polygon): passthrough via ProcessSingle",
			geometry: lineA,
			want:     map[tms20.TMID]SnapResult{1: {Geometry: lineA, Tiles: nil}},
		},
		{
			name:     "single Point (non-polygon): passthrough via ProcessSingle",
			geometry: pointA,
			want:     map[tms20.TMID]SnapResult{1: {Geometry: pointA, Tiles: nil}},
		},
		{
			name:     "MultiPolygon with 2 polygons: merged via ProcessMulti",
			geometry: geom.MultiPolygon{polyA, polyB},
			want:     map[tms20.TMID]SnapResult{1: {Geometry: geom.MultiPolygon{polyA, polyB}, Tiles: nil}},
		},
		{
			name:     "MultiLineString with 1 line (non-polygon): passthrough via ProcessMulti",
			geometry: geom.MultiLineString{lineA},
			want:     map[tms20.TMID]SnapResult{1: {Geometry: lineA, Tiles: nil}},
		},
		{
			name:     "MultiPoint with 1 point (non-polygon): passthrough via ProcessMulti",
			geometry: geom.MultiPoint{pointA},
			want:     map[tms20.TMID]SnapResult{1: {Geometry: pointA, Tiles: nil}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := processor.Process(tt.geometry)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Process() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestGeometryProcessorProcess_OutsideGrid covers the OutsideGridError path
// shared by ProcessSingle: when a geometry falls outside the index's
// grid/extent and Config.IgnoreOutsideGrid is set, an empty result map is
// returned instead of panicking.
func TestGeometryProcessorProcess_OutsideGrid(t *testing.T) {
	outsidePoint := geom.Point{10000000, 10000000}
	factory := newTestIndexFactory(t, 1)
	tmIDs := []tms20.TMID{1}

	t.Run("IgnoreOutsideGrid=true: empty result, no panic", func(t *testing.T) {
		config := Config{IgnoreOutsideGrid: true}
		processor := NewGeometryProcessor(tmIDs, config, identitySnapFunc, factory)
		got := processor.Process(outsidePoint)
		assert.Empty(t, got)
	})

	t.Run("IgnoreOutsideGrid=false: panics", func(t *testing.T) {
		config := Config{IgnoreOutsideGrid: false}
		processor := NewGeometryProcessor(tmIDs, config, identitySnapFunc, factory)
		assert.Panics(t, func() {
			processor.Process(outsidePoint)
		})
	})
}
