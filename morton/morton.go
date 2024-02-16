package morton

import (
	"fmt"
	"math"
)

type Z = uint

var (
	masks = [...]uint{
		0b0101010101010101010101010101010101010101010101010101010101010101,
		0b0011001100110011001100110011001100110011001100110011001100110011,
		0b0000111100001111000011110000111100001111000011110000111100001111,
		0b0000000011111111000000001111111100000000111111110000000011111111,
		0b0000000000000000111111111111111100000000000000001111111111111111,
		0b0000000000000000000000000000000011111111111111111111111111111111,
	}
	powersOfTwo = [...]uint{0, 1, 2, 4, 8, 16}
)

func ToZ(x, y uint) (z Z, ok bool) {
	ok = x <= math.MaxUint32 && y <= math.MaxUint32
	for i := 4; i >= 0; i-- {
		x = (x | (x << powersOfTwo[i+1])) & masks[i]
		y = (y | (y << powersOfTwo[i+1])) & masks[i]
	}
	z = x | (y << 1)
	return z, ok
}

func MustToZ(x, y uint) Z {
	z, ok := ToZ(x, y)
	if !ok {
		panic(fmt.Errorf(`cannot make Z out of %v and %v`, x, y))
	}
	return z
}

func FromZ(z Z) (x, y uint) {
	x = z
	y = z >> 1
	for i := 0; i <= 5; i++ {
		x = (x | (x >> powersOfTwo[i])) & masks[i]
		y = (y | (y >> powersOfTwo[i])) & masks[i]
	}
	return x, y
}
