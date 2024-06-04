package pointindex

import (
	"fmt"
	"os"
	"testing"

	"github.com/pdok/texel/mapslicehelp"
	"github.com/pdok/texel/mathhelp"
	"github.com/pdok/texel/morton"

	"github.com/stretchr/testify/require"

	"github.com/pdok/texel/tms20"

	"github.com/go-spatial/geom/encoding/wkt"
	"github.com/pdok/texel/intgeom"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"
)

func assertNoErr(t assert.TestingT, err error, _ ...any) bool {
	return assert.Nil(t, err)
}

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
	extent := intgeom.Extent{0, 0, intgeom.One, intgeom.One}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsPoint(intgeom.FromGeomPoint(tt.pt), extent)
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
		name          string
		intRootExtent intgeom.Extent
		want          wants
	}{
		{
			name:          "simple",
			intRootExtent: intgeom.Extent{0, 0, intgeom.One, intgeom.One},
			want: wants{
				extent:   intgeom.Extent{0, 0, intgeom.One, intgeom.One},
				centroid: intgeom.Point{intgeom.Half, intgeom.Half},
			},
		},
		{
			name:          "zero",
			intRootExtent: intgeom.Extent{0, 0, 0.0, 0.0},
			want: wants{
				extent:   intgeom.Extent{0, 0, 0, 0},
				centroid: intgeom.Point{0, 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := PointIndex{
				deepestLevel: 0,
				deepestSize:  1,
				deepestRes:   tt.intRootExtent.XSpan() / 1,
			}
			extent, centroid := ix.getQuadrantExtentAndCentroid(0, 0, 0, tt.intRootExtent)
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
				Quadrant: Quadrant{
					intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 1.0, 1.0}),
					intCentroid: intgeom.FromGeomPoint(geom.Point{0.5, 0.5}),
				},
				deepestLevel: 0,
				deepestSize:  mathhelp.Pow2(0),
				deepestRes:   intgeom.FromGeomOrd(1.0) / intgeom.M(mathhelp.Pow2(0)),
				quadrants: map[Level]map[morton.Z]Quadrant{0: {0: Quadrant{
					intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 1.0, 1.0}),
					intCentroid: intgeom.FromGeomPoint(geom.Point{0.5, 0.5}),
				}}},
			},
		},
		{
			name:  "centroid",
			ix:    newSimplePointIndex(1, 0.5),
			point: geom.Point{0.5, 0.5},
			want: PointIndex{
				Quadrant: Quadrant{
					intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 1.0, 1.0}),
					intCentroid: intgeom.FromGeomPoint(geom.Point{0.5, 0.5}),
				},
				deepestLevel: 1,
				deepestSize:  mathhelp.Pow2(1),
				deepestRes:   intgeom.FromGeomOrd(1.0) / intgeom.M(mathhelp.Pow2(1)),
				quadrants: map[Level]map[morton.Z]Quadrant{
					0: {0: Quadrant{
						intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 1.0, 1.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{0.5, 0.5}),
					}},
					1: {0b11: Quadrant{
						z:           0b11,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{0.5, 0.5, 1.0, 1.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{0.75, 0.75}),
					}},
				},
			},
		},
		{
			name:  "deep",
			ix:    newSimplePointIndex(3, 0.5),
			point: geom.Point{2.8, 3.2},
			want: PointIndex{
				Quadrant: Quadrant{
					intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 4.0, 4.0}),
					intCentroid: intgeom.FromGeomPoint(geom.Point{2.0, 2.0}),
				},
				deepestLevel: 3,
				deepestSize:  mathhelp.Pow2(3),
				deepestRes:   intgeom.FromGeomOrd(4.0) / intgeom.M(mathhelp.Pow2(3)),
				quadrants: map[Level]map[morton.Z]Quadrant{
					0: {0: Quadrant{
						z:           0,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 4.0, 4.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{2.0, 2.0}),
					}},
					1: {0b11: Quadrant{
						z:           0b11,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 2.0, 4.0, 4.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{3.0, 3.0}),
					}},
					2: {0b1110: Quadrant{
						z:           0b1110,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 3.0, 3.0, 4.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{2.5, 3.5}),
					}},
					3: {0b111001: Quadrant{
						z:           0b111001,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{2.5, 3.0, 3.0, 3.5}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{2.75, 3.25}),
					}},
				},
			},
		},
		{
			name:  "deeper",
			ix:    newSimplePointIndex(5, 0.5),
			point: geom.Point{2.0, 6.0},
			want: PointIndex{
				Quadrant: Quadrant{
					intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 16.0, 16.0}),
					intCentroid: intgeom.FromGeomPoint(geom.Point{8.0, 8.0}),
				},
				deepestLevel: 5,
				deepestSize:  mathhelp.Pow2(5),
				deepestRes:   intgeom.FromGeomOrd(16.0) / intgeom.M(mathhelp.Pow2(5)),
				quadrants: map[Level]map[morton.Z]Quadrant{
					0: {0: Quadrant{
						z:           0,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 16.0, 16.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{8.0, 8.0}),
					}},
					1: {0b0: Quadrant{
						z:           0b0,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 8.0, 8.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{4.0, 4.0}),
					}},
					2: {0b10: Quadrant{
						z:           0b10,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 4.0, 4.0, 8.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{2.0, 6.0}),
					}},
					3: {morton.MustToZ(1, 3): Quadrant{
						z:           morton.MustToZ(1, 3),
						intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 6.0, 4.0, 8.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{3.0, 7.0}),
					}},
					4: {morton.MustToZ(2, 6): Quadrant{
						z:           morton.MustToZ(2, 6),
						intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 6.0, 3.0, 7.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{2.5, 6.5}),
					}},
					5: {morton.MustToZ(4, 12): Quadrant{
						z:           morton.MustToZ(4, 12),
						intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 6.0, 2.5, 6.5}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{2.25, 6.25}),
					}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := tt.ix
			ix.InsertPoint(tt.point)
			if tt.want.hitOnce == nil {
				tt.want.hitOnce = make(map[morton.Z]map[intgeom.Point][]int)
			}
			if tt.want.hitMultiple == nil {
				tt.want.hitMultiple = make(map[morton.Z]map[intgeom.Point][]int)
			}
			assert.EqualValues(t, tt.want, *ix)
		})
	}
}

func TestPointIndex_InsertPoint_Deepest(t *testing.T) {
	tests := []struct {
		name  string
		tmsID string
		tmID  tms20.TMID
		point geom.Point
		want  Quadrant
	}{
		{
			name:  "example point causing panic if span/size is not round",
			tmsID: "WebMercatorQuad",
			tmID:  17,
			point: geom.Point{642743.3299, 6898063.027},
			want: Quadrant{ // want no panic
				z:           225954093760580854,
				intExtent:   intgeom.Extent{6427432856623948, 68980629641080914, 6427433603079302, 68980630387536268},
				intCentroid: intgeom.Point{6427433229851625, 68980630014308591},
			},
		},
		{
			name:  "another point causing panic if span/size is not round, even if precision is upped, because of quadrants shifting",
			tmsID: "WebMercatorQuad",
			tmID:  17,
			point: geom.Point{642743.4434337, 6898062.9994258},
			want: Quadrant{ // want no panic
				z:           225954093760581026,
				intExtent:   intgeom.Extent{6427434349534656, 68980629641080914, 6427435095990010, 68980630387536268},
				intCentroid: intgeom.Point{6427434722762333, 68980630014308591},
			},
		},
		{
			name:  "in RD, deepestRes is round, so no problems with quadrants shifting",
			tmsID: "NetherlandsRDNewQuad",
			tmID:  16,
			point: geom.Point{155000, 463000},
			want: Quadrant{
				z:           0xc0000000000000,
				intExtent:   intgeom.FromGeomExtent(geom.Extent{155000, 463000, 155000 + 0.00328125, 463000 + 0.00328125}),
				intCentroid: intgeom.FromGeomPoint(geom.Point{155000 + (0.00328125 / 2), 463000 + (0.00328125 / 2)}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tms := loadEmbeddedTileMatrixSet(t, tt.tmsID)
			ix, err := FromTileMatrixSet(tms, tt.tmID)
			require.Nil(t, err)

			ix.InsertPoint(tt.point)
			assert.Equal(t, 1, len(ix.quadrants[ix.deepestLevel]))
			for z, quadrant := range ix.quadrants[ix.deepestLevel] {
				assert.EqualValues(t, tt.want, quadrant)
				assert.EqualValues(t, tt.want.z, z)
			}
		})
	}
}

func TestPointIndex_SnapClosestPoints(t *testing.T) {
	tests := []struct {
		name   string
		ix     *PointIndex
		poly   geom.Polygon
		line   geom.Line
		ringID int
		levels []Level
		want   map[Level][][2]float64
	}{
		{
			name: "nowhere even close",
			ix:   newSimplePointIndex(4, 0.5),
			poly: geom.Polygon{
				{{0.0, 0.0}, {0.0, 2.0}, {2.0, 2.0}, {2.0, 0.0}},
			},
			line:   geom.Line{{4.0, 4.0}, {8.0, 8.0}},
			ringID: 0,
			want:   make(map[Level][][2]float64), // nothing because the line is not part of the original geom so no points indexed
		},
		{
			name: "no extra points",
			ix:   newSimplePointIndex(5, 0.5),
			poly: geom.Polygon{
				{{0.0, 0.0}, {0.0, 8.0}, {8.0, 8.0}, {8.0, 0.0}},
				{{2.0, 2.0}, {6.0, 2.0}, {6.0, 6.0}, {2.0, 6.0}},
			},
			line:   geom.Line{{2.0, 2.0}, {6.0, 2.0}},
			ringID: 1,
			want:   map[Level][][2]float64{5: {{2.25, 2.25}, {6.25, 2.25}}}, // same amount of points, but snapped to centroid
		},
		{
			name: "extra points (scary geom 1)",
			ix:   newSimplePointIndex(4, 0.5),
			poly: geom.Polygon{
				{{0.0, 5.0}, {5.0, 4.0}, {5.0, 0.0}, {3.0, 0.0}, {0.0, 2.0}},
				{{1.0, 3.0}, {3.0, 3.0}, {3.0, 1.0}, {1.25, 1.25}},
			},
			line:   geom.Line{{3.0, 0.0}, {0.0, 2.0}},
			ringID: 0,
			want:   map[Level][][2]float64{4: {{3.25, 0.25}, {1.25, 1.25}, {0.25, 2.25}}}, // extra point in the middle
		},
		{
			name: "horizontal line",
			ix:   newPointIndexFromEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad", 14),
			poly: geom.Polygon{{
				{110906.87099999999918509, 504428.79999999998835847}, // horizontal line between quadrants
				{110907.64400000000023283, 504428.79999999998835847}, // horizontal line between quadrants
			}},
			line: geom.Line{
				{110906.87099999999918509, 504428.79999999998835847}, // horizontal line between quadrants
				{110907.64400000000023283, 504428.79999999998835847},
			},
			ringID: 0,
			levels: []Level{14 + 8 + 4},
			want: map[Level][][2]float64{14 + 8 + 4: {
				{110906.8709375, 504428.8065625}, // horizontal line still here
				{110907.6453125, 504428.8065625}, // horizontal line still here
			}},
		},
		{
			name:   "corner case topleft",
			ix:     newSimplePointIndex(2, 1.0),
			poly:   geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line:   geom.Line{{0.0, 4.0}, {1.0, 3.0}},
			ringID: 0,
			want:   map[Level][][2]float64{},
		},
		{
			name:   "corner case topright",
			ix:     newSimplePointIndex(2, 1.0),
			poly:   geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line:   geom.Line{{4.0, 4.0}, {3.0, 3.0}},
			ringID: 0,
			want:   map[Level][][2]float64{},
		},
		{
			name:   "corner case bottomright",
			ix:     newSimplePointIndex(2, 1.0),
			poly:   geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line:   geom.Line{{4.0, 0.0}, {3.0, 1.0}},
			ringID: 0,
			want:   map[Level][][2]float64{},
		},
		{
			name:   "corner case bottomleft",
			ix:     newSimplePointIndex(2, 1.0),
			poly:   geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line:   geom.Line{{0.0, 0.0}, {1.0, 1.0}},
			ringID: 0,
			want:   map[Level][][2]float64{2: {{1.5, 1.5}}},
		},
		{
			name:   "edge case top",
			ix:     newSimplePointIndex(2, 1.0),
			poly:   geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line:   geom.Line{{0.0, 3.0}, {4.0, 3.0}},
			ringID: 0,
			want:   map[Level][][2]float64{},
		},
		{
			name:   "edge case right",
			ix:     newSimplePointIndex(2, 1.0),
			poly:   geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line:   geom.Line{{3.0, 4.0}, {3.0, 0.0}},
			ringID: 0,
			want:   map[Level][][2]float64{},
		},
		{
			name:   "edge case bottom",
			ix:     newSimplePointIndex(2, 1.0),
			poly:   geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line:   geom.Line{{0.0, 1.0}, {4.0, 1.0}},
			ringID: 0,
			want:   map[Level][][2]float64{2: {{1.5, 1.5}, {2.5, 1.5}}},
		},
		{
			name:   "edge case left",
			ix:     newSimplePointIndex(2, 1.0),
			poly:   geom.Polygon{{{1.5, 1.5}, {2.5, 1.5}, {2.5, 2.5}, {1.5, 2.5}}},
			line:   geom.Line{{1.0, 0.0}, {1.0, 4.0}},
			ringID: 0,
			want:   map[Level][][2]float64{2: {{1.5, 1.5}, {1.5, 2.5}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := tt.ix
			poly := tt.poly
			ix.InsertPolygon(poly)
			levels := tt.levels
			if levels == nil {
				levels = []Level{ix.deepestLevel}
			}
			got := ix.SnapClosestPoints(tt.line, mapslicehelp.AsKeys(levels), tt.ringID)
			if !assert.EqualValues(t, tt.want, got) {
				ix.ToWkt(os.Stdout)
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
			got := lineIntersects(tt.line, tt.extent)
			if tt.want != got {
				t.Logf("extent = %v", wkt.MustEncode(tt.extent.ToGeomExtent()))
				t.Errorf("lineIntersects(%v) = %v, want %v", wkt.MustEncode(tt.line.ToGeomLine()), got, tt.want)
			}
		})
	}
}

func newSimplePointIndex(deepestLevel Level, cellSize float64) *PointIndex {
	deepestSize := mathhelp.Pow2(deepestLevel)
	span := cellSize * float64(deepestSize)
	intExtent := intgeom.Extent{0.0, 0.0, intgeom.FromGeomOrd(span), intgeom.FromGeomOrd(span)}
	ix := PointIndex{
		Quadrant: Quadrant{
			intExtent: intExtent,
		},
		deepestLevel: deepestLevel,
		deepestSize:  deepestSize,
		deepestRes:   intExtent.XSpan() / int64(deepestSize),
		quadrants:    make(map[Level]map[morton.Z]Quadrant, deepestLevel+1),
		hitOnce:      make(map[morton.Z]map[intgeom.Point][]int, 0),
		hitMultiple:  make(map[morton.Z]map[intgeom.Point][]int, 0),
	}
	_, ix.intCentroid = ix.getQuadrantExtentAndCentroid(0, 0, 0, ix.intExtent)
	return &ix
}

func loadEmbeddedTileMatrixSet(t *testing.T, tmsID string) tms20.TileMatrixSet {
	tms, err := tms20.LoadEmbeddedTileMatrixSet(tmsID)
	require.NoError(t, err)
	return tms
}

func newPointIndexFromEmbeddedTileMatrixSet(t *testing.T, tmsID string, deepestTMID tms20.TMID) *PointIndex {
	tms, err := FromTileMatrixSet(loadEmbeddedTileMatrixSet(t, tmsID), deepestTMID)
	require.Nil(t, err)
	return tms
}

func TestIsQuadTree(t *testing.T) {
	tests := []struct {
		name    string
		tms     tms20.TileMatrixSet
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "NetherlandsRDNewQuad",
			tms:     loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			wantErr: assertNoErr,
		}, {
			name:    "WebMercatorQuad",
			tms:     loadEmbeddedTileMatrixSet(t, "WebMercatorQuad"),
			wantErr: assertNoErr,
		}, {
			name:    "EuropeanETRS89_LAEAQuad",
			tms:     loadEmbeddedTileMatrixSet(t, "EuropeanETRS89_LAEAQuad"),
			wantErr: assertNoErr,
		}, {
			name: "GNOSISGlobalGrid",
			tms:  loadEmbeddedTileMatrixSet(t, "GNOSISGlobalGrid"),
			wantErr: func(t assert.TestingT, err error, _ ...any) bool {
				return assert.ErrorContains(t, err, "tile matrix height should be same as width")
			},
		}, {
			name: "LINZAntarticaMapTilegrid",
			tms:  loadEmbeddedTileMatrixSet(t, "LINZAntarticaMapTilegrid"),
			wantErr: func(t assert.TestingT, err error, _ ...any) bool {
				return assert.ErrorContains(t, err, "tile matrix should double in size each level")
			},
		}, {
			name:    "WorldMercatorWGS84Quad",
			tms:     loadEmbeddedTileMatrixSet(t, "WorldMercatorWGS84Quad"),
			wantErr: assertNoErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, IsQuadTree(tt.tms), fmt.Sprintf("IsQuadTree(%v)", tt.tms))
		})
	}
}

func TestDeviationStats(t *testing.T) {
	tests := []struct {
		name                  string
		tms                   tms20.TileMatrixSet
		deepestTMID           tms20.TMID
		wantStats             string
		wantDeviationInUnits  float64
		wantDeviationInPixels float64
		margin                float64
		wantErr               assert.ErrorAssertionFunc
	}{
		{
			name:                  "NetherlandsRDNewQuad",
			tms:                   loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			deepestTMID:           16,
			wantDeviationInUnits:  0,
			wantDeviationInPixels: 0,
			margin:                1e-6, // micrometers
			wantErr:               assertNoErr,
		},
		{
			name:                  "WebMercatorQuad",
			tms:                   loadEmbeddedTileMatrixSet(t, "WebMercatorQuad"),
			deepestTMID:           18,
			wantDeviationInUnits:  0,
			wantDeviationInPixels: 0,
			margin:                1,
			wantErr:               assertNoErr,
		},
		{
			name:                  "WebMercatorQuad starting from 19 has more than 1 pixel deviation ... ;(",
			tms:                   loadEmbeddedTileMatrixSet(t, "WebMercatorQuad"),
			deepestTMID:           19,
			wantDeviationInUnits:  1,
			wantDeviationInPixels: 6,
			margin:                1,
			wantErr:               assertNoErr,
		},
		{
			name:                  "EuropeanETRS89_LAEAQuad",
			tms:                   loadEmbeddedTileMatrixSet(t, "EuropeanETRS89_LAEAQuad"),
			deepestTMID:           15,
			wantDeviationInUnits:  0,
			wantDeviationInPixels: 0,
			margin:                1,
			wantErr:               assertNoErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStats, gotDeviationInUnits, gotDeviationInPixels, err := DeviationStats(tt.tms, tt.deepestTMID)
			if !tt.wantErr(t, err, fmt.Sprintf("DeviationStats(%v, %v)", tt.tms.ID, tt.deepestTMID)) {
				return
			}
			if tt.wantStats != "" {
				assert.Containsf(t, tt.wantStats, gotStats, "DeviationStats(%v, %v)", tt.tms.ID, tt.deepestTMID)
			}
			assert.True(t, mathhelp.FBetweenInc(gotDeviationInUnits, tt.wantDeviationInUnits-tt.margin, tt.wantDeviationInUnits+tt.margin), "DeviationStats(%v, %v)", tt.tms.ID, tt.deepestTMID)
			assert.True(t, mathhelp.FBetweenInc(gotDeviationInPixels, tt.wantDeviationInPixels-tt.margin, tt.wantDeviationInPixels+tt.margin), "DeviationStats(%v, %v)", tt.tms.ID, tt.deepestTMID)
		})
	}
}
