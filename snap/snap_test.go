package snap

import (
	"testing"

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
			name: "missing corner",
			matrix: TileMatrix{
				MinX:      -285401.92,
				MaxY:      903401.92,
				PixelSize: 16,
				TileSize:  256,
				Level:     14,
				CellSize:  0.21,
			},
			polygon: &geom.Polygon{{
				{117220.282, 440135.898},
				{117210.713, 440135.101},
				{117211.129, 440130.102},
				{117222.198, 440131.000},
				{117221.990, 440133.510},
				{117220.500, 440133.380},
			}},
			want: &geom.Polygon{{
				{117220.28468750001, 440135.90218750015},
				{117210.71656250002, 440135.1015625001},
				{117211.1234375, 440130.10093750013},
				{117222.20093749999, 440131.00656250014},
				{117221.9909375, 440133.5134375001},
				{117220.4946875, 440133.38218750013},
			}},
		},
		{
			name: "horizontal line",
			matrix: TileMatrix{
				MinX:      -285401.92,
				MaxY:      903401.92,
				PixelSize: 16,
				TileSize:  256,
				Level:     14,
				CellSize:  0.21,
			},
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
				{110906.87093749997, 504428.8065625}, // horizontal line still here
				{110907.64531250001, 504428.8065625}, // horizontal line still here
				{110909.45656249998, 504436.9046875},
				{110920.03531250003, 504436.0778125},
				{110929.44593749999, 504407.8196875},
				{110892.9321875, 504407.8196875},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapPolygon(tt.polygon, tt.matrix)
			if !assert.EqualValues(t, tt.want, got) {
				t.Errorf("snapPolygon(...) = %v, want %v", got, tt.want)
			}
		})
	}
}
