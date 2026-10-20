package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubstituteEnv(t *testing.T) {
	t.Setenv("TEXEL_TEST_PATH", "/data/a.gpkg")
	t.Setenv("TEXEL_TEST_QUOTED", `\"TEST\"`)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no placeholder", `path = "x"`, `path = "x"`},
		{"simple", `path = "{{env.TEXEL_TEST_PATH}}"`, `path = "/data/a.gpkg"`},
		{"spaces", `path = "{{ env.TEXEL_TEST_PATH }}"`, `path = "/data/a.gpkg"`},
		{
			"multiple",
			`{{env.TEXEL_TEST_PATH}}:{{env.TEXEL_TEST_PATH}}`,
			`/data/a.gpkg:/data/a.gpkg`,
		},
		{"escaped quotes", `name = "{{env.TEXEL_TEST_QUOTED}}"`, `name = "\"TEST\""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := substituteEnv(tt.in)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSubstituteEnv_Unset(t *testing.T) {
	_, err := substituteEnv(`a = "{{env.TEXEL_TEST_DEFINITELY_UNSET}}"`)
	require.ErrorContains(t, err, "TEXEL_TEST_DEFINITELY_UNSET")
}
