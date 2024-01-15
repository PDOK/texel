package snap

import (
	"testing"

	"log"

	"github.com/pdok/texel/tms20"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"
)

func Test_snapPolygon(t *testing.T) {
	tests := []struct {
		name    string
		tms     tms20.TileMatrixSet
		tmIDs   []tms20.TMID
		polygon geom.Polygon
		want    map[tms20.TMID][]geom.Polygon
	}{
		{
			name:  "missing corner",
			tms:   loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs: []tms20.TMID{14},
			polygon: geom.Polygon{{
				{117220.282, 440135.898},
				{117210.713, 440135.101},
				{117211.129, 440130.102},
				{117222.198, 440131.000},
				{117221.990, 440133.510},
				{117220.500, 440133.380},
			}},
			want: map[tms20.TMID][]geom.Polygon{14: {{{
				{117220.2846875, 440135.9021875},
				{117210.7165625, 440135.1015625},
				{117211.1234375, 440130.1009375},
				{117222.2009375, 440131.0065625},
				{117221.9909375, 440133.5134375},
				{117220.4946875, 440133.3821875},
			}}}},
		},
		{
			name:  "horizontal line on edge",
			tms:   loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs: []tms20.TMID{14},
			polygon: geom.Polygon{{
				{110899.19100000000617001, 504431.15200000000186265},
				{110906.87099999999918509, 504428.79999999998835847}, // horizontal line between quadrants
				{110907.64400000000023283, 504428.79999999998835847}, // horizontal line between quadrants
				{110909.46300000000337604, 504436.90500000002793968},
				{110920.03599999999278225, 504436.07699999999022111},
				{110929.44700000000011642, 504407.8219999999855645},
				{110892.93099999999685679, 504407.8219999999855645},
			}},
			want: map[tms20.TMID][]geom.Polygon{14: {{{
				{110892.9321875, 504407.8196875},
				{110929.4459375, 504407.8196875},
				{110920.0353125, 504436.0778125},
				{110909.4565625, 504436.9046875},
				{110907.6453125, 504428.8065625}, // horizontal line still here
				{110906.8709375, 504428.8065625}, // horizontal line still here
				{110899.1928125, 504431.1559375},
			}}}},
		},
		{
			name:  "needs deduplication",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{{
				{0.0, 0.0},
				{15.0, 0.0},
				{15.0, 2.0},
				{2.0, 2.0},
				{2.0, 2.1},
				{15.0, 2.1},
				{15.0, 2.2},
				{2.0, 2.2},
				{2.0, 2.3},
				{15.0, 2.3},
				{15.0, 2.4},
				{2.0, 2.4},
				{0.0, 2.4},
			}},
			want: map[tms20.TMID][]geom.Polygon{1: {{{
				{0.25, 0.25},
				{15.25, 0.25},
				{15.25, 2.25},
				{2.25, 2.25},
				{0.25, 2.25},
			}}}},
		},
		{
			name:  "needs deduplication and reversal",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{{
				{0.0, 2.4},
				{2.0, 2.4},
				{15.0, 2.4},
				{15.0, 2.3},
				{2.0, 2.3},
				{2.0, 2.2},
				{15.0, 2.2},
				{15.0, 2.1},
				{2.0, 2.1},
				{2.0, 2.0},
				{15.0, 2.0},
				{15.0, 0.0},
				{0.0, 0.0},
			}},
			want: map[tms20.TMID][]geom.Polygon{1: {{{
				{0.25, 0.25},
				{15.25, 0.25},
				{15.25, 2.25},
				{2.25, 2.25},
				{0.25, 2.25},
			}}}},
		},
		{
			name:  "needs deduplication with one zigzag",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{{
				{0.0, 0.0},
				{15.0, 0.0},
				{15.0, 2.0},
				{7.0, 2.0},
				{2.0, 2.0},
				{2.0, 2.1},
				{7.0, 2.1},
				{15.0, 2.1},
				{15.0, 2.2},
				{7.0, 2.2},
				{2.0, 2.2},
				{2.0, 2.3},
				{7.0, 2.3},
				{15.0, 2.3},
				{15.0, 2.4},
				{7.0, 2.4},
				{2.0, 2.4},
				{0.0, 2.4},
			}},
			want: map[tms20.TMID][]geom.Polygon{1: {{{
				{0.25, 0.25},
				{15.25, 0.25},
				{15.25, 2.25},
				{7.25, 2.25},
				{2.25, 2.25},
				{0.25, 2.25},
			}}}},
		},
		{
			name:  "needs deduplication with more than one zigzag",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{{
				{0.0, 0.0},
				{15.0, 0.0},
				{15.0, 0.3},
				{2.0, 0.3},
				{2.0, 0.4},
				{15, 0.4},
				{15.0, 2.0},
				{7.0, 2.0},
				{2.0, 2.0},
				{2.0, 2.1},
				{7.0, 2.1},
				{15.0, 2.1},
				{15.0, 2.2},
				{7.0, 2.2},
				{2.0, 2.2},
				{2.0, 2.3},
				{7.0, 2.3},
				{15.0, 2.3},
				{15.0, 2.4},
				{7.0, 2.4},
				{2.0, 2.4},
				{0.0, 2.4},
			}},
			want: map[tms20.TMID][]geom.Polygon{1: {{{
				{0.25, 0.25},
				{2.25, 0.25},
				{15.25, 0.25},
				{15.25, 2.25},
				{7.25, 2.25},
				{2.25, 2.25},
				{0.25, 2.25},
			}}}},
		},
		{
			name:  "rightmostLowestPoint is one of the deduped points",
			tms:   loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs: []tms20.TMID{5},
			polygon: geom.Polygon{{
				{69840.279, 445755.872},
				{69842.666, 445755.289},
				{69843.225, 445753.053},
				{69838.492, 445706.609},
				{69839.888, 445711.026},
				{69844.08, 445714.626},
				{69878.365, 445712.156},
				{69879.413, 445710.912},
				{69837.833, 445705.673},
			}},
			want: map[tms20.TMID][]geom.Polygon{5: {
				{
					{{69840.8, 445753.12}, {69840.8, 445712.8}},
				}, {
					{{69840.8, 445712.8}, {69881.12, 445712.8}},
				},
			}},
		},
		{
			name:  "lines and points are not filtered out",
			tms:   loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs: []tms20.TMID{0},
			polygon: geom.Polygon{{
				{90713.55, 530388.466},
				{90741.04, 530328.675},
				{90673.689, 530324.552},
				{90664.068, 530379.532},
			}},
			want: map[tms20.TMID][]geom.Polygon{0: {
				{
					{{90810.56, 530415.04}, {90595.52, 530415.04}},
				},
			}},
		},
		{
			name:  "ring length < 3 _after_ deduping, also not filtered out",
			tms:   loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs: []tms20.TMID{0},
			polygon: geom.Polygon{{
				{211124.566, 574932.941},
				{211142.954, 574988.796},
				{211059.858, 574971.321},
				{211163.163, 574994.581},
			}},
			want: map[tms20.TMID][]geom.Polygon{0: {
				{
					{{211017.92, 574928.32}, {211232.96, 574928.32}},
				},
			}},
		},
		{
			name:  "outer ring only, needs splitting",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{{
				{0.0, 3.0},
				{3.0, 0.0},
				{6.0, 3.0},
				{9.0, 3.0},
				{12.0, 0.0},
				{15.0, 3.0},
				{12.0, 6.0},
				{9.0, 3.0},
				{6.0, 3.0},
				{3.0, 6.0},
			}},
			want: map[tms20.TMID][]geom.Polygon{1: { // 3 separate polygons:
				{ // left wing
					{{0.25, 3.25}, {3.25, 0.25}, {6.25, 3.25}, {3.25, 6.25}},
				},
				{ // right wing
					{{9.25, 3.25}, {12.25, 0.25}, {15.25, 3.25}, {12.25, 6.25}},
				},
				{ // line in between (last)
					{{6.25, 3.25}, {9.25, 3.25}},
				},
			}},
		},
		{
			name:  "outer ring with one inner ring, outer needs splitting",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{
				{
					{0.0, 3.0},
					{3.0, 0.0},
					{6.0, 3.0},
					{9.0, 3.0},
					{12.0, 0.0},
					{15.0, 3.0},
					{12.0, 6.0},
					{9.0, 3.0},
					{6.0, 3.0},
					{3.0, 6.0},
				},
				{
					{2.0, 3.0},
					{3.0, 4.0},
					{4.0, 3.0},
					{3.0, 2.0},
				},
			},
			want: map[tms20.TMID][]geom.Polygon{1: { // 3 separate polygons:
				{ // left wing, including inner ring
					{{0.25, 3.25}, {3.25, 0.25}, {6.25, 3.25}, {3.25, 6.25}},
					{{2.25, 3.25}, {3.25, 4.25}, {4.25, 3.25}, {3.25, 2.25}},
				},
				{ // right wing
					{{9.25, 3.25}, {12.25, 0.25}, {15.25, 3.25}, {12.25, 6.25}},
				},
				{ // line in between (last)
					{{6.25, 3.25}, {9.25, 3.25}},
				},
			}},
		},
		{
			name:  "outer ring with two inner ring, outer needs splitting, inner rings must be matched to new outer rings",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{
				{
					{0.0, 3.0},
					{3.0, 0.0},
					{6.0, 3.0},
					{9.0, 3.0},
					{12.0, 0.0},
					{15.0, 3.0},
					{12.0, 6.0},
					{9.0, 3.0},
					{6.0, 3.0},
					{3.0, 6.0},
				},
				{
					{2.0, 3.0},
					{3.0, 4.0},
					{4.0, 3.0},
					{3.0, 2.0},
				},
				{
					{11.0, 3.0},
					{12.0, 4.0},
					{13.0, 3.0},
					{12.0, 2.0},
				},
			},
			want: map[tms20.TMID][]geom.Polygon{1: { // 3 separate polygons:
				{ // left wing, including inner ring
					{{0.25, 3.25}, {3.25, 0.25}, {6.25, 3.25}, {3.25, 6.25}},
					{{2.25, 3.25}, {3.25, 4.25}, {4.25, 3.25}, {3.25, 2.25}},
				},
				{ // right wing
					{{9.25, 3.25}, {12.25, 0.25}, {15.25, 3.25}, {12.25, 6.25}},
					{{11.25, 3.25}, {12.25, 4.25}, {13.25, 3.25}, {12.25, 2.25}},
				},
				{ // line in between (last)
					{{6.25, 3.25}, {9.25, 3.25}},
				},
			}},
		},
		{
			name:  "outer ring only, needs splitting, expect two lines",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{{
				{0.0, 3.0},
				{3.0, 0.0},
				{6.0, 3.0},
				{9.0, 3.0},
				{12.0, 0.0},
				{15.0, 3.0},
				{12.0, 6.0},
				{12.0, 0.0},
				{9.0, 3.0},
				{6.0, 3.0},
				{3.0, 6.0},
			}},
			want: map[tms20.TMID][]geom.Polygon{1: { // 4 separate polygons:
				{ // left wing
					{{0.25, 3.25}, {3.25, 0.25}, {6.25, 3.25}, {3.25, 6.25}},
				},
				{ // right wing
					{{12.25, 0.25}, {15.25, 3.25}, {12.25, 6.25}},
				},
				{ // line 1
					{{6.25, 3.25}, {9.25, 3.25}},
				},
				{ // line 2
					{{9.25, 3.25}, {12.25, 0.25}},
				},
			}},
		},
		{
			name:  "outer ring with one inner ring, inner needs splitting",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{
				{
					{0.0, 3.0},
					{3.0, 0.0},
					{12.0, 0.0},
					{15.0, 3.0},
					{12.0, 6.0},
					{3.0, 6.0},
				},
				{
					{2.0, 3.0},
					{3.0, 4.0},
					{4.0, 3.0},
					{11.0, 3.0},
					{12.0, 4.0},
					{13.0, 3.0},
					{12.0, 2.0},
					{11.0, 3.0},
					{4.0, 3.0},
					{3.0, 2.0},
				},
			},
			want: map[tms20.TMID][]geom.Polygon{1: { // 2 separate polygons:
				{ // outer ring, including two inner rings
					{{0.25, 3.25}, {3.25, 0.25}, {12.25, 0.25}, {15.25, 3.25}, {12.25, 6.25}, {3.25, 6.25}},
					{{2.25, 3.25}, {3.25, 4.25}, {4.25, 3.25}, {3.25, 2.25}},
					{{11.25, 3.25}, {12.25, 4.25}, {13.25, 3.25}, {12.25, 2.25}},
				},
				{ // line in between inner rings (last)
					{{4.25, 3.25}, {11.25, 3.25}},
				},
			}},
		},
		{
			name:  "outer ring only, with external line",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{
				{
					{0.0, 0.0},
					{2.0, 0.0},
					{2.0, 10.0},
					{2.0, 12.0},
					{2.0, 10.0},
					{0.0, 10.0},
				},
			},
			want: map[tms20.TMID][]geom.Polygon{1: { // 2 separate polygons:
				{ // outer ring
					{{0.25, 0.25}, {2.25, 0.25}, {2.25, 10.25}, {0.25, 10.25}},
				},
				{ // line
					{{2.25, 10.25}, {2.25, 12.25}},
				},
			}},
		},
		{
			name:  "outer ring with 'false' inner rings",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{
				{
					{0.0, 0.0},
					{12.0, 0.0},
					{12.0, 6.0},
					{9.0, 3.0},
					{6.0, 6.0},
					{3.0, 3.0},
					{0.0, 6.0},
					{6.0, 6.0},
					{12.0, 6.0},
					{12.0, 12.0},
					{6.0, 6.0},
					{6.0, 12.0},
					{0.0, 6.0},
				},
			},
			want: map[tms20.TMID][]geom.Polygon{1: { // 1 polygon with 2 inner rings:
				{
					{{0.25, 0.25}, {12.25, 0.25}, {12.25, 6.25}, {12.25, 12.25}, {6.25, 6.25}, {6.25, 12.25}, {0.25, 6.25}},
					{{6.25, 6.25}, {3.25, 3.25}, {0.25, 6.25}},
					{{12.25, 6.25}, {9.25, 3.25}, {6.25, 6.25}},
				},
			}},
		},
		{
			name:  "snapping creates a new inner ring",
			tms:   newSimpleTileMatrixSet(1, 8),
			tmIDs: []tms20.TMID{1},
			polygon: geom.Polygon{
				{
					{0.0, 0.0},
					{15.0, 0.0},
					{15.0, 2.0},
					{2.0, 2.0},
					{2.0, 4.0},
					{5.0, 4.0},
					{5.0, 2.1},
					{12.0, 2.1},
					{12.0, 6.0},
					{0.0, 6.0},
				},
				{
					{7.0, 4.0},
					{10.0, 4.0},
					{10.0, 2.5},
					{7.0, 2.5},
				},
			},
			want: map[tms20.TMID][]geom.Polygon{1: { // 2 separate polygons:
				{ // original outer ring with 2 inner rings
					{{0.25, 0.25}, {15.25, 0.25}, {15.25, 2.25}, {12.25, 2.25}, {12.25, 6.25}, {0.25, 6.25}},
					{{5.25, 2.25}, {2.25, 2.25}, {2.25, 4.25}, {5.25, 4.25}},   // inner ring created by snapping
					{{7.25, 4.25}, {10.25, 4.25}, {10.25, 2.75}, {7.25, 2.75}}, // original inner ring
				},
				{ // self-tangent line split off as outer ring
					{{12.25, 2.25}, {5.25, 2.25}},
				},
			}},
		},
		{
			name:  "splitting of outer and inner ring produces duplicate lines",
			tms:   loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs: []tms20.TMID{5},
			polygon: geom.Polygon{
				{{27435.253, 392410.493}, {27339.366, 392266.876}, {27156.23, 392261.72}, {27150.921, 392265.803}, {27153.05, 392268.68}, {27337.2, 392270.744}, {27431.77, 392409.367}, {27435.253, 392410.493}},
				{{27325.12, 392269.53}, {27157.488, 392265.29}, {27153.165, 392267.869}, {27151.622, 392266.309}, {27156.228, 392262.52}, {27339.052, 392267.615}, {27434.775, 392409.844}, {27338.787, 392271.126}, {27337.382, 392269.953}, {27325.12, 392269.53}},
			},
			want: map[tms20.TMID][]geom.Polygon{5: { // 8 separate polygons:
				{ // outer ring with inner ring
					{{27323.36, 392268.64}, {27155.36, 392268.64}, {27148.64, 392268.64}, {27155.36, 392261.92}},
					{{27323.36, 392268.64}, {27155.36, 392261.92}, {27155.36, 392268.64}},
				},
				{ // line 1
					{{27430.88, 392409.76}, {27336.8, 392268.64}},
				},
				{ // line 2 == line 6 - TODO: remove duplicates
					{{27336.8, 392268.64}, {27323.36, 392268.64}},
				},
				{ // line 3 == line 5 - TODO: remove duplicates
					{{27430.88, 392409.76}, {27437.6, 392409.76}},
				},
				{ // line 4
					{{27336.8, 392268.64}, {27430.88, 392409.76}},
				},
				{ // line 5 == line 3 - TODO: remove duplicates
					{{27430.88, 392409.76}, {27437.6, 392409.76}},
				},
				{ // line 6 == line 2 - TODO: remove duplicates
					{{27336.8, 392268.64}, {27323.36, 392268.64}},
				},
				{ // line 7
					{{27155.36, 392268.64}, {27148.64, 392268.64}},
				},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SnapPolygon(tt.polygon, tt.tms, tt.tmIDs)
			for tmID, wantPoly := range tt.want {
				if !assert.EqualValues(t, wantPoly, got[tmID]) {
					// t.Errorf("snapPolygon(...) = %v, want %v", wkt.MustEncode(got[tmID]), wkt.MustEncode(wantPoly))
					t.Errorf("snapPolygon(...) = %v, want %v", got[tmID], wantPoly)
				}
				log.Printf("snapPolygon(...) = %v", got[tmID])
			}
		})
	}
}

func Test_ringContains(t *testing.T) {
	type args struct {
		ring  [][2]float64
		point [2]float64
	}
	tests := []struct {
		name           string
		args           args
		wantContains   bool
		wantOnBoundary bool
	}{
		{
			name: "fully contained",
			args: args{
				ring:  [][2]float64{{0.25, 3.25}, {3.25, 0.25}, {6.25, 3.25}, {3.25, 6.25}},
				point: [2]float64{2.25, 3.25},
			},
			wantContains:   true,
			wantOnBoundary: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotContains, gotOnBoundary := ringContains(tt.args.ring, tt.args.point)
			assert.Equalf(t, tt.wantContains, gotContains, "ringContains(%v, %v)", tt.args.ring, tt.args.point)
			assert.Equalf(t, tt.wantOnBoundary, gotOnBoundary, "ringContains(%v, %v)", tt.args.ring, tt.args.point)
		})
	}
}
