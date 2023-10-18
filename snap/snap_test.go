package snap

import (
	"testing"

	"github.com/go-spatial/geom/encoding/wkt"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"
)

func Test_snapPolygon(t *testing.T) {
	tests := []struct {
		name    string
		matrix  TileMatrix
		polygon *geom.Polygon
		want    *geom.Polygon
	}{
		{
			name:   "missing corner",
			matrix: newNetherlandsRDNewQuadTileMatrix(14),
			polygon: &geom.Polygon{{
				{117220.282, 440135.898},
				{117210.713, 440135.101},
				{117211.129, 440130.102},
				{117222.198, 440131.000},
				{117221.990, 440133.510},
				{117220.500, 440133.380},
			}},
			want: &geom.Polygon{{
				{117220.2846875, 440135.9021875},
				{117210.7165625, 440135.1015625},
				{117211.1234375, 440130.1009375},
				{117222.2009375, 440131.0065625},
				{117221.9909375, 440133.5134375},
				{117220.4946875, 440133.3821875},
			}},
		},
		{
			name:   "horizontal line on edge",
			matrix: newNetherlandsRDNewQuadTileMatrix(14),
			polygon: &geom.Polygon{{
				{110899.19100000000617001, 504431.15200000000186265},
				{110906.87099999999918509, 504428.79999999998835847}, // horizontal line between quadrants
				{110907.64400000000023283, 504428.79999999998835847}, // horizontal line between quadrants
				{110909.46300000000337604, 504436.90500000002793968},
				{110920.03599999999278225, 504436.07699999999022111},
				{110929.44700000000011642, 504407.8219999999855645},
				{110892.93099999999685679, 504407.8219999999855645},
			}},
			want: &geom.Polygon{{
				{110899.1928125, 504431.1559375},
				{110906.8709375, 504428.8065625}, // horizontal line still here
				{110907.6453125, 504428.8065625}, // horizontal line still here
				{110909.4565625, 504436.9046875},
				{110920.0353125, 504436.0778125},
				{110929.4459375, 504407.8196875},
				{110892.9321875, 504407.8196875},
			}},
		},
		{
			name:   "dedupe this",
			matrix: newSimpleTileMatrix(16.0, 5, 0.5),
			polygon: &geom.Polygon{{
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
			want: &geom.Polygon{{
				{0.25, 0.25},
				{15.25, 0.25},
				{15.25, 2.25},
				{2.25, 2.25},
				{0.25, 2.25},
			}},
		},
		{
			name:   "dedupe this longer one",
			matrix: newSimpleTileMatrix(16.0, 5, 0.5),
			polygon: &geom.Polygon{{
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
			want: &geom.Polygon{{
				{0.25, 0.25},
				{15.25, 0.25},
				{15.25, 2.25},
				{7.25, 2.25},
				{2.25, 2.25},
				{0.25, 2.25},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapPolygon(tt.polygon, tt.matrix)
			if !assert.EqualValues(t, tt.want, got) {
				t.Errorf("snapPolygon(...) = %v, want %v", wkt.MustEncode(got), wkt.MustEncode(tt.want))
			}
		})
	}
}
