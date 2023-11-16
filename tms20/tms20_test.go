package tms20

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadEmbeddedTileMatrixSet(t *testing.T) {
	tests := []struct {
		id string
	}{
		{id: "CanadianNAD83_LCC"},
		{id: "CDB1GlobalGrid"},
		{id: "EuropeanETRS89_LAEAQuad"},
		{id: "GNOSISGlobalGrid"},
		{id: "LINZAntarticaMapTilegrid"},
		{id: "NetherlandsRDNewQuad"},
		{id: "NZTM2000Quad"},
		{id: "UPSAntarcticWGS84Quad"},
		{id: "UPSArcticWGS84Quad"},
		{id: "UTM31WGS84Quad"},
		{id: "WebMercatorQuad"},
		{id: "WGS1984Quad"},
		{id: "WorldCRS84Quad"},
		{id: "WorldMercatorWGS84Quad"},
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
		})
	}
}
