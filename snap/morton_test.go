package snap

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_toZ(t *testing.T) {
	tests := []struct {
		x     uint
		y     uint
		z     Z
		notOK bool
	}{
		{x: 0b0, y: 0b0, z: 0b0},
		{x: 0b1, y: 0b1, z: 0b11},
		{x: 0b11, y: 0b0, z: 0b0101},
		{x: 0b1111111111111111, y: 0b0, z: 0b01010101010101010101010101010101},
		{x: 0b11111111111111111111111111111111, y: 0b0, z: 0b0101010101010101010101010101010101010101010101010101010101010101},
		{x: 0b100000000000000000000000000000000, notOK: true},
	}
	for _, tt := range tests {
		name := fmt.Sprintf(`toZ(%b, %b)`, tt.x, tt.y)
		t.Run(name, func(t *testing.T) {
			got, ok := toZ(tt.x, tt.y)
			if tt.notOK {
				require.False(t, ok)
			} else {
				require.Equalf(t, tt.z, got, `%032b and %032b should interleave into: %064b, got: %064b`, tt.x, tt.y, tt.z, got)
			}
		})
	}
}

func Test_fromZ(t *testing.T) {
	tests := []struct {
		z     Z
		x     uint
		y     uint
		notOK bool
	}{
		{z: 0b0, x: 0b0, y: 0b0},
		{z: 0b11, x: 0b1, y: 0b1},
		{z: 0b0101, x: 0b11, y: 0b0},
		{z: 0b01010101010101010101010101010101, x: 0b1111111111111111, y: 0b0},
		{z: 0b0101010101010101010101010101010101010101010101010101010101010101, x: 0b11111111111111111111111111111111, y: 0b0},
	}
	for _, tt := range tests {
		name := fmt.Sprintf(`fromZ(%b)`, tt.z)
		t.Run(name, func(t *testing.T) {
			gotX, gotY := fromZ(tt.z)
			require.Equalf(t, [2]uint{tt.x, tt.y}, [2]uint{gotX, gotY}, `%064b should deinterleave into: [%032b,%032b], got: [%032b,%032b]`, tt.z, tt.x, tt.y, gotX, gotY)
		})
	}
}
