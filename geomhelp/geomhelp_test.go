package geomhelp

import (
	"testing"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"
)

func TestGeomhelp_PolygonSliceToGeom(t *testing.T) {
	tests := []struct {
		name         string
		polygonSlice []geom.Polygon
		want         geom.Geometry
		wantPanic    bool
	}{
		{
			name:         "Empty polygon slice",
			polygonSlice: []geom.Polygon{},
			wantPanic:    true,
		},
		{
			name:         "Single polygon",
			polygonSlice: []geom.Polygon{{{{0.0, 0.0}, {1.0, 0.0}, {1.0, 1.0}}}},
			want:         geom.Polygon{{{0.0, 0.0}, {1.0, 0.0}, {1.0, 1.0}}},
		},
		{
			name: "Two polygons",
			polygonSlice: []geom.Polygon{
				{{{0.0, 0.0}, {1.0, 0.0}, {1.0, 1.0}}},
				{{{10.0, 10.0}, {10.0, 11.0}, {11.0, 11.0}}},
			},
			want: geom.MultiPolygon{
				{{{0.0, 0.0}, {1.0, 0.0}, {1.0, 1.0}}},
				{{{10.0, 10.0}, {10.0, 11.0}, {11.0, 11.0}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.Panics(t, func() { PolygonSliceToGeom(tt.polygonSlice) })
				return
			}
			got := PolygonSliceToGeom(tt.polygonSlice)
			assert.Equal(t, tt.want, got)
		})
	}
}
