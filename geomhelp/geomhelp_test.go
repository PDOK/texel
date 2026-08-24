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

func TestMergeGeometries(t *testing.T) {
	p1 := geom.Point{1, 2}
	p2 := geom.Point{3, 4}
	mp1 := geom.MultiPoint{{5, 6}, {7, 8}}
	mp2 := geom.MultiPoint{{9, 9}}

	l1 := geom.LineString{{0, 0}, {1, 1}}
	l2 := geom.LineString{{2, 2}, {3, 3}}
	ml1 := geom.MultiLineString{{{4, 4}, {5, 5}}}
	ml2 := geom.MultiLineString{{{6, 6}, {7, 7}}}

	poly1 := geom.Polygon{{{0, 0}, {1, 0}, {1, 1}}}
	poly2 := geom.Polygon{{{2, 2}, {3, 2}, {3, 3}}}
	mpoly1 := geom.MultiPolygon{{{{4, 4}, {5, 4}, {5, 5}}}}
	mpoly2 := geom.MultiPolygon{{{{6, 6}, {7, 6}, {7, 7}}}}

	tests := []struct {
		name string
		g, h geom.Geometry
		want geom.Geometry
	}{
		{name: "nil g", g: nil, h: p1, want: p1},
		{name: "nil h", g: p1, h: nil, want: p1},

		{name: "point + point", g: p1, h: p2, want: geom.MultiPoint{p1, p2}},
		{name: "point + multipoint", g: p1, h: mp1, want: geom.MultiPoint{{1, 2}, {5, 6}, {7, 8}}},
		{name: "multipoint + point", g: mp1, h: p1, want: geom.MultiPoint{{5, 6}, {7, 8}, {1, 2}}},
		{name: "multipoint + multipoint", g: mp1, h: mp2, want: geom.MultiPoint{{5, 6}, {7, 8}, {9, 9}}},

		{name: "linestring + linestring", g: l1, h: l2, want: geom.MultiLineString{l1, l2}},
		{
			name: "linestring + multilinestring", g: l1, h: ml1,
			want: geom.MultiLineString{{{0, 0}, {1, 1}}, {{4, 4}, {5, 5}}},
		},
		{
			name: "multilinestring + linestring", g: ml1, h: l1,
			want: geom.MultiLineString{{{4, 4}, {5, 5}}, {{0, 0}, {1, 1}}},
		},
		{
			name: "multilinestring + multilinestring", g: ml1, h: ml2,
			want: geom.MultiLineString{{{4, 4}, {5, 5}}, {{6, 6}, {7, 7}}},
		},

		{name: "polygon + polygon", g: poly1, h: poly2, want: geom.MultiPolygon{poly1, poly2}},
		{
			name: "polygon + multipolygon", g: poly1, h: mpoly1,
			want: geom.MultiPolygon{{{{0, 0}, {1, 0}, {1, 1}}}, {{{4, 4}, {5, 4}, {5, 5}}}},
		},
		{
			name: "multipolygon + polygon", g: mpoly1, h: poly1,
			want: geom.MultiPolygon{{{{4, 4}, {5, 4}, {5, 5}}}, {{{0, 0}, {1, 0}, {1, 1}}}},
		},
		{
			name: "multipolygon + multipolygon", g: mpoly1, h: mpoly2,
			want: geom.MultiPolygon{{{{4, 4}, {5, 4}, {5, 5}}}, {{{6, 6}, {7, 6}, {7, 7}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeGeometries(tt.g, tt.h)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMergeGeometriesPanicsOnMismatchedTypes(t *testing.T) {
	tests := []struct {
		name string
		g, h geom.Geometry
	}{
		{name: "point with linestring", g: geom.Point{1, 2}, h: geom.LineString{{0, 0}, {1, 1}}},
		{name: "linestring with polygon", g: geom.LineString{{0, 0}, {1, 1}}, h: geom.Polygon{{{0, 0}, {1, 0}, {1, 1}}}},
		{name: "polygon with point", g: geom.Polygon{{{0, 0}, {1, 0}, {1, 1}}}, h: geom.Point{1, 2}},
		{name: "unknown geometry types", g: 42, h: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() { MergeGeometries(tt.g, tt.h) })
		})
	}
}

func TestPointSlice(t *testing.T) {
	tests := []struct {
		name string
		g    geom.Geometry
		want []geom.Point
	}{
		{
			name: "point",
			g:    geom.Point{1, 2},
			want: []geom.Point{{1, 2}},
		},
		{
			name: "line string",
			g:    geom.LineString{{0, 0}, {1, 1}, {2, 2}},
			want: []geom.Point{{0, 0}, {1, 1}, {2, 2}},
		},
		{
			name: "polygon with single ring",
			g:    geom.Polygon{{{0, 0}, {1, 0}, {1, 1}, {0, 1}}},
			want: []geom.Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
		},
		{
			name: "polygon with hole",
			g: geom.Polygon{
				{{0, 0}, {10, 0}, {10, 10}, {0, 10}},
				{{2, 2}, {4, 2}, {4, 4}, {2, 4}},
			},
			want: []geom.Point{
				{0, 0},
				{10, 0},
				{10, 10},
				{0, 10},
				{2, 2},
				{4, 2},
				{4, 4},
				{2, 4},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PointSlice(tt.g)
			assert.Equal(t, tt.want, got)
		})
	}
}
