package processing

import (
	"reflect"
	"testing"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/tile"
	"github.com/pdok/texel/tms20"
)

// TestProcessGeometry covers processGeometry and processMultiPolygon
// Uses a stub processPolygonFunc
func TestProcessGeometry(t *testing.T) {
	polyA := geom.Polygon{{{0, 0}, {1, 0}, {1, 1}, {0, 0}}}
	polyB := geom.Polygon{{{2, 2}, {3, 2}, {3, 3}, {2, 2}}}
	tileA := tile.Tile{}
	tileB := tile.Tile{}

	tests := []struct {
		name string
		// geometry passed to processGeometry
		geometry geom.Geometry
		tmIDs    []tms20.TMID
		// callResults is a list of return values of the stub function in
		// call order.
		callResults []map[tms20.TMID]SnapResult
		want        map[tms20.TMID]SnapResult
	}{
		{
			name:     "single Polygon, single tmID: passthrough of f's result",
			geometry: polyA,
			tmIDs:    []tms20.TMID{1},
			callResults: []map[tms20.TMID]SnapResult{
				{1: {Geometry: polyA, Tiles: []tile.Tile{tileA}}},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: polyA, Tiles: []tile.Tile{tileA}},
			},
		},
		{
			name:        "unsupported geometry type (Point): default branch, f not called",
			geometry:    geom.Point{9, 9},
			tmIDs:       []tms20.TMID{1, 2},
			callResults: nil,
			want: map[tms20.TMID]SnapResult{
				1: {},
				2: {},
			},
		},
		{
			name:        "nil geometry value: default branch, f not called",
			geometry:    nil,
			tmIDs:       []tms20.TMID{1},
			callResults: nil,
			want: map[tms20.TMID]SnapResult{
				1: {},
			},
		},
		{
			name:        "empty MultiPolygon (len 0, non-nil): f never called, empty result map",
			geometry:    geom.MultiPolygon{},
			tmIDs:       []tms20.TMID{1, 2},
			callResults: nil,
			want:        map[tms20.TMID]SnapResult{},
		},
		{
			name:     "MultiPolygon with 1 polygon, single tmID: passthrough",
			geometry: geom.MultiPolygon{polyA},
			tmIDs:    []tms20.TMID{1},
			callResults: []map[tms20.TMID]SnapResult{
				{1: {Geometry: polyA, Tiles: []tile.Tile{tileA}}},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: polyA, Tiles: []tile.Tile{tileA}},
			},
		},
		{
			name:     "MultiPolygon with 2 polygons, single tmID: merged geometry and concatenated tiles",
			geometry: geom.MultiPolygon{polyA, polyB},
			tmIDs:    []tms20.TMID{1},
			callResults: []map[tms20.TMID]SnapResult{
				{1: {Geometry: polyA, Tiles: []tile.Tile{tileA}}},
				{1: {Geometry: polyB, Tiles: []tile.Tile{tileB}}},
			},
			want: map[tms20.TMID]SnapResult{
				1: {Geometry: geom.MultiPolygon{polyA, polyB}, Tiles: []tile.Tile{tileA, tileB}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			f := func(_ geom.Polygon, _ []tms20.TMID) map[tms20.TMID]SnapResult {
				if callCount >= len(tt.callResults) {
					t.Fatalf("unexpected call %d to f", callCount+1)
				}
				result := tt.callResults[callCount]
				callCount++
				return result
			}

			got := processGeometry(tt.geometry, tt.tmIDs, f)

			if callCount != len(tt.callResults) {
				t.Errorf("f called %d times, want %d", callCount, len(tt.callResults))
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processGeometry() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
