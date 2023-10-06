package snap

import (
	"os"
	"reflect"
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
			ix := &PointIndex{
				extent: geom.Extent{0.0, 0.0, 1.0, 1.0},
			}
			if got := ix.containsPoint(tt.pt); got != tt.want {
				t.Errorf("containsPoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPointIndex_GetCentroid(t *testing.T) {
	tests := []struct {
		name   string
		extent geom.Extent
		want   geom.Point
	}{
		{
			name:   "simple",
			extent: geom.Extent{0.0, 0.0, 1.0, 1.0},
			want:   geom.Point{0.5, 0.5},
		},
		{
			name:   "zero",
			extent: geom.Extent{0.0, 0.0, 0.0, 0.0},
			want:   geom.Point{0.0, 0.0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := &PointIndex{
				extent: tt.extent,
			}
			if got := ix.GetCentroid(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetCentroid() = %v, want %v", got, tt.want)
			}
		})
	}
}

//nolint:funlen
func TestPointIndex_InsertPoint(t *testing.T) {
	tests := []struct {
		name     string
		extent   geom.Extent
		maxDepth uint
		point    geom.Point
		want     PointIndex
	}{
		{
			name:     "leaf",
			extent:   geom.Extent{0.0, 0.0, 1.0, 1.0},
			maxDepth: 0,
			point:    geom.Point{0.5, 0.5},
			want: PointIndex{
				extent:    geom.Extent{0.0, 0.0, 1.0, 1.0},
				hasPoints: true,
				maxDepth:  0,
				quadrants: [4]*PointIndex{},
			},
		},
		{
			name:     "centroid",
			extent:   geom.Extent{0.0, 0.0, 1.0, 1.0},
			maxDepth: 1,
			point:    geom.Point{0.5, 0.5},
			want: PointIndex{
				extent:    geom.Extent{0.0, 0.0, 1.0, 1.0},
				hasPoints: true,
				maxDepth:  1,
				quadrants: [4]*PointIndex{
					nil, nil, nil, {
						level:     1,
						x:         1,
						y:         1,
						extent:    geom.Extent{0.5, 0.5, 1.0, 1.0},
						hasPoints: true,
						maxDepth:  0,
						quadrants: [4]*PointIndex{},
					},
				},
			},
		},
		{
			name:     "deep",
			extent:   geom.Extent{0.0, 0.0, 4.0, 4.0},
			maxDepth: 3,
			point:    geom.Point{2.8, 3.2},
			want: PointIndex{
				extent:    geom.Extent{0.0, 0.0, 4.0, 4.0},
				hasPoints: true,
				maxDepth:  3,
				quadrants: [4]*PointIndex{
					nil,
					nil,
					nil,
					{
						level:     1,
						x:         1,
						y:         1,
						extent:    geom.Extent{2.0, 2.0, 4.0, 4.0},
						hasPoints: true,
						maxDepth:  2,
						quadrants: [4]*PointIndex{
							nil,
							nil,
							{
								level:     2,
								x:         2,
								y:         3,
								extent:    geom.Extent{2.0, 3.0, 3.0, 4.0},
								hasPoints: true,
								maxDepth:  1,
								quadrants: [4]*PointIndex{
									nil,
									{
										level:     3,
										x:         5,
										y:         6,
										extent:    geom.Extent{2.5, 3.0, 3.0, 3.5},
										hasPoints: true,
										maxDepth:  0,
										quadrants: [4]*PointIndex{},
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
			name:     "deeper",
			extent:   geom.Extent{0.0, 0.0, 16.0, 16.0},
			maxDepth: 5, // deepest res = 0.5
			point:    geom.Point{2.0, 6.0},
			want: PointIndex{
				extent:    geom.Extent{0.0, 0.0, 16.0, 16.0},
				hasPoints: true,
				maxDepth:  5,
				quadrants: [4]*PointIndex{
					{
						level:     1,
						x:         0,
						y:         0,
						extent:    geom.Extent{0.0, 0.0, 8.0, 8.0},
						hasPoints: true,
						maxDepth:  4,
						quadrants: [4]*PointIndex{
							nil,
							nil,
							{
								level:     2,
								x:         0,
								y:         1,
								extent:    geom.Extent{0.0, 4.0, 4.0, 8.0},
								hasPoints: true,
								maxDepth:  3,
								quadrants: [4]*PointIndex{
									nil,
									nil,
									nil,
									{
										level:     3,
										x:         1,
										y:         3,
										extent:    geom.Extent{2.0, 6.0, 4.0, 8.0},
										hasPoints: true,
										maxDepth:  2,
										quadrants: [4]*PointIndex{
											{
												level:     4,
												x:         2,
												y:         6,
												extent:    geom.Extent{2.0, 6.0, 3.0, 7.0},
												hasPoints: true,
												maxDepth:  1,
												quadrants: [4]*PointIndex{
													{
														level:     5,
														x:         4,
														y:         12,
														extent:    geom.Extent{2.0, 6.0, 2.5, 6.5},
														hasPoints: true,
														maxDepth:  0,
														quadrants: [4]*PointIndex{},
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
			ix := &PointIndex{
				extent:   tt.extent,
				maxDepth: tt.maxDepth,
			}
			ix.InsertPoint(tt.point)
			assert.EqualValues(t, tt.want, *ix)
		})
	}
}

func TestPointIndex_SnapClosestPoints(t *testing.T) {
	tests := []struct {
		name string
		poly geom.Polygon
		line geom.Line
		want [][2]float64
	}{
		{
			name: "nowhere even close",
			poly: geom.Polygon{
				{{0.0, 0.0}, {0.0, 2.0}, {2.0, 2.0}, {2.0, 0.0}},
			},
			line: geom.Line{{4.0, 4.0}, {8.0, 8.0}},
			want: make([][2]float64, 0), // nothing because the line is not part of the original geom so no points indexed
			// TODO maybe you always want the start and end point of the line, regardless if there are points. don't think so though.
		},
		{
			name: "no extra points",
			poly: geom.Polygon{
				{{0.0, 0.0}, {0.0, 8.0}, {8.0, 8.0}, {8.0, 0.0}},
				{{2.0, 2.0}, {6.0, 2.0}, {6.0, 6.0}, {2.0, 6.0}},
			},
			line: geom.Line{{2.0, 2.0}, {6.0, 2.0}},
			want: [][2]float64{{2.25, 2.25}, {6.25, 2.25}}, // same amount of points, but snapped to centroid
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := &PointIndex{
				extent:   geom.Extent{0.0, 0.0, 16.0, 16.0},
				maxDepth: 5, // deepest res = 0.5
			}
			ix.InsertPolygon(&tt.poly)
			ix.toWkt(os.Stdout)
			got := ix.SnapClosestPoints(tt.line)
			if !assert.EqualValues(t, tt.want, got) {
				t.Errorf("SnapClosestPoints() = %v, want %v", got, tt.want)
			}
		})
	}
}
