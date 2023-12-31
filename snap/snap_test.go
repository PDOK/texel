package snap

import (
	"testing"

	"github.com/pdok/texel/tms20"

	"github.com/go-spatial/geom/encoding/wkt"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"
)

func Test_snapPolygon(t *testing.T) {
	tests := []struct {
		name    string
		tms     tms20.TileMatrixSet
		tmIDs   []tms20.TMID
		polygon geom.Polygon
		want    map[tms20.TMID]geom.Polygon
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
			want: map[tms20.TMID]geom.Polygon{14: {{
				{117220.2846875, 440135.9021875},
				{117210.7165625, 440135.1015625},
				{117211.1234375, 440130.1009375},
				{117222.2009375, 440131.0065625},
				{117221.9909375, 440133.5134375},
				{117220.4946875, 440133.3821875},
			}}},
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
			want: map[tms20.TMID]geom.Polygon{14: {{
				{110892.9321875, 504407.8196875},
				{110929.4459375, 504407.8196875},
				{110920.0353125, 504436.0778125},
				{110909.4565625, 504436.9046875},
				{110907.6453125, 504428.8065625}, // horizontal line still here
				{110906.8709375, 504428.8065625}, // horizontal line still here
				{110899.1928125, 504431.1559375},
			}}},
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
			want: map[tms20.TMID]geom.Polygon{1: {{
				{0.25, 0.25},
				{15.25, 0.25},
				{15.25, 2.25},
				{2.25, 2.25},
				{0.25, 2.25},
			}}},
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
			want: map[tms20.TMID]geom.Polygon{1: {{
				{0.25, 0.25},
				{15.25, 0.25},
				{15.25, 2.25},
				{2.25, 2.25},
				{0.25, 2.25},
			}}},
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
			want: map[tms20.TMID]geom.Polygon{1: {{
				{0.25, 0.25},
				{15.25, 0.25},
				{15.25, 2.25},
				{7.25, 2.25},
				{2.25, 2.25},
				{0.25, 2.25},
			}}},
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
			want: map[tms20.TMID]geom.Polygon{1: {{
				{0.25, 0.25},
				{2.25, 0.25},
				{15.25, 0.25},
				{15.25, 2.25},
				{7.25, 2.25},
				{2.25, 2.25},
				{0.25, 2.25},
			}}},
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
			want: map[tms20.TMID]geom.Polygon{5: {{
				{69840.8, 445753.12},
				{69840.8, 445712.8},
				{69881.12, 445712.8},
				{69840.8, 445712.8},
			}}},
		},
		{
			name:  "lines and points are filtered out (for now)",
			tms:   loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs: []tms20.TMID{0},
			polygon: geom.Polygon{{
				{90713.55, 530388.466},
				{90741.04, 530328.675},
				{90673.689, 530324.552},
				{90664.068, 530379.532},
			}},
			want: map[tms20.TMID]geom.Polygon{0: nil},
		},
		{
			name:  "ring length < 3 _after_ deduping, also should be filtered out",
			tms:   loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs: []tms20.TMID{0},
			polygon: geom.Polygon{{
				{211124.566, 574932.941},
				{211142.954, 574988.796},
				{211059.858, 574971.321},
				{211163.163, 574994.581},
			}},
			want: map[tms20.TMID]geom.Polygon{0: nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapPolygon(tt.polygon, tt.tms, tt.tmIDs)
			for tmID, wantPoly := range tt.want {
				if !assert.EqualValues(t, wantPoly, got[tmID]) {
					t.Errorf("snapPolygon(...) = %v, want %v", wkt.MustEncode(got[tmID]), wkt.MustEncode(wantPoly))
				}
			}
		})
	}
}
