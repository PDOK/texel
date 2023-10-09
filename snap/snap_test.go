package snap

import (
	"testing"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"
)

func Test_snapPolygon(t *testing.T) {
	tests := []struct {
		name       string
		tileMatrix TileMatrix
		polygon    *geom.Polygon
		want       *geom.Polygon
	}{
		{
			name: "missing corner",
			tileMatrix: TileMatrix{
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
				// {117210.71656250002, 440135.1015625001}, // FIXME dupe, shouldn't be here
				{117211.1234375, 440130.10093750013},     // FIXME currently missing in output
				{117222.20093749999, 440131.00656250014}, // FIXME currently missing in output
				{117221.9909375, 440133.5134375001},
				{117220.4946875, 440133.38218750013},
				// {117220.28468750001, 440135.90218750015}, // FIXME back to start unnecessary
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapPolygon(tt.polygon, tt.tileMatrix)
			if !assert.EqualValues(t, tt.want, got) {
				t.Errorf("snapPolygon(...) = %v, want %v", got, tt.want)
			}
		})
	}
}
