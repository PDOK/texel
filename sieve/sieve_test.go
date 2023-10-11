package sieve

import (
	"github.com/go-spatial/geom"
	"github.com/pdok/sieve/processing"
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
		outGeom := polygonSieve(test.geom, test.resolution)
		if test.sieved != nil && outGeom != nil {
			if area(outGeom) != area(test.sieved) {
				t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, outGeom)
			}
		} else if test.sieved == nil && outGeom != nil {
			t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, outGeom)
		} else if test.sieved != nil && outGeom == nil {
			t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, outGeom)
		}
	}
}

func TestMultiPolygonSieve(t *testing.T) {
	var tests = []struct {
		geom       geom.MultiPolygon
		resolution float64
		sieved     geom.MultiPolygon
	}{
		// Lower single polygon resolution
		0: {geom: geom.MultiPolygon{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}}, resolution: float64(1), sieved: geom.MultiPolygon{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}}},
		// Higher single polygon resolution
		1: {geom: geom.MultiPolygon{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}}, resolution: float64(101), sieved: nil},
		// Nil input
		2: {geom: nil, resolution: float64(1), sieved: nil},
		// Low multi polygon resolution
		3: {geom: geom.MultiPolygon{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, {{{15, 15}, {15, 20}, {20, 20}, {20, 15}, {15, 15}}}}, resolution: float64(1), sieved: geom.MultiPolygon{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, {{{15, 15}, {15, 20}, {20, 20}, {20, 15}, {15, 15}}}}},
		// single hit on multi polygon
		4: {geom: geom.MultiPolygon{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, {{{15, 15}, {15, 20}, {20, 20}, {20, 15}, {15, 15}}}}, resolution: float64(9), sieved: geom.MultiPolygon{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}}},
		// single hit on multi polygon
		5: {geom: geom.MultiPolygon{{{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}}, {{{15, 15}, {15, 20}, {20, 20}, {20, 15}, {15, 15}}}}, resolution: float64(101), sieved: nil},
	}

	for k, test := range tests {
		featuresIn := []*TestFeature{
			{geom: test.geom},
		}
		source := TestSource{
			FeaturesIn: featuresIn,
		}

		Sieve(&source, &source, test.resolution)

		var outGeom geom.MultiPolygon
		ok := true
		if len(source.FeaturesOut) > 0 {
			result := source.FeaturesOut[0].Geometry()
			outGeom, ok = result.(geom.MultiPolygon)
		}

		switch {
		case !ok:
			t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, outGeom)
		case test.sieved != nil && outGeom != nil:
			if len(outGeom) != len(test.sieved) {
				t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, outGeom)
			}
		case test.sieved == nil && outGeom != nil:
			fallthrough
		case test.sieved != nil && outGeom == nil:
			t.Errorf("test: %d, expected: %f \ngot: %f", k, test.sieved, outGeom)
		}
	}
}

type TestSource struct {
	FeaturesIn  []*TestFeature
	FeaturesOut []processing.Feature
}

func (s *TestSource) ReadFeatures(features chan<- processing.Feature) {
	for _, feature := range s.FeaturesIn {
		features <- feature
	}
	close(features)
}

func (s *TestSource) WriteFeatures(features <-chan processing.Feature) {
	for feature := range features {
		s.FeaturesOut = append(s.FeaturesOut, feature)
	}
}

type TestFeature struct {
	geom geom.Geometry
}

func (f TestFeature) Columns() []interface{} {
	return nil
}

func (f TestFeature) Geometry() geom.Geometry {
	return f.geom
}

func (f *TestFeature) UpdateGeometry(geom geom.Geometry) {
	f.geom = geom
}
