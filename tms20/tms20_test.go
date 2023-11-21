package tms20

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/slippy"

	"github.com/stretchr/testify/require"
)

func TestLoadEmbeddedTileMatrixSet(t *testing.T) {
	tests := []struct {
		id   string
		srid uint
	}{
		{id: "CanadianNAD83_LCC", srid: 3978},
		{id: "CDB1GlobalGrid", srid: 4326},
		{id: "EuropeanETRS89_LAEAQuad", srid: 3035},
		{id: "GNOSISGlobalGrid", srid: 4326},
		{id: "LINZAntarticaMapTilegrid", srid: 5482},
		{id: "NetherlandsRDNewQuad", srid: 28992},
		{id: "NZTM2000Quad", srid: 2193},
		{id: "UPSAntarcticWGS84Quad", srid: 5042},
		{id: "UPSArcticWGS84Quad", srid: 5041},
		{id: "UTM31WGS84Quad", srid: 32631},
		{id: "WebMercatorQuad", srid: 3857},
		{id: "WGS1984Quad", srid: 4326},
		{id: "WorldCRS84Quad", srid: 0},
		{id: "WorldMercatorWGS84Quad", srid: 3395},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, err := LoadEmbeddedTileMatrixSet(tt.id)
			require.NoErrorf(t, err, "LoadEmbeddedTileMatrixSet() error = %v", err)

			remarshalled, err := json.Marshal(&got)
			require.NoError(t, err)
			rawJSON, err := embeddedTileMatrixSetsJSONFS.ReadFile("tilematrixsets/" + tt.id + ".json")
			require.NoError(t, err)
			require.JSONEq(t, string(rawJSON), string(remarshalled))

			if tt.srid == 0 {
				require.Panics(t, func() { got.SRID() })
			} else {
				require.Equal(t, tt.srid, got.SRID())
			}
		})
	}
}

func TestLoadJSONTileMatrixSet(t *testing.T) {
	tests := []struct {
		id   string
		srid uint
	}{
		// SomethingWithBottomLeftAndLatLonAndDoubleHeight specials:
		// custom crs, should fall back to (informative) orderedAxes
		// different crs and orderedAxes in boundingBox. strange, but for testing
		// matrixHeight set to double matrixWidth
		{id: "SomethingWithBottomLeftAndLatLonAndDoubleHeight", srid: 1},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			jsonFilePath, err := filepath.Abs(path.Join("testdata", tt.id+".json"))
			require.NoError(t, err)
			got, err := LoadJSONTileMatrixSet(jsonFilePath)
			require.NoErrorf(t, err, "LoadJSONTileMatrixSet() error = %v", err)

			remarshalled, err := json.Marshal(&got)
			require.NoError(t, err)
			rawJSON, err := os.ReadFile(jsonFilePath)
			require.NoError(t, err)
			require.JSONEq(t, string(rawJSON), string(remarshalled))

			if tt.srid == 0 {
				require.Panics(t, func() { got.SRID() })
			} else {
				require.Equal(t, tt.srid, got.SRID())
			}
		})
	}
}

func TestTileMatrixSet_Size(t *testing.T) {
	type args struct {
		zoom uint
	}
	type want struct {
		ok   bool
		tile *slippy.Tile
	}
	tests := []struct {
		id string
		args
		want
	}{
		{id: "NetherlandsRDNewQuad",
			args: args{0},
			want: want{ok: true, tile: &slippy.Tile{Z: 0, X: 1, Y: 1}}},
		{id: "NetherlandsRDNewQuad",
			args: args{1},
			want: want{ok: true, tile: &slippy.Tile{Z: 1, X: 2, Y: 2}}},
		{id: "NetherlandsRDNewQuad",
			args: args{99},
			want: want{ok: false, tile: nil}},
		{id: "SomethingWithBottomLeftAndLatLonAndDoubleHeight",
			args: args{0},
			want: want{ok: true, tile: &slippy.Tile{Z: 0, X: 2, Y: 4}}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v.Size(%v)", tt.id, tt.zoom), func(t *testing.T) {
			tms, err := loadTestOrEmbeddedTileMatrix(tt.id)
			require.NoError(t, err)
			tile, ok := tms.Size(tt.args.zoom)
			if ok != tt.ok {
				t.Errorf("Size(...) ok = %v, want %v", ok, tt.ok)
			}
			if ok {
				require.Equal(t, tt.tile, tile)
			}
		})
	}
}

func TestTileMatrixSet_FromNative(t *testing.T) {
	type args struct {
		zoom uint
		pt   geom.Point
	}
	type want struct {
		ok   bool
		tile *slippy.Tile
	}
	tests := []struct {
		id string
		args
		want
	}{
		{id: "NetherlandsRDNewQuad",
			args: args{1, geom.Point{155000, 463000.0}}, // centroid of extent
			want: want{ok: true, tile: &slippy.Tile{Z: 1, X: 1, Y: 1}}},
		{"NetherlandsRDNewQuad",
			args{100, geom.Point{}}, // zoom too large
			want{false, nil}},
		{"NetherlandsRDNewQuad",
			args{0, geom.Point{-285401.92 - 1, 903401.92}}, // x too small
			want{false, nil}},
		{"NetherlandsRDNewQuad",
			args{0, geom.Point{-285401.92, 903401.92 + 1}}, // y too large
			want{false, nil}},
		{"NetherlandsRDNewQuad",
			args{0, geom.Point{595401.92 + 1, 22598.08}}, // x too large
			want{false, nil}},
		{"NetherlandsRDNewQuad",
			args{0, geom.Point{595401.92, 22598.08 - 1}}, // y too small
			want{false, nil}},
		{id: "SomethingWithBottomLeftAndLatLonAndDoubleHeight",
			args: args{0, geom.Point{256.0, 256.0}}, // centroid of extent
			want: want{ok: true, tile: &slippy.Tile{Z: 0, X: 1, Y: 1}}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v.FromNative(%v, %v)", tt.id, tt.zoom, tt.pt.XY()), func(t *testing.T) {
			tms, err := loadTestOrEmbeddedTileMatrix(tt.id)
			require.NoError(t, err)
			tile, ok := tms.FromNative(tt.args.zoom, tt.args.pt)
			if ok != tt.ok {
				t.Errorf("FromNative(...) ok = %v, want %v", ok, tt.ok)
			}
			if ok {
				require.Equal(t, tt.tile, tile)
			}
		})
	}
}

func TestTileMatrixSet_ToNative(t *testing.T) {
	type args struct {
		tile *slippy.Tile
	}
	type want struct {
		ok bool
		pt geom.Point
	}
	tests := []struct {
		id string
		args
		want
	}{
		{"NetherlandsRDNewQuad",
			args{&slippy.Tile{Z: 1, X: 1, Y: 1}},
			want{ok: true, pt: geom.Point{155000, 463000.0}}}, // centroid of extent, top
		{"SomethingWithBottomLeftAndLatLonAndDoubleHeight",
			args{&slippy.Tile{Z: 0, X: 1, Y: 1}},
			want{ok: true, pt: geom.Point{256.0, 512.0}}}, // centroid of extent, top
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v.ToNative(%v)", tt.id, tt.tile), func(t *testing.T) {
			tms, err := loadTestOrEmbeddedTileMatrix(tt.id)
			require.NoError(t, err)
			point, ok := tms.ToNative(tt.tile)
			if ok != tt.ok {
				t.Errorf("ToNative(...) ok = %v, want %v", ok, tt.ok)
			}
			if ok {
				require.Equal(t, tt.pt, point)
			}
		})
	}
}

func loadTestOrEmbeddedTileMatrix(id string) (TileMatrixSet, error) {
	p, err := filepath.Abs(path.Join("testdata", id+".json"))
	if err != nil {
		return TileMatrixSet{}, err
	}
	tms, err := LoadJSONTileMatrixSet(p)
	if err != nil {
		tms, err = LoadEmbeddedTileMatrixSet(id)
	}
	return tms, err
}
