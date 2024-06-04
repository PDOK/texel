package snap

import (
	"strconv"
	"testing"

	"github.com/pdok/texel/geomhelp"
	"github.com/pdok/texel/mathhelp"
	"github.com/pdok/texel/pointindex"
	"github.com/stretchr/testify/require"

	"github.com/go-spatial/geom/encoding/wkt"

	"github.com/pdok/texel/tms20"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"
)

func TestSnap_snapPolygon(t *testing.T) {
	tests := []struct {
		name      string
		tms       tms20.TileMatrixSet
		tmIDs     []tms20.TMID
		config    Config
		polygon   geom.Polygon
		want      map[tms20.TMID][]geom.Polygon
		wantPanic bool
	}{
		{
			name:   "missing corner",
			tms:    loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:  []tms20.TMID{14},
			config: Config{KeepPointsAndLines: true},
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
			name:   "horizontal line on edge",
			tms:    loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:  []tms20.TMID{14},
			config: Config{KeepPointsAndLines: true},
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
			name:   "needs deduplication",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "needs deduplication and reversal",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "needs deduplication with one zigzag",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "needs deduplication with more than one zigzag",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "rightmostLowestPoint is one of the deduped points",
			tms:    loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:  []tms20.TMID{5},
			config: Config{KeepPointsAndLines: true},
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
					{{69840.8, 445706.08}, {69881.12, 445712.8}, {69840.8, 445712.8}},
				},
				{
					{{69840.8, 445706.08}, {69840.8, 445712.8}},
				},
				{
					{{69840.8, 445712.8}, {69840.8, 445753.12}},
				},
			}},
		},
		{
			name:   "lines and points are not filtered out",
			tms:    loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:  []tms20.TMID{0},
			config: Config{KeepPointsAndLines: true},
			polygon: geom.Polygon{{
				{90713.55, 530388.466},
				{90741.04, 530328.675},
				{90673.689, 530324.552},
				{90664.068, 530379.532},
			}},
			want: map[tms20.TMID][]geom.Polygon{0: {
				{
					{{90595.52, 530415.04}, {90810.56, 530415.04}},
				},
			}},
		},
		{
			name:  "lines and points are filtered out, i.e. keepPointsAndLines = false ",
			tms:   loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs: []tms20.TMID{0},
			polygon: geom.Polygon{{
				{90713.55, 530388.466},
				{90741.04, 530328.675},
				{90673.689, 530324.552},
				{90664.068, 530379.532},
			}},
			want: map[tms20.TMID][]geom.Polygon{},
		},
		{
			name:   "ring length < 3 _after_ deduping, also not filtered out",
			tms:    loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:  []tms20.TMID{0},
			config: Config{KeepPointsAndLines: true},
			polygon: geom.Polygon{{
				{211124.566, 574932.941},
				{211142.954, 574988.796},
				{211059.858, 574971.321},
				{211163.163, 574994.581},
			}},
			want: map[tms20.TMID][]geom.Polygon{0: {
				{
					{{211232.96, 574928.32}, {211017.92, 574928.32}},
				},
			}},
		},
		{
			name:   "outer ring only, needs splitting",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "outer ring with one inner ring, outer needs splitting",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "outer ring with two inner ring, outer needs splitting, inner rings must be matched to new outer rings",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "outer ring only, needs splitting, expect two lines",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "outer ring with one inner ring, inner needs splitting",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "outer ring only, with external line",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "outer ring with 'false' inner rings",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
					{{12.25, 6.25}, {9.25, 3.25}, {6.25, 6.25}},
					{{6.25, 6.25}, {3.25, 3.25}, {0.25, 6.25}},
				},
			}},
		},
		{
			name:   "snapping creates a new inner ring",
			tms:    newSimpleTileMatrixSet(1, 8),
			tmIDs:  []tms20.TMID{1},
			config: Config{KeepPointsAndLines: true},
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
			name:   "splitting of outer and inner ring produces (mirrored) duplicate lines",
			tms:    loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:  []tms20.TMID{5},
			config: Config{KeepPointsAndLines: true},
			polygon: geom.Polygon{
				{{27435.253, 392410.493}, {27339.366, 392266.876}, {27156.23, 392261.72}, {27150.921, 392265.803}, {27153.05, 392268.68}, {27337.2, 392270.744}, {27431.77, 392409.367}, {27435.253, 392410.493}},
				{{27325.12, 392269.53}, {27157.488, 392265.29}, {27153.165, 392267.869}, {27151.622, 392266.309}, {27156.228, 392262.52}, {27339.052, 392267.615}, {27434.775, 392409.844}, {27338.787, 392271.126}, {27337.382, 392269.953}, {27325.12, 392269.53}},
			},
			want: map[tms20.TMID][]geom.Polygon{5: { // 8 separate polygons - TODO: remove (mirrored) duplicates in lines:
				{ // outer ring with inner ring
					{{27323.36, 392268.64}, {27155.36, 392268.64}, {27148.64, 392268.64}, {27155.36, 392261.92}},
					{{27323.36, 392268.64}, {27155.36, 392261.92}, {27155.36, 392268.64}},
				},
				{ // line 1 == line 6 mirrored
					{{27437.6, 392409.76}, {27430.88, 392409.76}},
				},
				{ // line 2 == line 5 mirrored
					{{27430.88, 392409.76}, {27336.8, 392268.64}},
				},
				{ // line 3 == line 4 mirrored
					{{27336.8, 392268.64}, {27323.36, 392268.64}},
				},
				{ // line 4 == line 3 mirrored
					{{27323.36, 392268.64}, {27336.8, 392268.64}},
				},
				{ // line 5 == line 2 mirrored
					{{27336.8, 392268.64}, {27430.88, 392409.76}},
				},
				{ // line 6 == line 1 mirrored
					{{27430.88, 392409.76}, {27437.6, 392409.76}},
				},
				{ // line 7
					{{27155.36, 392268.64}, {27148.64, 392268.64}},
				},
			}},
		},
		{
			name:   "inner but no outer error because of not reversing because of horizontal rightmostlowest",
			tms:    loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:  []tms20.TMID{5},
			config: Config{KeepPointsAndLines: true},
			polygon: geom.Polygon{{
				{139372.972, 527781.838}, {139525.129, 527782.562}, {139525.711, 527782.368}, {139526.378, 527781.182}, {139526.322, 527780.127}, {139525.935, 527779.501}, {139524.587, 527779.117}, {139519.249, 527779.07}, {139518.262, 527778.621}, {139516.868, 527776.991}, {139517.249, 527776.042}, {139521.038, 527771.641}, {139522.566, 527768.414}, {139518.146, 527765.041}, {139517.85, 527763.597}, {139518.704, 527762.318}, {139524.008, 527757.186}, {139525.037, 527756.403}, {139526.108, 527756.329}, {139527.952, 527757.006}, {139531.819, 527761.335}, {139535.209, 527761.176}, {139536.704, 527762.265}, {139535.71, 527772.004}, {139535.828, 527773.351}, {139537.605, 527775.971}, {139537.763, 527777.456}, {139536.555, 527779.007}, {139534.715, 527779.837}, {139532.606, 527779.984}, {139529.678, 527779.502}, {139528.964, 527779.875}, {139528.698, 527781.132}, {139529.076, 527782.085}, {139544.208, 527783.211}, {139589.555, 527786.608}, {139594.598, 527787.584}, {139609.462, 527788.495}, {139611.557, 527788.435}, {139649.733, 527791.455}, {139650.255, 527791.973}, {139654.415, 527792.496}, {139655.978, 527793.08}, {139670.378, 527794.059}, {139712.26, 527794.864}, {139729.764, 527794.755}, {139757.57, 527795.124}, {139787.205, 527793.835}, {139816.936, 527791.996}, {139824.33, 527791.319298176}, {139824.33, 527756.249181541}, {139806.072, 527754.485}, {139709.471, 527743.599}, {139682.187, 527741.701}, {139628.039, 527739.25}, // something with an inlet that becomes an inner ring
				{139566.36, 527736.9}, {139433.603, 527736.172}, {139364.846, 527736.093}, // horizontal with rightmostlowest
				{139348.24, 527736.425675193}, {139348.24, 527754.592389722}, // (vertical)
			}},
			want: map[tms20.TMID][]geom.Polygon{5: {
				{{{139345.76, 527757.28}, {139345.76, 527737.12}, {139365.92, 527737.12}, {139433.12, 527737.12}, {139567.52, 527737.12}, {139628, 527737.12}, {139681.76, 527743.84}, {139708.64, 527743.84}, {139802.72, 527757.28}, {139822.88, 527757.28}, {139822.88, 527790.88}, {139816.16, 527790.88}, {139789.28, 527790.88}, {139755.68, 527797.6}, {139728.8, 527797.6}, {139715.36, 527797.6}, {139668.32, 527790.88}, {139654.88, 527790.88}, {139648.16, 527790.88}, {139614.56, 527790.88}, {139607.84, 527790.88}, {139594.4, 527790.88}, {139587.68, 527784.16}, {139547.36, 527784.16}, {139527.2, 527784.16}, {139372.64, 527784.16}}, {{139527.2, 527777.44}, {139533.92, 527777.44}, {139533.92, 527770.72}, {139533.92, 527764}, {139527.2, 527757.28}, {139520.48, 527764}, {139520.48, 527770.72}, {139520.48, 527777.44}}},
				geom.Polygon{{{139527.2, 527784.16}, {139527.2, 527777.44}}},
				geom.Polygon{{{139533.92, 527777.44}, {139540.64, 527777.44}}},
				geom.Polygon{{{139520.48, 527777.44}, {139513.76, 527777.44}}},
			}}, // want no panicInnerRingsButNoOuterRings
		},
		{
			name:   "inner but no outer error because of not reversing because of a very sharp leg/extension",
			tms:    loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:  []tms20.TMID{0},
			config: Config{KeepPointsAndLines: true},
			polygon: geom.Polygon{{
				{48158.204, 392310.062},
				{47753.125, 391885.44}, {48565.4, 391515.876}, {47751.195, 391884.821}, // a very sharp leg/extension
				{47677.592, 392079.403},
			}},
			want: map[tms20.TMID][]geom.Polygon{0: {
				{{{47587.52, 392144.32}, {47802.56, 391929.28}, {48232.64, 392359.36}}}, // turned counterclockwise
				{{{47802.56, 391929.28}, {48662.72, 391499.2}}},
			}}, // want no panicInnerRingsButNoOuterRings
		},
		{
			name:    "split ring from outer is cw, should be ccw",
			tms:     loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:   []tms20.TMID{0},
			config:  Config{KeepPointsAndLines: true},
			polygon: geom.Polygon{{{179334.089, 408229.072}, {179121.631, 408528.181}, {179328.228, 408231.924}, {178889.903, 408431.167}, {178531.386, 408106.618}, {178497.492, 407886.329}, {178535.353, 408103.574}, {178862.244, 408226.852}, {178891.816, 408426.547}, {179173.349, 408187.199}, {178893.957, 408423.424}, {178864.491, 408223.293}, {178537.744, 408101.003}, {178504.209, 407887.598}, {178510.008, 407890.491}, {178542.44, 408098.473}, {178867.788, 408219.534}, {178897.835, 408417.763}, {179170.131, 408181.285}}}, // ccw (with a sharp leg/extension also)
			want: map[tms20.TMID][]geom.Polygon{0: {
				{{{178976.96, 408487.36}, {178761.92, 408272.32}, {178546.88, 408057.28}, {179192, 408272.32}}}, // ccw
				// bunch of points and lines:
				{{{179407.04, 408272.32}, {179192, 408272.32}}}, {{{179192, 408272.32}, {179192, 408487.36}}}, {{{179407.04, 408272.32}, {179192, 408272.32}}}, {{{179192, 408272.32}, {178976.96, 408487.36}}}, {{{178976.96, 408487.36}, {178761.92, 408272.32}}}, {{{178761.92, 408272.32}, {178546.88, 408057.28}}}, {{{178546.88, 408057.28}, {178546.88, 407842.24}}},
			}},
		},
		{
			name:    "one of three split outer rings is cw and turned outer after no matching outer",
			tms:     loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:   []tms20.TMID{0},
			config:  Config{KeepPointsAndLines: true},
			polygon: geom.Polygon{{{88580.011, 439678.996}, {88337.73, 439237.216}, {89273.964, 438026.4}, {89386.079, 438023.335}, {90251.524, 438784.15}, {89852.567, 439284.421}, {89425.263, 439355.284}, {89247.228, 439563.507}, {89089.95, 439692.364}, {88959.832, 439729.531}, {89055.886, 439819.684}, {89466.904, 439382.346}, {89899.488, 439311.969}, {90170.183, 438911.775}, {90329.354, 438821.391}, {90651.094, 438796.963}, {91473.854, 439243.296}, {90632.307, 438747.518}, {90270.708, 438757.632}, {89555.357, 437677.283}, {90499.163, 436096.427}, {91435.651, 435963.019}, {91404.334, 436039.088}, {91254.337, 436091.084}, {90500.745, 436098.362}, {90076.214, 437042.706}, {89870.055, 437307.816}, {89768.94, 437363.42}, {89650.683, 437521.434}, {89640.994, 437568.838}, {89558.222, 437677.647}, {90269.467, 438753.387}, {90632.85, 438744.94}, {91313.174, 439143.369}, {91477.748, 439241.657}, {91475.353, 439245.66}, {91457.592, 439266.852}, {91243.008, 439179.921}, {90710.843, 438897.924}, {90650.175, 438799.288}, {90440.729, 438846.985}, {90395.019, 438846.967}, {90329.938, 438823.822}, {90287.474, 438885.328}, {90172.086, 438913.396}, {90044.257, 439125.421}, {89901.052, 439313.924}, {89885.113, 439321.991}, {89835.824, 439335.083}, {89468.228, 439384.467}, {89173.832, 439758.873}, {89061.413, 439821.909}, {89054.68, 439821.883}, {89023.222, 439801.24}, {88989.659, 439763.597}, {88949.781, 439739.428}, {88958.959, 439726.203}, {89088.39, 439690.41}, {89245.45, 439561.75}, {89388.248, 439376.01}, {89424.081, 439353.075}, {89566.906, 439317.631}, {89851.03, 439282.45}, {90111.766, 438914.525}, {90249.027, 438784.029}, {90211.25, 438760.51}, {90183.492, 438736.293}, {89584.683, 438207.656}, {89384.579, 438025.335}, {89274.819, 438028.749}, {88339.974, 439238.317}, {88419.861, 439377.057}, {88447.454, 439387.602}, {88485.231, 439376.209}, {88505.9, 439379.802}, {88564.366, 439441.722}, {88589.428, 439478.721}, {88598.844, 439504.106}, {88608.517, 439561.563}, {88582.418, 439679.669}, {88565.692, 439724.97}, {88480.367, 439857.335}, {88409.981, 439938.527}, {88412.431, 439940.265}, {88366.171, 440033.682}, {88353.723, 440046.457}, {88356.08, 440054.25}, {88342.856, 440086.861}, {88266.552, 440224.799}, {88252.681, 440243.646}, {88196.44, 440306.135}, {87992.789, 440467.453}, {88250.595, 440274.14}, {88508.083, 439845.775}, {88270.249, 440256.888}, {88194.893, 440335.659}, {88010.485, 440474.349}, {87996.213, 440475.679}, {87990.894, 440469.07}, {88580.011, 439678.996}}},
			want:    map[tms20.TMID][]geom.Polygon{}, // want no panicNoMatchingOuterForInnerRing
		},
		{
			name:   "sneaky nested pseudo ring creates more than 1 matching outer ring",
			tms:    loadEmbeddedTileMatrixSet(t, "NetherlandsRDNewQuad"),
			tmIDs:  []tms20.TMID{0},
			config: Config{KeepPointsAndLines: true},
			polygon: geom.Polygon{
				{{198877.1, 506188.635}, {198805.608, 506361.231}, {198633.011, 506432.722}, {198460.415, 506361.23}, {198388.924, 506188.633}, {198460.416, 506016.037}, {198633.013, 505944.546}, {198805.609, 506016.038}},
				{{198429.407, 506188.635}, {198489.229, 506332.782}, {198633.531, 506392.228}, {198777.528, 506332.045}, {198836.612, 506187.594}, {198776.434, 506044.111}, {198632.5, 505985.022}, {198488.864, 506044.832}, {198429.407, 506188.615}, {198551.204, 506045.823}, {198690.244, 506034.324}, {198792.36, 506147.487}, {198748.509, 506305.863}, {198576.128, 506343.056}},
				{{198633.012, 506279.536}, {198766.685, 506188.158}, {198632.396, 506055.195}, {198499.739, 506188.974}}},
			want: map[tms20.TMID][]geom.Polygon{}, // want no panicMoreThanOneMatchingOuterRing
		},
		{
			name: "nested rings",
			tms:  newSimpleTileMatrixSet(2, 64),
			tmIDs: []tms20.TMID{
				1, // 32 * 8.0
			},
			config: Config{KeepPointsAndLines: true},
			polygon: geom.Polygon{
				{{4.0, 124.0}, {4.0, 4.0}, {60.0, 4.0}, {60.0, 124.0}}, // big outer
				{{12.0, 52.0}, {12.0, 12.0}, {52.0, 12.0}, {52.0, 52.0}, {30.0, 52.0}, {30.0, 44.0}, {44.0, 44.0}, {44.0, 20.0}, {20.0, 20.0}, {20.0, 44.0}, {27.0, 44.0}, {27.0, 52.0}},     // big letter C that turns into nested rings when snapped
				{{12.0, 116.0}, {52.0, 116.0}, {52.0, 76.0}, {30.0, 76.0}, {30.0, 84.0}, {44.0, 84.0}, {44.0, 108.0}, {20.0, 108.0}, {20.0, 84.0}, {27.0, 84.0}, {27.0, 76.0}, {12.0, 76.0}}, // above mirrored vertically
				{{30.0, 53.0}, {30.0, 54.0}, {54.0, 54.0}, {54.0, 10.0}, {10.0, 10.0}, {10.0, 54.0}, {27.0, 54.0}, {27.0, 53.0}, {11.0, 53.0}, {11.0, 11.0}, {53.0, 11.0}, {53.0, 53.0}},     // another C around the original C which snaps to a duplicate outer
				{{28.0, 28.0}, {36.0, 28.0}, {36.0, 36.0}, {29.0, 36.0}, {29.0, 92.0}, {36.0, 92.0}, {36.0, 100.0}, {28.0, 100.0}},                                                           // dumbbell inside the two C's that also turns into a nested ring when snapped (and some lines)
			},
			want: map[tms20.TMID][]geom.Polygon{
				// 1 big outer with 2 holes. and 2 new outers/polygons (inside those holes from the big outer) each with their own hole.
				// a duplicate inner/outer pair from the extra C is removed
				1: {
					{
						{{4.0, 124.0}, {4.0, 4.0}, {60.0, 4.0}, {60.0, 124.0}},                   // ccw
						{{12.0, 116.0}, {52.0, 116.0}, {52.0, 76.0}, {28.0, 76.0}, {12.0, 76.0}}, // cw
						{{28.0, 52.0}, {52.0, 52.0}, {52.0, 12.0}, {12.0, 12.0}, {12.0, 52.0}},   // cw
					}, {
						{{28.0, 44.0}, {20.0, 44.0}, {20.0, 20.0}, {44.0, 20.0}, {44.0, 44.0}}, // ccw
						{{28.0, 36.0}, {36.0, 36.0}, {36.0, 28.0}, {28.0, 28.0}},               // cw
					}, {
						{{28.0, 84.0}, {44.0, 84.0}, {44.0, 108.0}, {20.0, 108.0}, {20.0, 84.0}}, // ccw
						{{28.0, 100.0}, {36.0, 100.0}, {36.0, 92.0}, {28.0, 92.0}},               // cw
					},
					// and some lines
					{{{28.0, 52.0}, {28.0, 44.0}}},
					{{{28.0, 76.0}, {28.0, 84.0}}},
					{{{28.0, 92.0}, {28.0, 84.0}}},
					{{{28.0, 84.0}, {28.0, 76.0}}},
					{{{28.0, 76.0}, {28.0, 52.0}}},
					{{{28.0, 52.0}, {28.0, 44.0}}},
					{{{28.0, 44.0}, {28.0, 36.0}}},
				},
			},
		},
		{
			name:   "no points found panic on TMS other than RD",
			tms:    loadEmbeddedTileMatrixSet(t, "WebMercatorQuad"),
			tmIDs:  []tms20.TMID{17},
			config: Config{KeepPointsAndLines: true},
			polygon: geom.Polygon{
				{{642743.3299, 6898063.027}, {642694.6797, 6898049.319}, {642671.3143, 6898042.735}, {642671.3143, 6898042.735}, {642668.1822, 6898053.868}, {642740.1897, 6898074.148}},
			},
			want: map[tms20.TMID][]geom.Polygon{}, // want no panicNoPointsFoundForVertices
		},
		{
			name:      "panic outside grid",
			tms:       newSimpleTileMatrixSet(0, 1),
			tmIDs:     []tms20.TMID{0},
			polygon:   geom.Polygon{{{0.1, 0.1}, {0.2, 0.1}, {0.2, -0.1}}},
			want:      map[tms20.TMID][]geom.Polygon{}, // empty, ignored
			wantPanic: true,
		},
		{
			name:    "ignore outside grid",
			tms:     newSimpleTileMatrixSet(0, 1),
			config:  Config{IgnoreOutsideGrid: true},
			tmIDs:   []tms20.TMID{0},
			polygon: geom.Polygon{{{0.1, 0.1}, {0.2, 0.1}, {0.2, -0.1}}},
			want:    map[tms20.TMID][]geom.Polygon{}, // empty, ignored
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				require.Panics(t, func() {
					SnapPolygon(tt.polygon, tt.tms, tt.tmIDs, tt.config)
				})
				return
			}
			got := SnapPolygon(tt.polygon, tt.tms, tt.tmIDs, tt.config)
			for tmID, wantPoly := range tt.want {
				if !assert.EqualValues(t, wantPoly, got[tmID]) {
					t.Errorf("snapPolygon(%v, _, %v)\n=     %v\nwant: %v",
						wkt.MustEncode(tt.polygon), tmID, geomhelp.WktMustEncodeSlice(got[tmID], 0), geomhelp.WktMustEncodeSlice(wantPoly, 0))
				}
			}
		})
	}
}

func TestSnap_ringContains(t *testing.T) {
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

func Test_kmpDeduplicate(t *testing.T) {
	tests := []struct {
		name string
		ring [][2]float64
		want [][2]float64
	}{
		{
			name: "triangle should stay",
			ring: [][2]float64{
				{2, 1}, // A
				{1, 1}, // B
				{1, 0}, // C
				{1, 1}, // B
				{0, 1}, // D
				{1, 0}, // C
				{1, 1}, // B
			},
			want: [][2]float64{
				{2, 1}, // A
				{1, 1}, // B
				{0, 1}, // D
				{1, 0}, // C
				{1, 1}, // B
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, kmpDeduplicate(tt.ring), "kmpDeduplicate(%v)", tt.ring)
		})
	}
}

func Test_dedupeInnersOuters(t *testing.T) {
	type args struct {
		outers [][][2]float64
		inners [][][2]float64
	}
	tests := []struct {
		name       string
		args       args
		wantOuters [][][2]float64
		wantInners [][][2]float64
	}{
		{
			name: "#outer, #inner = 0, 0",
			args: args{
				outers: squareRingArray(0, true),
				inners: squareRingArray(0, false),
			},
			wantOuters: squareRingArray(0, true),
			wantInners: squareRingArray(0, false),
		},
		{
			name: "#outer, #inner = 1, 0",
			args: args{
				outers: squareRingArray(1, true),
				inners: squareRingArray(0, false),
			},
			wantOuters: squareRingArray(1, true),
			wantInners: squareRingArray(0, false),
		},
		{
			name: "#outer, #inner = 1, 1",
			args: args{
				outers: squareRingArray(1, true),
				inners: squareRingArray(1, false),
			},
			wantOuters: squareRingArray(1, true),
			wantInners: squareRingArray(1, false),
		},
		{
			name: "#outer, #inner = 2, 1",
			args: args{
				outers: squareRingArray(2, true),
				inners: squareRingArray(1, false),
			},
			wantOuters: squareRingArray(1, true),
			wantInners: squareRingArray(0, false),
		},
		{
			name: "#outer, #inner = 2, 2",
			args: args{
				outers: squareRingArray(2, true),
				inners: squareRingArray(2, false),
			},
			wantOuters: squareRingArray(1, true),
			wantInners: squareRingArray(1, false),
		},
		{
			name: "#outer, #inner = 0, 1",
			args: args{
				outers: squareRingArray(0, true),
				inners: squareRingArray(1, false),
			},
			wantOuters: squareRingArray(0, true),
			wantInners: squareRingArray(1, false),
		},
		{
			name: "#outer, #inner = 1, 2",
			args: args{
				outers: squareRingArray(1, true),
				inners: squareRingArray(2, false),
			},
			wantOuters: squareRingArray(0, true),
			wantInners: squareRingArray(1, false),
		},
		{
			name: "#outer, #inner = 2, 0",
			args: args{
				outers: squareRingArray(2, true),
				inners: squareRingArray(0, false),
			},
			wantOuters: squareRingArray(2, true),
			wantInners: squareRingArray(0, false),
		},
		{
			name: "#outer, #inner = 0, 2",
			args: args{
				outers: squareRingArray(0, true),
				inners: squareRingArray(2, false),
			},
			wantOuters: squareRingArray(0, true),
			wantInners: squareRingArray(2, false),
		},
		{
			name: "#outer, #inner = 3, 1",
			args: args{
				outers: squareRingArray(3, true),
				inners: squareRingArray(1, false),
			},
			wantOuters: squareRingArray(2, true),
			wantInners: squareRingArray(0, false),
		},
		{
			name: "#outer, #inner = 1, 3",
			args: args{
				outers: squareRingArray(1, true),
				inners: squareRingArray(3, false),
			},
			wantOuters: squareRingArray(0, true),
			wantInners: squareRingArray(2, false),
		},
		// add different ring, which should be left alone
		{
			name: "#outer, #inner = 1, 1 + dummy ring",
			args: args{
				outers: squareRingArray(1, true),
				inners: append(squareRingArray(1, false), [][2]float64{{0, 0}, {1, 0}, {2, 1}}),
			},
			wantOuters: squareRingArray(1, true),
			wantInners: append(squareRingArray(1, false), [][2]float64{{0, 0}, {1, 0}, {2, 1}}),
		},
		{
			name: "#outer, #inner = 3, 1 + dummy ring",
			args: args{
				outers: squareRingArray(3, true),
				inners: append(squareRingArray(1, false), [][2]float64{{0, 0}, {1, 0}, {2, 1}}),
			},
			wantOuters: squareRingArray(2, true),
			wantInners: append(squareRingArray(0, false), [][2]float64{{0, 0}, {1, 0}, {2, 1}}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := dedupeInnersOuters(tt.args.outers, tt.args.inners)
			assert.Equalf(t, tt.wantOuters, got, "dedupeInnersOuters(%v, %v)", tt.args.outers, tt.args.inners)
			assert.Equalf(t, tt.wantInners, got1, "dedupeInnersOuters(%v, %v)", tt.args.outers, tt.args.inners)
		})
	}
}

// newSimpleTileMatrixSet creates a tms for snap testing purposes
// the effective quadrant amount (one axis) on the deepest level will be 2^deepestLevel * 16 (vt internal pixel res)
// the effective quadrant size (one axis) on the deepest level will be cellSize / 16 (vt internal pixel res)
func newSimpleTileMatrixSet(deepestTMID pointindex.Level, cellSize float64) tms20.TileMatrixSet {
	zeroZero := tms20.TwoDPoint([2]float64{0.0, 0.0})
	tms := tms20.TileMatrixSet{
		CRS:          fakeCRS{},
		OrderedAxes:  []string{"X", "Y"},
		TileMatrices: make(map[tms20.TMID]tms20.TileMatrix, deepestTMID+1),
	}
	for tmID := 0; tmID <= int(deepestTMID); tmID++ {
		// (only values from the root tm are used, for the rest it is assumed to follow quad matrix rules)
		tmCellSize := cellSize * float64(mathhelp.Pow2(deepestTMID-uint(tmID)))
		tms.TileMatrices[tmID] = tms20.TileMatrix{
			ID:               strconv.Itoa(tmID),
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
	return tms
}

func loadEmbeddedTileMatrixSet(t *testing.T, tmsID string) tms20.TileMatrixSet {
	tms, err := tms20.LoadEmbeddedTileMatrixSet(tmsID)
	require.NoError(t, err)
	return tms
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

func squareRingArray(number int, isOuter bool) [][][2]float64 {
	outerSquare := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}} // square, counter clockwise
	innerSquare := [][2]float64{{0, 0}, {0, 1}, {1, 1}, {1, 0}} // square, clockwise
	var squares = [][][2]float64{}
	var square [][2]float64
	// outer or inner
	if isOuter {
		square = outerSquare
	} else {
		square = innerSquare
	}
	// add squares
	for i := 0; i < number; i++ {
		squares = append(squares, square)
	}
	return squares
}
