package snap

import (
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pdok/texel/tms20"

	"github.com/go-spatial/geom/encoding/wkt"
	"github.com/pdok/texel/intgeom"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"
)

func TestPointIndex_containsPoint(t *testing.T) {
	tests := []struct {
		name string
		pt   geom.Point
		want bool
	}{
		{
			name: "centroid",
			pt:   geom.Point{0.5, 0.5},
			want: true,
		},
		{
			name: "inclusive edge left",
			pt:   geom.Point{0.5, 0.0},
			want: true,
		},
		{
			name: "inclusive edge bottom",
			pt:   geom.Point{0.0, 0.5},
			want: true,
		},
		{
			name: "exclusive edge right",
			pt:   geom.Point{1.0, 0.5},
			want: false,
		},
		{
			name: "exclusive edge top",
			pt:   geom.Point{0.5, 1.0},
			want: false,
		},
		{
			name: "inclusive corner bottomleft",
			pt:   geom.Point{0.0, 0.0},
			want: true,
		},
		{
			name: "exclusive corner bottomright",
			pt:   geom.Point{1.0, 0.0},
			want: false,
		},
		{
			name: "exclusive corner topright",
			pt:   geom.Point{1.0, 1.0},
			want: false,
		},
		{
			name: "exclusive corner topleft",
			pt:   geom.Point{0.0, 1.0},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := newSimplePointIndex(0, 1.0)
			got := ix.containsPoint(intgeom.FromGeomPoint(tt.pt))
			if got != tt.want {
				t.Errorf("containsPoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPointIndex_getQuadrantExtentAndCentroid(t *testing.T) {
	type wants struct {
		extent   intgeom.Extent
		centroid intgeom.Point
	}
	tests := []struct {
		name string
		ix   *PointIndex
		want wants
	}{
		{
			name: "simple",
			ix:   newSimplePointIndex(0, 1.0),
			want: wants{
				extent:   intgeom.Extent{0, 0, intOne, intOne},
				centroid: intgeom.Point{intHalf, intHalf},
			},
		},
		{
			name: "zero",
			ix:   newSimplePointIndex(0, 0.0),
			want: wants{
				extent:   intgeom.Extent{0, 0, 0, 0},
				centroid: intgeom.Point{0, 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extent, centroid := tt.ix.getQuadrantExtentAndCentroid(0, 0, 0)
			if !assert.EqualValues(t, tt.want.extent, extent) {
				t.Errorf("getQuadrantExtentAndCentroid() = %v, want %v", extent, tt.want.extent)
			}
			if !assert.EqualValues(t, tt.want.centroid, centroid) {
				t.Errorf("getQuadrantExtentAndCentroid() = %v, want %v", centroid, tt.want.centroid)
			}
		})
	}
}

func TestPointIndex_InsertPoint(t *testing.T) {
	tests := []struct {
		name  string
		ix    *PointIndex
		point geom.Point
		want  PointIndex
	}{
		{
			name:  "leaf",
			ix:    newSimplePointIndex(0, 1.0),
			point: geom.Point{0.5, 0.5},
			want: PointIndex{
				intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 1.0, 1.0}),
				intCentroid: intgeom.FromGeomPoint(geom.Point{0.5, 0.5}),
				hasPoints:   true,
				maxDepth:    0,
				quadrants:   [4]*PointIndex{},
			},
		},
		{
			name:  "centroid",
			ix:    newSimplePointIndex(1, 0.5),
			point: geom.Point{0.5, 0.5},
			want: PointIndex{
				intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 1.0, 1.0}),
				intCentroid: intgeom.FromGeomPoint(geom.Point{0.5, 0.5}),
				hasPoints:   true,
				maxDepth:    1,
				quadrants: [4]*PointIndex{
					nil, nil, nil, {
						level:       1,
						x:           1,
						y:           1,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{0.5, 0.5, 1.0, 1.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{0.75, 0.75}),
						hasPoints:   true,
						maxDepth:    0,
						quadrants:   [4]*PointIndex{},
					},
				},
			},
		},
		{
			name:  "deep",
			ix:    newSimplePointIndex(3, 0.5),
			point: geom.Point{2.8, 3.2},
			want: PointIndex{
				intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 4.0, 4.0}),
				intCentroid: intgeom.FromGeomPoint(geom.Point{2.0, 2.0}),
				hasPoints:   true,
				maxDepth:    3,
				quadrants: [4]*PointIndex{
					nil,
					nil,
					nil,
					{
						level:       1,
						x:           1,
						y:           1,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 2.0, 4.0, 4.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{3.0, 3.0}),
						hasPoints:   true,
						maxDepth:    2,
						quadrants: [4]*PointIndex{
							nil,
							nil,
							{
								level:       2,
								x:           2,
								y:           3,
								intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 3.0, 3.0, 4.0}),
								intCentroid: intgeom.FromGeomPoint(geom.Point{2.5, 3.5}),
								hasPoints:   true,
								maxDepth:    1,
								quadrants: [4]*PointIndex{
									nil,
									{
										level:       3,
										x:           5,
										y:           6,
										intExtent:   intgeom.FromGeomExtent(geom.Extent{2.5, 3.0, 3.0, 3.5}),
										intCentroid: intgeom.FromGeomPoint(geom.Point{2.75, 3.25}),
										hasPoints:   true,
										maxDepth:    0,
										quadrants:   [4]*PointIndex{},
									},
									nil,
									nil,
								},
							},
							nil,
						},
					},
				},
			},
		},
		{
			name:  "deeper",
			ix:    newSimplePointIndex(5, 0.5),
			point: geom.Point{2.0, 6.0},
			want: PointIndex{
				intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 16.0, 16.0}),
				intCentroid: intgeom.FromGeomPoint(geom.Point{8.0, 8.0}),
				hasPoints:   true,
				maxDepth:    5,
				quadrants: [4]*PointIndex{
					{
						level:       1,
						x:           0,
						y:           0,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 8.0, 8.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{4.0, 4.0}),
						hasPoints:   true,
						maxDepth:    4,
						quadrants: [4]*PointIndex{
							nil,
							nil,
							{
								level:       2,
								x:           0,
								y:           1,
								intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 4.0, 4.0, 8.0}),
								intCentroid: intgeom.FromGeomPoint(geom.Point{2.0, 6.0}),
								hasPoints:   true,
								maxDepth:    3,
								quadrants: [4]*PointIndex{
									nil,
									nil,
									nil,
									{
										level:       3,
										x:           1,
										y:           3,
										intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 6.0, 4.0, 8.0}),
										intCentroid: intgeom.FromGeomPoint(geom.Point{3.0, 7.0}),
										hasPoints:   true,
										maxDepth:    2,
										quadrants: [4]*PointIndex{
											{
												level:       4,
												x:           2,
												y:           6,
												intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 6.0, 3.0, 7.0}),
												intCentroid: intgeom.FromGeomPoint(geom.Point{2.5, 6.5}),
												hasPoints:   true,
												maxDepth:    1,
												quadrants: [4]*PointIndex{
													{
														level:       5,
														x:           4,
														y:           12,
														intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 6.0, 2.5, 6.5}),
														intCentroid: intgeom.FromGeomPoint(geom.Point{2.25, 6.25}),
														hasPoints:   true,
														maxDepth:    0,
														quadrants:   [4]*PointIndex{},
													},
													nil,
													nil,
													nil,
												},
											},
											nil,
											nil,
											nil,
										},
									},
								},
							},
							nil,
						},
					},
					nil,
					nil,
					nil,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := tt.ix
			ix.InsertPoint(tt.point)
			setRootExtents(&tt.want, ix.intRootExtent) //nolint:gosec
			assert.EqualValues(t, tt.want, *ix)
		})
	}
}

func TestPointIndex_SnapClosestPoints(t *testing.T) {
	tests := []struct {
		name   string
		ix     *PointIndex
		poly   geom.Polygon
		line   geom.Line
		levels []Level
		want   map[Level][][2]float64
	}{
		{
			name: "nowhere even close",
			ix:   newSimplePointIndex(4, 0.5),
			poly: geom.Polygon{
				{{0.0, 0.0}, {0.0, 2.0}, {2.0, 2.0}, {2.0, 0.0}},
			},
			line: geom.Line{{4.0, 4.0}, {8.0, 8.0}},
			want: make(map[Level][][2]float64), // nothing because the line is not part of the original geom so no points indexed
		},
		{
			name: "no extra points",
			ix:   newSimplePointIndex(5, 0.5),
			poly: geom.Polygon{
				{{0.0, 0.0}, {0.0, 8.0}, {8.0, 8.0}, {8.0, 0.0}},
				{{2.0, 2.0}, {6.0, 2.0}, {6.0, 6.0}, {2.0, 6.0}},
			},
			line: geom.Line{{2.0, 2.0}, {6.0, 2.0}},
			want: map[Level][][2]float64{5: {{2.25, 2.25}, {6.25, 2.25}}}, // same amount of points, but snapped to centroid
		},
		{
			name: "extra points (scary geom 1)",
			ix:   newSimplePointIndex(4, 0.5),
			poly: geom.Polygon{
				{{0.0, 5.0}, {5.0, 4.0}, {5.0, 0.0}, {3.0, 0.0}, {0.0, 2.0}},
				{{1.0, 3.0}, {3.0, 3.0}, {3.0, 1.0}, {1.25, 1.25}},
			},
			line: geom.Line{{3.0, 0.0}, {0.0, 2.0}},
			want: map[Level][][2]float64{4: {{3.25, 0.25}, {1.25, 1.25}, {0.25, 2.25}}}, // extra point in the middle
		},
		{
			name: "horizontal line",
			ix:   newPointIndexFromEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad", []tms20.TMID{14}),
			poly: geom.Polygon{{
				{110906.87099999999918509, 504428.79999999998835847}, // horizontal line between quadrants
				{110907.64400000000023283, 504428.79999999998835847}, // horizontal line between quadrants
			}},
			line: geom.Line{
				{110906.87099999999918509, 504428.79999999998835847}, // horizontal line between quadrants
				{110907.64400000000023283, 504428.79999999998835847},
			},
			levels: []Level{14 + 8 + 4},
			want: map[Level][][2]float64{14 + 8 + 4: {
				{110906.8709375, 504428.8065625}, // horizontal line still here
				{110907.6453125, 504428.8065625}, // horizontal line still here
			}},
		},
		{
			name: "corner case topleft",
			ix:   newSimplePointIndex(2, 1.0),
			poly: geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line: geom.Line{{0.0, 4.0}, {1.0, 3.0}},
			want: map[Level][][2]float64{},
		},
		{
			name: "corner case topright",
			ix:   newSimplePointIndex(2, 1.0),
			poly: geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line: geom.Line{{4.0, 4.0}, {3.0, 3.0}},
			want: map[Level][][2]float64{},
		},
		{
			name: "corner case bottomright",
			ix:   newSimplePointIndex(2, 1.0),
			poly: geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line: geom.Line{{4.0, 0.0}, {3.0, 1.0}},
			want: map[Level][][2]float64{},
		},
		{
			name: "corner case bottomleft",
			ix:   newSimplePointIndex(2, 1.0),
			poly: geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line: geom.Line{{0.0, 0.0}, {1.0, 1.0}},
			want: map[Level][][2]float64{2: {{1.5, 1.5}}},
		},
		{
			name: "edge case top",
			ix:   newSimplePointIndex(2, 1.0),
			poly: geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line: geom.Line{{0.0, 3.0}, {4.0, 3.0}},
			want: map[Level][][2]float64{},
		},
		{
			name: "edge case right",
			ix:   newSimplePointIndex(2, 1.0),
			poly: geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line: geom.Line{{3.0, 4.0}, {3.0, 0.0}},
			want: map[Level][][2]float64{},
		},
		{
			name: "edge case bottom",
			ix:   newSimplePointIndex(2, 1.0),
			poly: geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line: geom.Line{{0.0, 1.0}, {4.0, 1.0}},
			want: map[Level][][2]float64{2: {{1.5, 1.5}, {2.5, 1.5}}},
		},
		{
			name: "edge case left",
			ix:   newSimplePointIndex(2, 1.0),
			poly: geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line: geom.Line{{1.0, 0.0}, {1.0, 4.0}},
			want: map[Level][][2]float64{2: {{1.5, 1.5}, {1.5, 2.5}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := tt.ix
			poly := tt.poly
			ix.InsertPolygon(&poly)
			levels := tt.levels
			if levels == nil {
				levels = []Level{ix.maxDepth}
			}
			got := ix.SnapClosestPoints(tt.line, asKeys(levels))
			if !assert.EqualValues(t, tt.want, got) {
				ix.toWkt(os.Stdout)
				t.Errorf("SnapClosestPoints() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPointIndex_lineIntersects(t *testing.T) {
	tests := []struct {
		name   string
		extent intgeom.Extent
		line   intgeom.Line
		want   bool
	}{
		{
			name: "false positive with naive integer intersect implementation",
			extent: intgeom.Extent{
				135196160000000,
				516981760000000,
				135202880000000,
				516988480000000,
			},
			line: intgeom.Line{
				{135201147999999, 516929654000000},
				{135145991000000, 516996354000000},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := &PointIndex{
				intExtent: tt.extent,
			}
			got := ix.lineIntersects(tt.line)
			if tt.want != got {
				t.Logf("extent = %v", wkt.MustEncode(tt.extent.ToGeomExtent()))
				t.Errorf("lineIntersects(%v) = %v, want %v", wkt.MustEncode(tt.line.ToGeomLine()), got, tt.want)
			}
		})
	}
}

func newSimplePointIndex(maxDepth Level, cellSize float64) *PointIndex {
	span := cellSize * float64(pow2(maxDepth))
	ix := PointIndex{
		intRootExtent: intgeom.Extent{0.0, 0.0, intgeom.FromGeomOrd(span), intgeom.FromGeomOrd(span)},
		maxDepth:      maxDepth,
	}
	ix.intExtent, ix.intCentroid = ix.getQuadrantExtentAndCentroid(0, 0, 0)
	return &ix
}

//nolint:unparam
func newSimpleTileMatrixSet(maxDepth Level, cellSize float64) *tms20.TileMatrixSet {
	zeroZero := tms20.TwoDPoint([2]float64{0.0, 0.0})
	tms := tms20.TileMatrixSet{
		CRS:          fakeCRS{},
		OrderedAxes:  []string{"X", "Y"},
		TileMatrices: make(map[tms20.TMID]tms20.TileMatrix),
	}
	for tmID := 0; tmID <= int(maxDepth); tmID++ {
		tmCellSize := cellSize * float64(pow2(maxDepth-uint(tmID)))
		tms.TileMatrices[tmID] = tms20.TileMatrix{
			ID:               "0",
			ScaleDenominator: tmCellSize / tms20.StandardizedRenderingPixelSize,
			CellSize:         tmCellSize,
			CornerOfOrigin:   tms20.BottomLeft,
			PointOfOrigin:    &zeroZero,
			TileWidth:        1,
			TileHeight:       1,
			MatrixWidth:      1,
			MatrixHeight:     1,
		}
	}
	return &tms
}

func loadEmbeddedTileMatrixSet(t *testing.T, tmsID string) *tms20.TileMatrixSet {
	tms, err := tms20.LoadEmbeddedTileMatrixSet(tmsID)
	require.NoError(t, err)
	return &tms
}

func newPointIndexFromEmbeddedTileMatrixSet(t *testing.T, tmsID string, tmIDs []tms20.TMID) *PointIndex {
	tms := loadEmbeddedTileMatrixSet(t, tmsID)
	deepestID := slices.Max(tmIDs)
	ix := newPointIndexFromTileMatrixSet(tms, deepestID)
	return ix
}

func setRootExtents(ix *PointIndex, intRootExtent intgeom.Extent) {
	ix.intRootExtent = intRootExtent
	for _, qu := range ix.quadrants {
		if qu == nil {
			continue
		}
		setRootExtents(qu, intRootExtent)
	}
}

type fakeCRS struct{}

func (f fakeCRS) Description() string {
	return ""
}

func (f fakeCRS) Authority() string {
	return ""
}

func (f fakeCRS) Version() string {
	return ""
}

func (f fakeCRS) Code() string {
	return ""
}
