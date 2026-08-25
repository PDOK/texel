package processing

import (
	"errors"
	"reflect"
	"testing"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/intgeom"
	"github.com/pdok/texel/pointindex"
	"github.com/pdok/texel/tile"
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

// Test tile detector //

type mockTileDetector struct {
	lineTraceQueue [][]tile.Tile
	bboxQueue      [][]tile.Tile
}

func (mock *mockTileDetector) DetectTilesViaLineTrace(_ geom.Geometry, _ tms20.TMID, _ uint) []tile.Tile {
	if len(mock.lineTraceQueue) == 0 {
		panic("Empty queue")
	}
	result := mock.lineTraceQueue[0]
	mock.lineTraceQueue = mock.lineTraceQueue[1:]

	return result
}

func (mock *mockTileDetector) GetQBBoxWithBuffer(_ tms20.TMID, _ uint) []tile.Tile {
	if len(mock.bboxQueue) == 0 {
		panic("Empty queue")
	}
	result := mock.bboxQueue[0]
	tail := mock.bboxQueue[1:]
	mock.bboxQueue = tail

	return result
}

func (mock *mockTileDetector) isEmpty() bool {
	return len(mock.lineTraceQueue) == 0 && len(mock.bboxQueue) == 0
}

// Test a few variations of config and behaviour of resulting functions
func TestNewTileDetector(t *testing.T) {
	tileA := tile.Tile{X: 1, Y: 1}
	tileB := tile.Tile{X: 2, Y: 2}
	geomA := geom.Point{1, 1}
	geomB := geom.Point{2, 2}

	tests := []struct {
		name           string
		config         Config
		tmsID          tms20.TMID
		newGeometries  []geom.Geometry
		lineTraceQueue [][]tile.Tile
		bboxQueue      [][]tile.Tile
		wantTiles      []tile.Tile
	}{
		{
			name:           "encoding disabled: no PIndex calls, nil result",
			config:         Config{EncodeTiles: false, UseLineTrace: true, Buffer: 3},
			tmsID:          1,
			newGeometries:  []geom.Geometry{geomA, geomB},
			lineTraceQueue: nil,
			bboxQueue:      nil,
			wantTiles:      nil,
		},
		{
			name:           "line trace: called once per geometry, tiles concatenated in order",
			config:         Config{EncodeTiles: true, UseLineTrace: true, Buffer: 3},
			tmsID:          2,
			newGeometries:  []geom.Geometry{geomA, geomB},
			lineTraceQueue: [][]tile.Tile{{tileA}, {tileA, tileB}},
			// TODO Duplicate tile A
			wantTiles: []tile.Tile{tileA, tileB},
		},
		{
			name:          "bbox: called exactly once regardless of geometry count",
			config:        Config{EncodeTiles: true, UseLineTrace: false, Buffer: 5},
			tmsID:         3,
			newGeometries: []geom.Geometry{geomA, geomB},
			bboxQueue:     [][]tile.Tile{{tileA, tileB}},
			wantTiles:     []tile.Tile{tileA, tileB},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTileDetector{
				lineTraceQueue: tt.lineTraceQueue,
				bboxQueue:      tt.bboxQueue,
			}
			detect := newTileDetector(tt.config)

			got := detect(mock, tt.tmsID, tt.newGeometries)

			assert.Equal(t, tt.wantTiles, got)
			assert.True(t, mock.isEmpty())
		})
	}
}

// TestNewEncoder covers newEncoder's Config.EncodeTiles gate and, when
// enabled, that it produces exactly one tile.EncodedGeometry per
// SnapResult.Tiles entry, reusing a single precomputed default (tile-
// filling) encoding across all of them.
func TestNewEncoder(t *testing.T) {
	tests := []struct {
		name       string
		config     Config
		snapResult SnapResult
		wantNil    bool
	}{
		{
			name:   "encoding disabled: nil regardless of tiles present",
			config: Config{EncodeTiles: false},
			snapResult: SnapResult{
				Geometry: geom.Point{1, 1},
				Tiles: []tile.Tile{
					{X: 1, Y: 1, IsContained: true},
					{X: 2, Y: 2, IsContained: true},
				},
			},
			wantNil: true,
		},
		{
			name:   "encoding enabled: one EncodedGeometry per tile, in order",
			config: Config{EncodeTiles: true, Buffer: 0},
			snapResult: SnapResult{
				Geometry: geom.Point{1, 1},
				Tiles: []tile.Tile{
					{X: 1, Y: 1, IsContained: true},
					{X: 2, Y: 3, IsContained: true},
					{X: 4, Y: 5, IsContained: true},
				},
			},
		},
		{
			name:   "encoding enabled, no tiles: empty (non-nil) slice",
			config: Config{EncodeTiles: true, Buffer: 0},
			snapResult: SnapResult{
				Geometry: geom.Point{1, 1},
				Tiles:    []tile.Tile{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encode := newEncoder(tt.config)

			got := encode(tt.snapResult)

			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.Len(t, got, len(tt.snapResult.Tiles))

			defaultEnc, err := tile.NewDefaultEncoding(tt.config.Buffer)
			require.NoError(t, err)
			for i, q := range tt.snapResult.Tiles {
				want := tile.MvtEncodeGeometry(q, tt.snapResult.Geometry, defaultEnc)
				assert.Equal(t, want, got[i])
			}
		})
	}
}

////////////////////////////////
// Test ProcessSingleGeometry //
////////////////////////////////

// For mocking PIndex
type MockPointIndexProcessing struct {
	// Keep track of which function should be called next
	state string
	// Keep track of how often "Detect" has been called
	numgeomsleft *int
	// Whether "InsertGeometry" should raise error
	insertErr error
	// Tile slice to eventually return
	detectTilesResult map[tms20.TMID][]tile.Tile
}

func createMockIndexFactory(numDetect *int, insertErr error, detectTilesResult map[tms20.TMID][]tile.Tile) func() PIndex {
	return func() PIndex {
		return &MockPointIndexProcessing{
			state:             "insert",
			numgeomsleft:      numDetect,
			insertErr:         insertErr,
			detectTilesResult: detectTilesResult,
		}
	}
}

func (mock *MockPointIndexProcessing) InsertGeometry(_ geom.Geometry) error {
	if mock.state != "insert" {
		panic("InsertGeometry called out of order")
	}
	if mock.insertErr != nil {
		return mock.insertErr
	}
	mock.state = "snap"
	return nil
}

func createMockSnapFunc(result map[tms20.TMID][]geom.Geometry) SnapFunc {
	return func(ix PIndex, _ geom.Geometry, _ []tms20.TMID, _ Config) map[tms20.TMID][]geom.Geometry {
		if mock, ok := ix.(*MockPointIndexProcessing); ok {
			if mock.state != "snap" {
				panic("snap called out of order")
			}
			mock.state = "detect"
		}
		return result
	}
}

func (mock *MockPointIndexProcessing) DetectTilesViaLineTrace(_ geom.Geometry, _ tms20.TMID, _ uint) []tile.Tile {
	panic("MockPointIndexProcessing should be used with BBox detection")
}

func (mock *MockPointIndexProcessing) GetQBBoxWithBuffer(tmsID tms20.TMID, _ uint) []tile.Tile {
	if mock.state != "detect" {
		panic("GetQBBoxWithBuffer called out of order")
	}

	*mock.numgeomsleft--

	if *mock.numgeomsleft == 0 {
		mock.state = "done"
	}

	return mock.detectTilesResult[tmsID]
}

func (mock *MockPointIndexProcessing) InternalPixelLevelFromTmsID(_ tms20.TMID) pointindex.Level {
	return 0
}

func (mock *MockPointIndexProcessing) SnapClosestPoints(_ geom.Line, _ map[pointindex.Level]any, _ int) map[pointindex.Level][][2]float64 {
	return nil
}

func (mock *MockPointIndexProcessing) GetHitMultiple(_ pointindex.Level) map[intgeom.Point][]int {
	return nil
}

func TestProcessSingle(t *testing.T) {
	polyA := geom.Polygon{{{0, 0}, {1, 0}, {1, 1}, {0, 0}}}
	polyB := geom.Polygon{{{10, 10}, {11, 10}, {11, 11}, {10, 10}}}
	tileA := tile.Tile{X: 1, Y: 1}
	tileB := tile.Tile{X: 2, Y: 2}

	tests := []struct {
		name              string
		tmIDs             []tms20.TMID
		insertErr         error
		snapOutput        map[tms20.TMID][]geom.Geometry
		detectTilesResult map[tms20.TMID][]tile.Tile
		wantPanic         bool
		config            Config
		geometry          geom.Geometry
		want              map[tms20.TMID]SnapResult
	}{
		{
			name:      "index factory error: panics",
			tmIDs:     []tms20.TMID{1},
			insertErr: errors.New("boom"),
			wantPanic: true,
		},
		{
			name:      "index skipped (e.g. outside grid): empty, non-nil result map",
			tmIDs:     []tms20.TMID{1, 2},
			insertErr: pointindex.OutsideGridError{},
			config:    Config{IgnoreOutsideGrid: true},
			want:      map[tms20.TMID]SnapResult{},
		},
		{
			name:   "single tile matrix: snap/detectTiles results wired into one SnapResult",
			tmIDs:  []tms20.TMID{1},
			config: Config{EncodeTiles: true, UseLineTrace: false},
			snapOutput: map[tms20.TMID][]geom.Geometry{
				1: {polyA},
			},
			detectTilesResult: map[tms20.TMID][]tile.Tile{
				1: {tileA},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: polyA, Tiles: []tile.Tile{tileA}},
			},
		},
		{
			name:   "multiple tile matrices: independent per tmID, multi-geometry result built as multipolygon",
			tmIDs:  []tms20.TMID{1, 2},
			config: Config{EncodeTiles: true, UseLineTrace: false},
			snapOutput: map[tms20.TMID][]geom.Geometry{
				1: {polyA},
				2: {polyA, polyB},
			},
			detectTilesResult: map[tms20.TMID][]tile.Tile{
				1: {tileA},
				2: {tileA, tileB},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: polyA, Tiles: []tile.Tile{tileA}},
				2: {Geometry: geom.MultiPolygon{polyA, polyB}, Tiles: []tile.Tile{tileA, tileB}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This counts how often "tileDetect" has been called
			numDetect := len(tt.tmIDs)

			processor := NewGeometryProcessor(tt.tmIDs, tt.config, createMockSnapFunc(tt.snapOutput), createMockIndexFactory(&numDetect, tt.insertErr, tt.detectTilesResult))

			if tt.wantPanic {
				assert.Panics(t, func() {
					processor.ProcessSingle(tt.geometry)
				})
				return
			}

			got := processor.ProcessSingle(tt.geometry)
			assert.Equal(t, tt.want, got)

			// Check that "tileDetect" has been called the correct amount of times.
			if len(tt.detectTilesResult) > 0 {
				assert.Equal(t, 0, numDetect)
			}
		})
	}
}

func TestProcessMulti(t *testing.T) {
	pointA := geom.Point{1, 1}
	pointB := geom.Point{2, 2}
	pointC := geom.Point{3, 3}
	tileA := tile.Tile{X: 1, Y: 1}
	tileB := tile.Tile{X: 2, Y: 2}
	tileC := tile.Tile{X: 3, Y: 3}

	tests := []struct {
		name          string
		tmIDs         []tms20.TMID
		multiGeometry []geom.Geometry
		tilesByPoint  map[geom.Point]map[tms20.TMID][]tile.Tile
		want          map[tms20.TMID]SnapResult
	}{
		{
			name:          "single geometry: passthrough, no MultiPoint wrapping",
			tmIDs:         []tms20.TMID{1},
			multiGeometry: []geom.Geometry{pointA},
			tilesByPoint: map[geom.Point]map[tms20.TMID][]tile.Tile{
				pointA: {1: {tileA}},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: pointA, Tiles: []tile.Tile{tileA}},
			},
		},
		{
			name:          "two geometries: merged into MultiPoint, tiles concatenated",
			tmIDs:         []tms20.TMID{1},
			multiGeometry: []geom.Geometry{pointA, pointB},
			tilesByPoint: map[geom.Point]map[tms20.TMID][]tile.Tile{
				pointA: {1: {tileA}},
				pointB: {1: {tileB}},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: geom.MultiPoint{pointA, pointB}, Tiles: []tile.Tile{tileA, tileB}},
			},
		},
		{
			name:          "three geometries across two tile matrices: independent merge/tiles per tmID",
			tmIDs:         []tms20.TMID{1, 2},
			multiGeometry: []geom.Geometry{pointA, pointB, pointC},
			tilesByPoint: map[geom.Point]map[tms20.TMID][]tile.Tile{
				pointA: {1: {tileA}, 2: nil},
				pointB: {1: {tileB}, 2: {tileB}},
				pointC: {1: {tileC}, 2: {tileC}},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: geom.MultiPoint{pointA, pointB, pointC}, Tiles: []tile.Tile{tileA, tileB, tileC}},
				2: {Geometry: geom.MultiPoint{pointA, pointB, pointC}, Tiles: []tile.Tile{tileB, tileC}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gp := &GeometryProcessor{
				tmIDs: tt.tmIDs,
				newIndex: func(_ geom.Geometry) (PIndex, error) {
					return &MockPointIndexProcessing{}, nil
				},
				snap: func(_ PIndex, g geom.Geometry) map[tms20.TMID][]geom.Geometry {
					result := make(map[tms20.TMID][]geom.Geometry, len(tt.tmIDs))
					for _, tmID := range tt.tmIDs {
						result[tmID] = []geom.Geometry{g}
					}
					return result
				},
				detectTiles: func(_ TDetector, tmsID tms20.TMID, newGeometries []geom.Geometry) []tile.Tile {
					p := newGeometries[0].(geom.Point)
					return tt.tilesByPoint[p][tmsID]
				},
			}

			got := gp.ProcessMulti(tt.multiGeometry)
			assert.Equal(t, tt.want, got)
		})
	}
}
