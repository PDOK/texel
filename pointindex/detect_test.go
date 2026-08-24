package pointindex

import (
	"sort"
	"testing"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pdok/texel/intgeom"
	"github.com/pdok/texel/mathhelp"
	"github.com/pdok/texel/morton"
	"github.com/pdok/texel/tile"
	"github.com/pdok/texel/tms20"
)

func TestPointIndex_GetQBBoxWithBuffer(t *testing.T) {
	type tileCoord struct{ x, y uint }
	tests := []struct {
		name         string
		tmsID        tms20.TMID
		tilePixels   uint
		deepestLevel Level
		points       [][2]int // Points to be inserted at deepestlevel
		bufferSize   uint
		wantTiles    []tileCoord
	}{
		{
			// deepestLevel 4 -> 16x16 pixel grid, tmsID 2 -> 4x4 tiles of 4x4 pixels each.
			name:       "points all within one tile, no buffer",
			tmsID:      2,
			tilePixels: 4,
			points:     [][2]int{{5, 5}, {6, 6}},
			bufferSize: 0,
			wantTiles:  []tileCoord{{1, 1}},
		},
		{
			name:       "point in one tile, buffer expands to multiple tiles",
			tmsID:      2,
			tilePixels: 4,
			points:     [][2]int{{0, 0}},
			bufferSize: 6,
			wantTiles:  []tileCoord{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
		},
		{
			name:       "non-square rectangle of tiles",
			tmsID:      2,
			tilePixels: 4,
			points:     [][2]int{{4, 4}, {11, 5}},
			bufferSize: 0,
			wantTiles:  []tileCoord{{1, 1}, {2, 1}},
		},
		{
			name:       "points in diagonal tiles, expands to rectangle",
			tmsID:      2,
			tilePixels: 4,
			points:     [][2]int{{3, 3}, {4, 4}},
			bufferSize: 0,
			wantTiles:  []tileCoord{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := newPointIndex(tt.tmsID, tt.tilePixels, 1, 1.0)
			for _, p := range tt.points {
				require.NoError(t, ix.InsertCoord(p[0], p[1]))
			}

			l := levelFromTmsId(tt.tmsID)
			want := make([]tile.Tile, 0, len(tt.wantTiles))
			for _, tc := range tt.wantTiles {
				extent, _ := ix.getQuadrantExtentAndCentroid(l, tc.x, tc.y, ix.intExtent)
				want = append(want, tile.Tile{
					Extent:      extent,
					X:           tc.x,
					Y:           tc.y,
					IsContained: false,
				})
			}

			got := ix.GetQBBoxWithBuffer(tt.tmsID, tt.bufferSize)
			assert.ElementsMatch(t, want, got)
		})
	}
}

// registeredTile records a single call to a RegisterFunc during a test.
type registeredTile struct {
	x, y uint
}

// Register function for testing
func recordingRegister(dst *[]registeredTile) registerFunc {
	return func(tileX, tileY uint, l Level) {
		maxCoord := mathhelp.Pow2(l) - 1
		if tileX > maxCoord || tileY > maxCoord {
			return
		}
		*dst = append(*dst, registeredTile{tileX, tileY})
	}
}

// order and deduplicate slice of registeredTile (for testing)
func uniqueTileCoords(records []registeredTile) [][2]uint {
	seen := map[[2]uint]bool{}
	var out [][2]uint
	for _, r := range records {
		key := [2]uint{r.x, r.y}
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// Pointindex with centre not at (0,0)
func newOffsetPointIndex(deepestLevel Level, cellSize, originX, originY float64) *PointIndex {
	deepestSize := mathhelp.Pow2(deepestLevel)
	span := cellSize * float64(deepestSize)
	intExtent := intgeom.Extent{
		intgeom.FromGeomOrd(originX), intgeom.FromGeomOrd(originY),
		intgeom.FromGeomOrd(originX + span), intgeom.FromGeomOrd(originY + span),
	}
	return &PointIndex{
		Quadrant:     Quadrant{intExtent: intExtent},
		deepestLevel: deepestLevel,
		deepestSize:  deepestSize,
		//nolint:gosec // G115
		deepestRes: intExtent.XSpan() / int64(deepestSize),
	}
}

func TestLineTrace_TilesTouched(t *testing.T) {
	tests := []struct {
		name         string
		deepestLevel Level
		l            Level
		cellSize     float64
		originX      float64
		originY      float64
		line         geom.Line
		buffer       uint
		want         [][2]uint
	}{
		{
			name:         "no buffer, +x+y diagonal",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 2}, geom.Point{14, 10}},
			buffer: 0,
			want:   [][2]uint{{0, 0}, {1, 0}, {1, 1}, {2, 1}, {2, 2}, {3, 2}},
		},
		{
			name:         "no buffer, -x-y diagonal (reverse of +x+y)",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{14, 10}, geom.Point{2, 2}},
			buffer: 0,
			want:   [][2]uint{{0, 0}, {1, 0}, {1, 1}, {2, 1}, {2, 2}, {3, 2}},
		},
		{
			name:         "no buffer, +x-y diagonal",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 10}, geom.Point{14, 2}},
			buffer: 0,
			want:   [][2]uint{{0, 2}, {1, 1}, {1, 2}, {2, 0}, {2, 1}, {3, 0}},
		},
		{
			name:         "no buffer, -x+y diagonal (reverse of +x-y)",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{14, 2}, geom.Point{2, 10}},
			buffer: 0,
			want:   [][2]uint{{0, 2}, {1, 1}, {1, 2}, {2, 0}, {2, 1}, {3, 0}},
		},
		{
			name:         "buffer, +x+y diagonal (reaches beyond index limit)",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 2}, geom.Point{14, 9}},
			buffer: 2,
			want:   [][2]uint{{0, 0}, {0, 1}, {1, 0}, {1, 1}, {1, 2}, {2, 0}, {2, 1}, {2, 2}, {3, 1}, {3, 2}},
		},
		{
			name:         "buffer, -x-y diagonal (ververse of +x+y)",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{14, 9}, geom.Point{2, 2}},
			buffer: 2,
			want:   [][2]uint{{0, 0}, {0, 1}, {1, 0}, {1, 1}, {1, 2}, {2, 0}, {2, 1}, {2, 2}, {3, 1}, {3, 2}},
		},
		{
			name:         "buffer, +x-y diagonal",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 10}, geom.Point{14, 2}},
			buffer: 2,
			want:   [][2]uint{{0, 1}, {0, 2}, {0, 3}, {1, 0}, {1, 1}, {1, 2}, {1, 3}, {2, 0}, {2, 1}, {2, 2}, {3, 0}, {3, 1}},
		},
		{
			name:         "buffer, -x+y diagonal: (reverse of +x-y)",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{14, 2}, geom.Point{2, 10}},
			buffer: 2,
			want:   [][2]uint{{0, 1}, {0, 2}, {0, 3}, {1, 0}, {1, 1}, {1, 2}, {1, 3}, {2, 0}, {2, 1}, {2, 2}, {3, 0}, {3, 1}},
		},
		{
			name:         "horizontal line",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 5}, geom.Point{14, 5}},
			buffer: 0,
			want:   [][2]uint{{0, 1}, {1, 1}, {2, 1}, {3, 1}},
		},
		{
			name:         "vertical line",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{5, 2}, geom.Point{5, 14}},
			buffer: 0,
			want:   [][2]uint{{1, 0}, {1, 1}, {1, 2}, {1, 3}},
		},
		{
			name:         "end in upper right corner",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 2}, geom.Point{3, 3}},
			buffer: 0,
			want:   [][2]uint{{0, 0}},
		},
		{
			name:         "alongside upper edge",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 2}, geom.Point{3, 3}},
			buffer: 1,
			want:   [][2]uint{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
		},
		{
			name:         "start in corner, register tiles behind you",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{3, 3}, geom.Point{2, 2}},
			buffer: 1,
			want:   [][2]uint{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
		},
		{
			name:         "no buffer: walk across tile corner",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{6, 6}, geom.Point{10, 10}},
			buffer: 0,
			want:   [][2]uint{{1, 1}, {1, 2}, {2, 1}, {2, 2}},
		},
		{
			name:         "buffer: walk across tile corner",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{6, 6}, geom.Point{7, 7}},
			buffer: 1,
			want:   [][2]uint{{1, 1}, {1, 2}, {2, 1}, {2, 2}},
		},
		{
			name:         "buffer clips upper right corner of tile",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{12, 14}, geom.Point{14, 12}},
			buffer: 1,
			want:   [][2]uint{{2, 2}, {2, 3}, {3, 2}, {3, 3}}, // Arguably {2,2} should not be here
		},
		{
			name:         "non-zero origin: same relative result as +x+y diagonal",
			deepestLevel: 4, l: 2, cellSize: 1.0, originX: 100, originY: 100,
			line:   geom.Line{geom.Point{102, 102}, geom.Point{114, 110}},
			buffer: 0,
			want:   [][2]uint{{0, 0}, {1, 0}, {1, 1}, {2, 1}, {2, 2}, {3, 2}},
		},
		{
			name:         "non-zero origin with buffer: same relative result as buffered corner case",
			deepestLevel: 4, l: 2, cellSize: 1.0, originX: 100, originY: 100,
			line:   geom.Line{geom.Point{103, 103}, geom.Point{105, 105}},
			buffer: 1,
			want:   [][2]uint{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := newOffsetPointIndex(tt.deepestLevel, tt.cellSize, tt.originX, tt.originY)

			var recorded []registeredTile
			ix.lineTrace(tt.line, tt.l, tt.deepestLevel, tt.buffer, recordingRegister(&recorded))

			got := uniqueTileCoords(recorded)
			assert.Equal(t, tt.want, got)
		})
	}
}

// helper for building classification data
func buildClassification(tuples ...[3]uint) map[Level]map[morton.Z]TileClassification {
	classification := make(map[Level]map[morton.Z]TileClassification)
	for _, tuple := range tuples {
		level, x, y := tuple[0], tuple[1], tuple[2]
		if classification[level] == nil {
			classification[level] = make(map[morton.Z]TileClassification)
		}
		classification[level][morton.MustToZ(x, y)] = ClassificationIntersect
	}
	return classification
}

func TestPointIndex_findIntersectingTilesLeft(t *testing.T) {
	tests := []struct {
		name           string
		x, y           uint
		targetLevel    Level
		classification map[Level]map[morton.Z]TileClassification
		want           []morton.Z
	}{
		{
			name:           "trivial example at level 0",
			x:              0,
			y:              0,
			targetLevel:    0,
			classification: buildClassification(),
			want:           []morton.Z{0},
		},
		{
			name:           "no tiles",
			x:              3,
			y:              3,
			targetLevel:    2,
			classification: buildClassification(),
			want:           []morton.Z{},
		},
		{
			name:        "single tile registered, check left from this tile.",
			x:           0,
			y:           0,
			targetLevel: 3,
			classification: buildClassification(
				[3]uint{0, 0, 0},
				[3]uint{1, 0, 0},
				[3]uint{2, 0, 0},
				[3]uint{3, 0, 0},
			),
			want: []morton.Z{0},
		},
		{
			name:        "single tile registered, check this tile, requires rightChild",
			x:           1,
			y:           0,
			targetLevel: 1,
			classification: buildClassification(
				[3]uint{0, 0, 0},
				[3]uint{1, 1, 0},
			),
			want: []morton.Z{1},
		},
		{
			name:        "tile registered right of target",
			x:           0,
			y:           0,
			targetLevel: 1,
			classification: buildClassification(
				[3]uint{0, 0, 0},
				[3]uint{1, 1, 0},
			),
			want: []morton.Z{},
		},
		{
			name:        "use odd y coordinate",
			x:           0,
			y:           1,
			targetLevel: 1,
			classification: buildClassification(
				[3]uint{0, 0, 0},
				[3]uint{1, 0, 1},
			),
			want: []morton.Z{2},
		},
		{
			name:        "use odd y and rightChild",
			x:           0,
			y:           1,
			targetLevel: 1,
			classification: buildClassification(
				[3]uint{0, 0, 0},
				[3]uint{1, 1, 1},
			),
			want: []morton.Z{},
		},
		{
			name:        "ordd y and rightChild, want to find it",
			x:           1,
			y:           1,
			targetLevel: 1,
			classification: buildClassification(
				[3]uint{0, 0, 0},
				[3]uint{1, 1, 1},
			),
			want: []morton.Z{3},
		},
		{
			name:        "register entire row",
			x:           3,
			y:           0,
			targetLevel: 2,
			classification: buildClassification(
				[3]uint{0, 0, 0},
				[3]uint{1, 0, 0},
				[3]uint{1, 1, 0},
				[3]uint{2, 0, 0},
				[3]uint{2, 1, 0},
				[3]uint{2, 2, 0},
				[3]uint{2, 3, 0},
			),
			want: []morton.Z{0, 1, 4, 5},
		},
		{
			name:        "corners of grid populated",
			x:           7,
			y:           7,
			targetLevel: 3,
			classification: buildClassification(
				[3]uint{0, 0, 0},
				[3]uint{1, 0, 0}, [3]uint{1, 0, 1}, [3]uint{1, 1, 0}, [3]uint{1, 1, 1},
				[3]uint{2, 0, 0}, [3]uint{2, 0, 3}, [3]uint{2, 3, 0}, [3]uint{2, 3, 3},
				[3]uint{3, 0, 0}, [3]uint{3, 7, 0}, [3]uint{3, 0, 7}, [3]uint{3, 7, 7},
			),
			want: []morton.Z{42, 63},
		},
		{
			name:        "general test with two disjoint tiles to pick up",
			x:           6,
			y:           7,
			targetLevel: 3,
			classification: buildClassification(
				[3]uint{0, 0, 0},
				[3]uint{1, 0, 1}, [3]uint{1, 1, 1},
				[3]uint{2, 0, 3}, [3]uint{2, 2, 3}, [3]uint{2, 3, 3},
				[3]uint{3, 0, 7}, [3]uint{2, 6, 1}, [3]uint{3, 5, 7}, [3]uint{3, 7, 7},
			),
			want: []morton.Z{42, 59},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := newSimplePointIndex(tt.targetLevel, 1.0)
			got := ix.findIntersectingTilesLeft(tt.x, tt.y, tt.targetLevel, tt.classification)
			t.Logf("findIntersectingTilesLeft(%d, %d, %d) = %v", tt.x, tt.y, tt.targetLevel, got)
			assert.Equal(t, tt.want, got)
		})
	}
}

func squarePolygon(minX, minY, maxX, maxY float64) geom.Polygon {
	return geom.Polygon{
		{{minX, minY}, {maxX, minY}, {maxX, maxY}, {minX, maxY}},
	}
}

func squareWithHolePolygon(minX, minY, maxX, maxY, holeMinX, holeMinY, holeMaxX, holeMaxY float64) geom.Polygon {
	return geom.Polygon{
		{{minX, minY}, {maxX, minY}, {maxX, maxY}, {minX, maxY}},
		{{holeMinX, holeMinY}, {holeMinX, holeMaxY}, {holeMaxX, holeMaxY}, {holeMaxX, holeMinY}},
	}
}

func trianglePolygon(x1, y1, x2, y2, x3, y3 float64) geom.Polygon {
	return geom.Polygon{
		{{x1, y1}, {x2, y2}, {x3, y3}},
	}
}

func TestPointIndex_classifyNonIntersectingTile(t *testing.T) {
	tests := []struct {
		name         string
		ix           *PointIndex
		polygon      geom.Polygon
		tmsID        tms20.TMID
		buffer       uint
		tileLevel    Level
		tileX, tileY uint
		want         TileClassification
	}{
		{
			name:      "tile outside a centered square",
			ix:        newSimplePointIndex(3, 1.0),
			polygon:   squarePolygon(2, 2, 6, 6),
			tmsID:     3,
			buffer:    0,
			tileLevel: 3,
			tileX:     0,
			tileY:     0,
			want:      ClassificationOutside,
		},
		{
			name:      "tile outside a centered square",
			ix:        newSimplePointIndex(3, 1.5),
			polygon:   squarePolygon(3, 3, 9, 9),
			tmsID:     3,
			buffer:    0,
			tileLevel: 3,
			tileX:     0,
			tileY:     0,
			want:      ClassificationOutside,
		},
		{
			name:      "tile inside a centered square",
			ix:        newSimplePointIndex(3, 1.0),
			polygon:   squarePolygon(2, 2, 6, 6),
			tmsID:     3,
			buffer:    0,
			tileLevel: 3,
			tileX:     3,
			tileY:     3,
			want:      ClassificationInside,
		},
		{
			name:      "higher-level tile inside a centered square",
			ix:        newSimplePointIndex(3, 1.0),
			polygon:   squarePolygon(1, 1, 6, 6),
			tmsID:     3,
			buffer:    0,
			tileLevel: 2,
			tileX:     1,
			tileY:     1,
			want:      ClassificationInside,
		},
		{
			name:      "tile clearly outside, opposite corner",
			ix:        newSimplePointIndex(3, 1.0),
			polygon:   squarePolygon(2, 2, 6, 6),
			tmsID:     3,
			buffer:    0,
			tileLevel: 3,
			tileX:     7,
			tileY:     7,
			want:      ClassificationOutside,
		},
		{
			name: "tile inside the hole of a donut polygon",
			ix:   newSimplePointIndex(3, 1.0),
			// This should work with hole 3,3,5,5 but lineIntesects is buggy
			polygon:   squareWithHolePolygon(0, 0, 7, 7, 3, 3, 6, 6),
			tmsID:     3,
			buffer:    0,
			tileLevel: 3,
			tileX:     4,
			tileY:     4,
			want:      ClassificationOutside,
		},
		{
			name:      "tile in the solid part of a donut polygon",
			ix:        newSimplePointIndex(3, 1.0),
			polygon:   squareWithHolePolygon(0, 0, 8, 8, 3, 3, 5, 5),
			tmsID:     3,
			buffer:    0,
			tileLevel: 3,
			tileX:     1,
			tileY:     1,
			want:      ClassificationInside,
		},
		{
			name:      "tile in the solid part of a donut polygon, far corner",
			ix:        newSimplePointIndex(3, 1.0),
			polygon:   squareWithHolePolygon(0, 0, 8, 8, 3, 3, 6, 6),
			tmsID:     3,
			buffer:    0,
			tileLevel: 3,
			tileX:     7,
			tileY:     7,
			want:      ClassificationOutside,
		},
		{
			name:      "tile below a diagonal triangle (raycast hits intersection)",
			ix:        newSimplePointIndex(3, 1.0),
			polygon:   trianglePolygon(0, 0, 0, 7, 7, 7),
			tmsID:     3,
			buffer:    0,
			tileLevel: 3,
			tileX:     1,
			tileY:     0,
			want:      ClassificationOutside,
		},
		{
			name:      "tile above a diagonal triangle (raycast hits intersection)",
			ix:        newSimplePointIndex(3, 1.0),
			polygon:   trianglePolygon(0, 0, 7, 0, 0, 7),
			tmsID:     3,
			buffer:    0,
			tileLevel: 3,
			tileX:     1,
			tileY:     7,
			want:      ClassificationOutside,
		},
		{
			name:      "nonzero buffer around a centered square",
			ix:        newSimplePointIndex(3, 1.0),
			polygon:   squarePolygon(2, 2, 6, 6),
			tmsID:     3,
			buffer:    4,
			tileLevel: 3,
			tileX:     0,
			tileY:     0,
			want:      ClassificationOutside,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments, classification := tt.ix.lineTracePolygonEdges(tt.polygon, tt.tmsID, tt.buffer)
			z := morton.MustToZ(tt.tileX, tt.tileY)
			got := tt.ix.fillTile(z, tt.tileLevel, segments, classification, tt.polygon)
			t.Logf("classifyNonIntersectingTile(tile=(%d,%d)) = %v", tt.tileX, tt.tileY, got)
			assert.Equal(t, tt.want, got)
		})
	}
}

// convert classification data to visual test data
func classificationGrid(classification map[Level]map[morton.Z]TileClassification, targetLevel Level) map[Level][][]TileClassification {
	grid := make(map[Level][][]TileClassification, targetLevel+1)
	for l := Level(0); l <= targetLevel; l++ {
		levelClassification := classification[l]
		size := uint(1) << l
		rows := make([][]TileClassification, size)
		for y := range size {
			row := make([]TileClassification, size)
			for x := range size {
				z := morton.MustToZ(x, y)
				if c, ok := levelClassification[z]; ok {
					row[x] = c
				} else {
					row[x] = ClassificationUnknown
				}
			}
			rows[y] = row
		}
		grid[l] = rows
	}
	return grid
}

func TestPointIndex_classifyNonIntersectingTiles(t *testing.T) {
	const (
		u = ClassificationUnknown
		x = ClassificationIntersect
		i = ClassificationInside
		o = ClassificationOutside
	)
	tests := []struct {
		name    string
		ix      *PointIndex
		polygon geom.Polygon
		tmsID   tms20.TMID
		buffer  uint
		want    map[Level][][]TileClassification
	}{
		{
			name:    "centered square",
			ix:      newSimplePointIndex(3, 1.0),
			polygon: squarePolygon(2, 2, 6, 6),
			tmsID:   3,
			buffer:  0,
			want: map[Level][][]TileClassification{
				0: {{x}},
				1: {{x, x}, {x, x}},
				2: {
					{o, x, x, o},
					{x, x, x, x},
					{x, x, x, x},
					{o, x, x, x},
				},
				3: {
					{u, u, o, o, o, o, u, u},
					{u, u, x, x, x, x, u, u},
					{o, x, x, x, x, x, x, o},
					{o, x, x, i, i, x, x, o},
					{o, x, x, i, i, x, x, o},
					{o, x, x, x, x, x, x, o},
					{u, u, x, x, x, x, x, o},
					{u, u, o, o, o, o, o, o},
				},
			},
		},
		{
			name:    "donut polygon (square with a square hole)",
			ix:      newSimplePointIndex(3, 1.0),
			polygon: squareWithHolePolygon(0, 0, 7, 7, 3, 3, 5, 5),
			tmsID:   3,
			buffer:  0,
			want: map[Level][][]TileClassification{
				0: {{x}},
				1: {{x, x}, {x, x}},
				2: {
					{x, x, x, x},
					{x, x, x, x},
					{x, x, x, x},
					{x, x, x, x},
				},
				3: {
					{x, x, x, x, x, x, x, x},
					{x, i, i, i, i, i, x, x},
					{x, i, i, x, x, i, x, x},
					{x, i, x, x, x, x, x, x},
					{x, i, x, x, x, x, x, x},
					{x, i, i, x, x, x, x, x},
					{x, x, x, x, x, x, x, x},
					{x, x, x, x, x, x, x, x},
				},
			},
		},
		{
			name:    "diagonal triangle",
			ix:      newSimplePointIndex(3, 1.0),
			polygon: trianglePolygon(0, 0, 7, 0, 7, 7),
			tmsID:   3,
			buffer:  0,
			want: map[Level][][]TileClassification{
				0: {{x}},
				1: {{x, x}, {x, x}},
				2: {
					{x, x, x, x},
					{x, x, x, x},
					{o, x, x, x},
					{o, o, x, x},
				},
				3: {
					{x, x, x, x, x, x, x, x},
					{x, x, x, i, i, i, x, x},
					{o, x, x, x, i, i, x, x},
					{o, o, x, x, x, i, x, x},
					{u, u, o, x, x, x, x, x},
					{u, u, o, o, x, x, x, x},
					{u, u, u, u, o, x, x, x},
					{u, u, u, u, o, o, o, x},
				},
			},
		},
		{
			name:    "polygon covering the whole extent",
			ix:      newSimplePointIndex(2, 1.0),
			polygon: squarePolygon(0, 0, 4, 4),
			tmsID:   2,
			buffer:  0,
			want: map[Level][][]TileClassification{
				0: {{x}},
				1: {{x, x}, {x, x}},
				2: {
					{x, x, x, x},
					{x, i, i, x},
					{x, i, i, x},
					{x, x, x, x},
				},
			},
		},
		{
			name:    "tiny polygon in a single corner tile",
			ix:      newSimplePointIndex(3, 1.0),
			polygon: squarePolygon(0.25, 0.25, 0.75, 0.75),
			tmsID:   3,
			buffer:  0,
			want: map[Level][][]TileClassification{
				0: {{x}},
				1: {{x, o}, {o, o}},
				2: {
					{x, o, u, u},
					{o, o, u, u},
					{u, u, u, u},
					{u, u, u, u},
				},
				3: {
					{x, o, u, u, u, u, u, u},
					{o, o, u, u, u, u, u, u},
					{u, u, u, u, u, u, u, u},
					{u, u, u, u, u, u, u, u},
					{u, u, u, u, u, u, u, u},
					{u, u, u, u, u, u, u, u},
					{u, u, u, u, u, u, u, u},
					{u, u, u, u, u, u, u, u},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetLevel := Level(tt.tmsID) //nolint:gosec // G115
			segments, classification := tt.ix.lineTracePolygonEdges(tt.polygon, tt.tmsID, tt.buffer)
			tt.ix.fillTiles(targetLevel, 0, 0, true, segments, classification, tt.polygon)

			got := classificationGrid(classification, targetLevel)
			for l := Level(0); l <= targetLevel; l++ {
				t.Logf("level %d: %v", l, got[l])
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
