package snap

import (
	"github.com/pdok/texel/intgeom"
	"os"
	"testing"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"
)

//nolint:funlen
func TestPointIndex_containsPoint(t *testing.T) {
	tests := []struct {
		name string
		pt   geom.Point
		want bool
	}{
		{
			name: "centroid",
			pt:   geom.Point{0.5, 0.5},
			want: true,
		},
		{
			name: "inclusive edge left",
			pt:   geom.Point{0.5, 0.0},
			want: true,
		},
		{
			name: "inclusive edge bottom",
			pt:   geom.Point{0.0, 0.5},
			want: true,
		},
		{
			name: "exclusive edge right",
			pt:   geom.Point{1.0, 0.5},
			want: false,
		},
		{
			name: "exclusive edge top",
			pt:   geom.Point{0.5, 1.0},
			want: false,
		},
		{
			name: "inclusive corner bottomleft",
			pt:   geom.Point{0.0, 0.0},
			want: true,
		},
		{
			name: "exclusive corner bottomright",
			pt:   geom.Point{1.0, 0.0},
			want: false,
		},
		{
			name: "exclusive corner topright",
			pt:   geom.Point{1.0, 1.0},
			want: false,
		},
		{
			name: "exclusive corner topleft",
			pt:   geom.Point{0.0, 1.0},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := NewPointIndexFromTileMatrix(newSimpleTileMatrix(1.0, 0, 1.0))
			got := ix.containsPoint(intgeom.FromGeomPoint(tt.pt))
			if got != tt.want {
				t.Errorf("containsPoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPointIndex_getQuadrantExtentAndCentroid(t *testing.T) {
	type wants struct {
		extent   intgeom.Extent
		centroid intgeom.Point
	}
	tests := []struct {
		name   string
		matrix TileMatrix
		want   wants
	}{
		{
			name:   "simple",
			matrix: newSimpleTileMatrix(1.0, 0, 1.0),
			want: wants{
				extent:   intgeom.Extent{0, 0, 1000000000, 1000000000},
				centroid: intgeom.Point{intHalf, intHalf},
			},
		},
		{
			name:   "zero",
			matrix: newSimpleTileMatrix(0.0, 0, 0.0),
			want: wants{
				extent:   intgeom.Extent{0, 0, 0, 0},
				centroid: intgeom.Point{0, 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extent, centroid := getQuadrantExtentAndCentroid(&tt.matrix, 0, 0, 0)
			if !assert.EqualValues(t, tt.want.extent, extent) {
				t.Errorf("getQuadrantExtentAndCentroid() = %v, want %v", extent, tt.want.extent)
			}
			if !assert.EqualValues(t, tt.want.centroid, centroid) {
				t.Errorf("getQuadrantExtentAndCentroid() = %v, want %v", centroid, tt.want.centroid)
			}
		})
	}
}

//nolint:funlen
func TestPointIndex_InsertPoint(t *testing.T) {
	tests := []struct {
		name   string
		matrix TileMatrix
		point  geom.Point
		want   PointIndex
	}{
		{
			name:   "leaf",
			matrix: newSimpleTileMatrix(1.0, 0, 1.0),
			point:  geom.Point{0.5, 0.5},
			want: PointIndex{
				intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 1.0, 1.0}),
				intCentroid: intgeom.FromGeomPoint(geom.Point{0.5, 0.5}),
				hasPoints:   true,
				maxDepth:    0,
				quadrants:   [4]*PointIndex{},
			},
		},
		{
			name:   "centroid",
			matrix: newSimpleTileMatrix(1.0, 1, 0.5),
			point:  geom.Point{0.5, 0.5},
			want: PointIndex{
				intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 1.0, 1.0}),
				intCentroid: intgeom.FromGeomPoint(geom.Point{0.5, 0.5}),
				hasPoints:   true,
				maxDepth:    1,
				quadrants: [4]*PointIndex{
					nil, nil, nil, {
						level:       1,
						x:           1,
						y:           1,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{0.5, 0.5, 1.0, 1.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{0.75, 0.75}),
						hasPoints:   true,
						maxDepth:    0,
						quadrants:   [4]*PointIndex{},
					},
				},
			},
		},
		{
			name:   "deep",
			matrix: newSimpleTileMatrix(4.0, 3, 0.5),
			point:  geom.Point{2.8, 3.2},
			want: PointIndex{
				intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 4.0, 4.0}),
				intCentroid: intgeom.FromGeomPoint(geom.Point{2.0, 2.0}),
				hasPoints:   true,
				maxDepth:    3,
				quadrants: [4]*PointIndex{
					nil,
					nil,
					nil,
					{
						level:       1,
						x:           1,
						y:           1,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 2.0, 4.0, 4.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{3.0, 3.0}),
						hasPoints:   true,
						maxDepth:    2,
						quadrants: [4]*PointIndex{
							nil,
							nil,
							{
								level:       2,
								x:           2,
								y:           3,
								intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 3.0, 3.0, 4.0}),
								intCentroid: intgeom.FromGeomPoint(geom.Point{2.5, 3.5}),
								hasPoints:   true,
								maxDepth:    1,
								quadrants: [4]*PointIndex{
									nil,
									{
										level:       3,
										x:           5,
										y:           6,
										intExtent:   intgeom.FromGeomExtent(geom.Extent{2.5, 3.0, 3.0, 3.5}),
										intCentroid: intgeom.FromGeomPoint(geom.Point{2.75, 3.25}),
										hasPoints:   true,
										maxDepth:    0,
										quadrants:   [4]*PointIndex{},
									},
									nil,
									nil,
								},
							},
							nil,
						},
					},
				},
			},
		},
		{
			name:   "deeper",
			matrix: newSimpleTileMatrix(16.0, 5, 0.5),
			point:  geom.Point{2.0, 6.0},
			want: PointIndex{
				intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 16.0, 16.0}),
				intCentroid: intgeom.FromGeomPoint(geom.Point{8.0, 8.0}),
				hasPoints:   true,
				maxDepth:    5,
				quadrants: [4]*PointIndex{
					{
						level:       1,
						x:           0,
						y:           0,
						intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 0.0, 8.0, 8.0}),
						intCentroid: intgeom.FromGeomPoint(geom.Point{4.0, 4.0}),
						hasPoints:   true,
						maxDepth:    4,
						quadrants: [4]*PointIndex{
							nil,
							nil,
							{
								level:       2,
								x:           0,
								y:           1,
								intExtent:   intgeom.FromGeomExtent(geom.Extent{0.0, 4.0, 4.0, 8.0}),
								intCentroid: intgeom.FromGeomPoint(geom.Point{2.0, 6.0}),
								hasPoints:   true,
								maxDepth:    3,
								quadrants: [4]*PointIndex{
									nil,
									nil,
									nil,
									{
										level:       3,
										x:           1,
										y:           3,
										intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 6.0, 4.0, 8.0}),
										intCentroid: intgeom.FromGeomPoint(geom.Point{3.0, 7.0}),
										hasPoints:   true,
										maxDepth:    2,
										quadrants: [4]*PointIndex{
											{
												level:       4,
												x:           2,
												y:           6,
												intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 6.0, 3.0, 7.0}),
												intCentroid: intgeom.FromGeomPoint(geom.Point{2.5, 6.5}),
												hasPoints:   true,
												maxDepth:    1,
												quadrants: [4]*PointIndex{
													{
														level:       5,
														x:           4,
														y:           12,
														intExtent:   intgeom.FromGeomExtent(geom.Extent{2.0, 6.0, 2.5, 6.5}),
														intCentroid: intgeom.FromGeomPoint(geom.Point{2.25, 6.25}),
														hasPoints:   true,
														maxDepth:    0,
														quadrants:   [4]*PointIndex{},
													},
													nil,
													nil,
													nil,
												},
											},
											nil,
											nil,
											nil,
										},
									},
								},
							},
							nil,
						},
					},
					nil,
					nil,
					nil,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := NewPointIndexFromTileMatrix(tt.matrix)
			ix.InsertPoint(tt.point)
			setRootMatrices(&tt.want, &tt.matrix)
			assert.EqualValues(t, tt.want, *ix)
		})
	}
}

func TestPointIndex_SnapClosestPoints(t *testing.T) {
	tests := []struct {
		name   string
		matrix TileMatrix
		poly   geom.Polygon
		line   geom.Line
		want   [][2]float64
	}{
		{
			name:   "nowhere even close",
			matrix: newSimpleTileMatrix(8.0, 4, 0.5),
			poly: geom.Polygon{
				{{0.0, 0.0}, {0.0, 2.0}, {2.0, 2.0}, {2.0, 0.0}},
			},
			line: geom.Line{{4.0, 4.0}, {8.0, 8.0}},
			want: make([][2]float64, 0), // nothing because the line is not part of the original geom so no points indexed
		},
		{
			name:   "no extra points",
			matrix: newSimpleTileMatrix(16.0, 5, 0.5),
			poly: geom.Polygon{
				{{0.0, 0.0}, {0.0, 8.0}, {8.0, 8.0}, {8.0, 0.0}},
				{{2.0, 2.0}, {6.0, 2.0}, {6.0, 6.0}, {2.0, 6.0}},
			},
			line: geom.Line{{2.0, 2.0}, {6.0, 2.0}},
			want: [][2]float64{{2.25, 2.25}, {6.25, 2.25}}, // same amount of points, but snapped to centroid
		},
		{
			name:   "extra points (scary geom 1)",
			matrix: newSimpleTileMatrix(8.0, 4, 0.5),
			poly: geom.Polygon{
				{{0.0, 5.0}, {5.0, 4.0}, {5.0, 0.0}, {3.0, 0.0}, {0.0, 2.0}},
				{{1.0, 3.0}, {3.0, 3.0}, {3.0, 1.0}, {1.25, 1.25}},
			},
			line: geom.Line{{3.0, 0.0}, {0.0, 2.0}},
			want: [][2]float64{{3.25, 0.25}, {1.25, 1.25}, {0.25, 2.25}}, // extra point in the middle
		},
		{
			name:   "horizontal line",
			matrix: newNetherlandsRDNewQuadTileMatrix(14),
			poly: geom.Polygon{{
				{110906.87099999999918509, 504428.79999999998835847}, // horizontal line between quadrants
				{110907.64400000000023283, 504428.79999999998835847}, // horizontal line between quadrants
			}},
			line: geom.Line{
				{110906.87099999999918509, 504428.79999999998835847}, // horizontal line between quadrants
				{110907.64400000000023283, 504428.79999999998835847},
			},
			want: [][2]float64{
				{110906.8709375, 504428.8065625}, // horizontal line still here
				{110907.6453125, 504428.8065625}, // horizontal line still here
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := NewPointIndexFromTileMatrix(tt.matrix)
			ix.InsertPolygon(&tt.poly)
			ix.toWkt(os.Stdout)
			got := ix.SnapClosestPoints(tt.line)
			if !assert.EqualValues(t, tt.want, got) {
				t.Errorf("SnapClosestPoints() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newSimpleTileMatrix(maxY float64, level uint, cellSize float64) TileMatrix {
	return TileMatrix{
		MaxY:      maxY,
		PixelSize: 1,
		TileSize:  1,
		Level:     level,
		CellSize:  cellSize,
	}
}

func newNetherlandsRDNewQuadTileMatrix(level uint) TileMatrix {
	return TileMatrix{
		MinX:      -285401.92,
		MaxY:      903401.92,
		PixelSize: 16,
		TileSize:  256,
		Level:     level,
		CellSize:  3440.64 / float64(pow2(level)),
	}
}

func setRootMatrices(pi *PointIndex, matrix *TileMatrix) {
	pi.rootMatrix = matrix
	for _, qu := range pi.quadrants {
		if qu == nil {
			continue
		}
		setRootMatrices(qu, matrix)
	}
}
