package intgeom

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntgeom_constInterval(t *testing.T) {
	tests := []struct {
		name                  string
		p, low, high          int64
		lowStrict, highStrict bool
		isUnitInterval        bool
	}{
		{
			name: "below low",
			p:    -1, low: 0, high: 10,
		},
		{
			name: "at inclusive low",
			p:    0, low: 0, high: 10,
			highStrict: true, isUnitInterval: true,
		},
		{
			name: "at exclusive low",
			p:    0, low: 0, high: 10,
			lowStrict: true,
		},
		{
			name: "between bounds",
			p:    5, low: 0, high: 10,
			lowStrict: true, highStrict: true, isUnitInterval: true,
		},
		{
			name: "at inclusive high",
			p:    10, low: 0, high: 10,
			lowStrict: true, isUnitInterval: true,
		},
		{
			name: "at exclusive high",
			p:    10, low: 0, high: 10,
			highStrict: true,
		},
		{
			name: "above high",
			p:    11, low: 0, high: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected := emptyInterval
			if tt.isUnitInterval {
				expected = unitInterval
			}

			actual := constInterval(tt.p, tt.low, tt.high, tt.lowStrict, tt.highStrict)
			assert.Equal(t, expected, actual)
		})
	}
}

func TestIntgeom_cmp(t *testing.T) {
	const (
		maxInt64 = int64(1<<63 - 1)
		minInt64 = int64(-1 << 63)
	)

	tests := []struct {
		name     string
		a        Rational
		b        Rational
		expected int64
	}{
		{
			name:     "signs/negative is less than positive",
			a:        Rational{-maxInt64, maxInt64 - 1},
			b:        Rational{maxInt64, maxInt64 - 1},
			expected: -1,
		},
		{
			name:     "signs/positive is greater than negative",
			a:        Rational{maxInt64, maxInt64 - 1},
			b:        Rational{-maxInt64, maxInt64 - 1},
			expected: 1,
		},
		{
			name:     "signs/negative is less than zero",
			a:        Rational{-maxInt64, maxInt64 - 1},
			b:        Rational{0, maxInt64 - 1},
			expected: -1,
		},
		{
			name:     "signs/zero is greater than negative",
			a:        Rational{0, maxInt64 - 1},
			b:        Rational{-maxInt64, maxInt64 - 1},
			expected: 1,
		},
		{
			name:     "signs/zero is less than positive",
			a:        Rational{0, maxInt64 - 1},
			b:        Rational{maxInt64, maxInt64 - 1},
			expected: -1,
		},
		{
			name:     "signs/positive is greater than zero",
			a:        Rational{maxInt64, maxInt64 - 1},
			b:        Rational{0, maxInt64 - 1},
			expected: 1,
		},
		{
			name:     "ordering/positive fractions, less with large cross-products",
			a:        Rational{maxInt64 - 2, maxInt64 - 1},
			b:        Rational{maxInt64 - 1, maxInt64},
			expected: -1,
		},
		{
			name:     "ordering/positive fractions, greater",
			a:        Rational{maxInt64 - 1, maxInt64},
			b:        Rational{maxInt64 - 2, maxInt64 - 1},
			expected: 1,
		},
		{
			name:     "ordering/negative fractions, less",
			a:        Rational{-(maxInt64 - 1), maxInt64},
			b:        Rational{-(maxInt64 - 2), maxInt64 - 1},
			expected: -1,
		},
		{
			name:     "ordering/negative fractions, greater",
			a:        Rational{-(maxInt64 - 2), maxInt64 - 1},
			b:        Rational{-(maxInt64 - 1), maxInt64},
			expected: 1,
		},
		{
			name:     "ordering/small positive fractions",
			a:        Rational{1, 2},
			b:        Rational{2, 3},
			expected: -1,
		},
		{
			name:     "equal/positive equivalent large fractions",
			a:        Rational{6000000000000000000, 4000000000000000000},
			b:        Rational{3000000000000000000, 2000000000000000000},
			expected: 0,
		},
		{
			name:     "equal/negative equivalent large fractions",
			a:        Rational{-6000000000000000000, 4000000000000000000},
			b:        Rational{-3000000000000000000, 2000000000000000000},
			expected: 0,
		},
		{
			name:     "equal/equivalent large and small fraction",
			a:        Rational{6000000000000000000, 4000000000000000000},
			b:        Rational{3, 2},
			expected: 0,
		},
		{
			name:     "equal/zero with different small denominators",
			a:        Rational{0, 1},
			b:        Rational{0, 7},
			expected: 0,
		},
		{
			name:     "large cross-products differ in high 64 bits",
			a:        Rational{maxInt64, maxInt64 / 2},
			b:        Rational{maxInt64 / 2, maxInt64},
			expected: 1,
		},
		{
			name:     "minimum numerator",
			a:        Rational{minInt64, maxInt64},
			b:        Rational{-1, 1},
			expected: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, cmp(tt.a, tt.b))
		})
	}
}

func TestIntgeom_isEmpty(t *testing.T) {
	tests := []struct {
		name         string
		bottom       Rational
		top          Rational
		bottomStrict bool
		topStrict    bool
		expected     bool
	}{
		{
			name:     "bottom below top",
			bottom:   Rational{9223372036854775805, 9223372036854775806},
			top:      Rational{9223372036854775806, 9223372036854775807},
			expected: false,
		},
		{
			name:     "bottom above top",
			bottom:   Rational{9223372036854775806, 9223372036854775807},
			top:      Rational{9223372036854775805, 9223372036854775806},
			expected: true,
		},
		{
			name:     "equal inclusive bounds",
			bottom:   Rational{6000000000000000000, 4000000000000000000},
			top:      Rational{3000000000000000000, 2000000000000000000},
			expected: false,
		},
		{
			name:         "equal strict bottom",
			bottom:       Rational{6000000000000000000, 4000000000000000000},
			top:          Rational{3000000000000000000, 2000000000000000000},
			bottomStrict: true,
			expected:     true,
		},
		{
			name:      "equal strict top",
			bottom:    Rational{6000000000000000000, 4000000000000000000},
			top:       Rational{3000000000000000000, 2000000000000000000},
			topStrict: true,
			expected:  true,
		},
		{
			name:         "equal strict bounds",
			bottom:       Rational{6000000000000000000, 4000000000000000000},
			top:          Rational{3000000000000000000, 2000000000000000000},
			bottomStrict: true,
			topStrict:    true,
			expected:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interval := Interval{
				bottom:       tt.bottom,
				top:          tt.top,
				bottomStrict: tt.bottomStrict,
				topStrict:    tt.topStrict,
			}
			assert.Equal(t, tt.expected, interval.isEmpty())
		})
	}
}

func TestIntgeom_IntersectsLine(t *testing.T) {
	testCases := []struct {
		name     string
		line     Line
		expected bool
	}{
		// The used extent is [0, 10) x [0, 10).

		// Corners
		{
			name:     "corner/ends on min corner from outside",
			line:     Line{{0, 0}, {-5, -5}},
			expected: true,
		},
		{
			name:     "corner/ends on max corner from outside",
			line:     Line{{10, 10}, {15, 15}},
			expected: false,
		},
		{
			name:     "corner/ends on max-x min-y corner from outside",
			line:     Line{{10, 0}, {15, -5}},
			expected: false,
		},
		{
			name:     "corner/ends on min-x max-y corner from outside",
			line:     Line{{0, 10}, {-5, 15}},
			expected: false,
		},
		{
			name:     "corner/starts on max corner, rest inside",
			line:     Line{{10, 10}, {5, 5}},
			expected: true,
		},
		{
			name:     "corner/slope -2 touches only min corner",
			line:     Line{{-1, 2}, {1, -2}},
			expected: true,
		},
		{
			name:     "corner/slope -2 touches only max corner",
			line:     Line{{9, 12}, {11, 8}},
			expected: false,
		},
		{
			name:     "corner/slope -2 passes one unit outside min corner",
			line:     Line{{-1, 1}, {1, -3}},
			expected: false,
		},
		{
			name:     "corner/slope -2 clips min corner into interior",
			line:     Line{{-1, 4}, {2, -2}},
			expected: true,
		},
		{
			name:     "corner/slope -1/3 touches only min corner",
			line:     Line{{-3, 1}, {3, -1}},
			expected: true,
		},
		{
			name:     "corner/slope -3/7 touches only max corner",
			line:     Line{{3, 13}, {17, 7}},
			expected: false,
		},
		{
			name:     "corner/outside end on max corner",
			line:     Line{{20, 20}, {10, 10}},
			expected: false,
		},

		// Basic geometry
		{
			name:     "basic/fully inside",
			line:     Line{{2, 2}, {8, 8}},
			expected: true,
		},
		{
			name:     "basic/one end inside",
			line:     Line{{5, 5}, {20, 12}},
			expected: true,
		},
		{
			name:     "basic/through opposite edges",
			line:     Line{{-5, 3}, {15, 7}},
			expected: true,
		},
		{
			name:     "basic/through adjacent edges",
			line:     Line{{-2, 5}, {5, -2}},
			expected: true,
		},
		{
			name:     "basic/misses min corner",
			line:     Line{{-3, 2}, {2, -3}},
			expected: false,
		},
		{
			name:     "basic/far away",
			line:     Line{{20, 20}, {30, 25}},
			expected: false,
		},

		// Segment versus infinite line
		// All use a line that does cross the extent
		{
			name:     "segment/stops short of extent",
			line:     Line{{-10, -3}, {-2, 1}},
			expected: false,
		},
		{
			name:     "segment/starts past extent and points away",
			line:     Line{{12, 7}, {20, 11}},
			expected: false,
		},
		{
			name:     "segment/ends exactly on min edge",
			line:     Line{{-10, -3}, {0, 2}},
			expected: true,
		},
		{
			name:     "segment/ends exactly on max edge",
			line:     Line{{20, 11}, {10, 6}},
			expected: false,
		},

		// Axis-aligned lines and edges
		{
			name:     "axis/horizontal through extent",
			line:     Line{{-5, 5}, {15, 5}},
			expected: true,
		},
		{
			name:     "axis/horizontal along max-y edge",
			line:     Line{{-5, 10}, {15, 10}},
			expected: false,
		},
		{
			name:     "axis/vertical along min-x edge",
			line:     Line{{0, -5}, {0, 5}},
			expected: true,
		},
		{
			name:     "axis/vertical along max-x edge",
			line:     Line{{10, -5}, {10, 5}},
			expected: false,
		},
		{
			name:     "axis/ends on middle of max-y edge, pointing out",
			line:     Line{{5, 10}, {5, 15}},
			expected: false,
		},
		{
			name:     "axis/ends on middle of min-y edge, pointing out",
			line:     Line{{5, 0}, {5, -5}},
			expected: true,
		},

		// Zero-length lines
		{
			name:     "point/inside",
			line:     Line{{5, 5}, {5, 5}},
			expected: true,
		},
		{
			name:     "point/on max-x edge",
			line:     Line{{10, 5}, {10, 5}},
			expected: false,
		},
		{
			name:     "point/on min corner",
			line:     Line{{0, 0}, {0, 0}},
			expected: true,
		},
		{
			name:     "point/outside",
			line:     Line{{-3, -3}, {-3, -3}},
			expected: false,
		},
	}
	for _, tt := range testCases {
		extent := Extent{0, 0, 10, 10}
		reversed := Line{tt.line[1], tt.line[0]}
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extent.IntersectsLine(tt.line))
		})
		t.Run(tt.name+"/reversed", func(t *testing.T) {
			assert.Equal(t, tt.expected, extent.IntersectsLine(reversed))
		})
	}
}

// LineIntersects replaces an old function, for completeness the old tests have
// been included here
func TestIntgeom_LineIntersects_old(t *testing.T) {
	tests := []struct {
		name   string
		extent Extent
		line   Line
		want   bool
	}{
		{
			name: "false positive with naive integer intersect implementation",
			extent: Extent{
				135196160000000,
				516981760000000,
				135202880000000,
				516988480000000,
			},
			line: Line{
				{135201147999999, 516929654000000},
				{135145991000000, 516996354000000},
			},
			want: false,
		},
		{
			name: "diagonally opposed line between non-inclusive points",
			extent: Extent{
				0o0000000, 0o0000000, 10000000, 10000000,
			},
			line: Line{
				{0o0000000, 10000000}, {10000000, 0o0000000},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.extent.IntersectsLine(tt.line)
			assert.Equal(t, tt.want, got)
		})
	}
}
