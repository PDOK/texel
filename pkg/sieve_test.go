package pkg

import (
	"testing"
)

func TestShoelace(t *testing.T) {
	var tests = []struct {
		pts  [][2]float64
		area float64
	}{
		// Rectangle
		0: {pts: [][2]float64{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}, area: float64(100)},
		// Triangle
		1: {pts: [][2]float64{{0, 0}, {5, 10}, {0, 10}, {0, 0}}, area: float64(25)},
		// Missing 'official closing point
		2: {pts: [][2]float64{{0, 0}, {0, 10}, {10, 10}, {10, 0}}, area: float64(100)},
		// Single point
		3: {pts: [][2]float64{{1234, 4321}}, area: float64(0.000000)},
		// No point
		4: {pts: nil, area: float64(0.000000)},
		// Empty point
		5: {pts: [][2]float64{}, area: float64(0.000000)},
	}

	for k, test := range tests {
		area := shoelace(test.pts)
		if area != test.area {
			t.Errorf("test: %d, expected: %f \ngot: %f", k, test.area, area)
		}
	}
}

func TestArea(t *testing.T) {
	var tests = []struct {
		geom [][][2]float64
		area float64
	}{
		// Rectangle
		0: {geom: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, area: float64(100)},
		// Rectangle with hole
		1: {geom: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}, {{2, 2}, {2, 8}, {8, 8}, {8, 2}, {2, 2}}}, area: float64(64)},
		// Rectangle with empty hole
		2: {geom: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}, {}}, area: float64(100)},
		// Rectangle with nil hole
		3: {geom: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}, nil}, area: float64(100)},
		// nil geometry
		4: {geom: nil, area: float64(0)},
	}

	for k, test := range tests {
		area := area(test.geom)
		if area != test.area {
			t.Errorf("test: %d, expected: %f \ngot: %f", k, test.area, area)
		}
	}
}

func TestPolygonSieve(t *testing.T) {
	var tests = []struct {
		geom       [][][2]float64
		resolution float64
		sieved     [][][2]float64
	}{
		// Lower resolution
		0: {geom: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, resolution: float64(9), sieved: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}},
		// Higher resolution
		1: {geom: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, resolution: float64(101), sieved: nil},
		// Nil input
		2: {geom: nil, resolution: float64(1), sieved: nil},
		// Filterout donut
		3: {geom: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}, {{5, 5}, {5, 6}, {6, 6}, {6, 5}, {5, 5}}}, resolution: float64(9), sieved: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}},
		// Donut stays
		4: {geom: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}, {{5, 5}, {5, 8.5}, {8.5, 8.5}, {8.5, 5}, {5, 5}}}, resolution: float64(3), sieved: [][][2]float64{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}, {{5, 5}, {5, 8.5}, {8.5, 8.5}, {8.5, 5}, {5, 5}}}},
	}

	for k, test := range tests {
		geom := polygonSieve(test.geom, test.resolution)
		if test.sieved != nil && geom != nil {
			if area(geom) != area(test.sieved) {
				t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, geom)
			}
		} else if test.sieved == nil && geom != nil {
			t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, geom)
		} else if test.sieved != nil && geom == nil {
			t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, geom)
		}
	}
}

func TestMultiPolygonSieve(t *testing.T) {
	var tests = []struct {
		geom       [][][][2]float64
		resolution float64
		sieved     [][][][2]float64
	}{
		// Lower single polygon resolution
		0: {geom: [][][][2]float64{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}}, resolution: float64(1), sieved: [][][][2]float64{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}}},
		// Higher single polygon resolution
		1: {geom: [][][][2]float64{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}}, resolution: float64(101), sieved: nil},
		// Nil input
		2: {geom: nil, resolution: float64(1), sieved: nil},
		// Low multi polygon resolution
		3: {geom: [][][][2]float64{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, {{{15, 15}, {15, 20}, {20, 20}, {20, 15}, {15, 15}}}}, resolution: float64(1), sieved: [][][][2]float64{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, {{{15, 15}, {15, 20}, {20, 20}, {20, 15}, {15, 15}}}}},
		// single hit on multi polygon
		4: {geom: [][][][2]float64{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, {{{15, 15}, {15, 20}, {20, 20}, {20, 15}, {15, 15}}}}, resolution: float64(9), sieved: [][][][2]float64{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}}},
		// single hit on multi polygon
		5: {geom: [][][][2]float64{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, {{{15, 15}, {15, 20}, {20, 20}, {20, 15}, {15, 15}}}}, resolution: float64(101), sieved: nil},
	}

	for k, test := range tests {
		geom := multiPolygonSieve(test.geom, test.resolution)
		if test.sieved != nil && geom != nil {
			if len(geom) != len(test.sieved) {
				t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, geom)
			}
		} else if test.sieved == nil && geom != nil {
			t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, geom)
		} else if test.sieved != nil && geom == nil {
			t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, geom)
		}
	}
}
