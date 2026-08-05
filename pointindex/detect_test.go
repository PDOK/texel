package pointindex

import (
	"sort"
	"testing"

	"github.com/go-spatial/geom"
	"github.com/stretchr/testify/assert"

	"github.com/pdok/texel/intgeom"
	"github.com/pdok/texel/mathhelp"
)

// registeredTile records a single call to a RegisterFunc during a test.
type registeredTile struct {
	x, y       uint
	segmentIdx SegmentIdx
}

// Register function for testing
func recordingRegister(dst *[]registeredTile) RegisterFunc {
	return func(tileX, tileY uint, l Level, segmentIdx SegmentIdx) {
		maxCoord := mathhelp.Pow2(l) - 1 //nolint:gosec // level should fit max coords
		if tileX > maxCoord || tileY > maxCoord {
			return
		}
		*dst = append(*dst, registeredTile{tileX, tileY, segmentIdx})
	}
}

// uniqueTileCoords reduces recorded tiles to the (deduplicated, sorted) set
// of (x, y) tile coordinates touched. registerQuadrant/register is expected
// to be idempotent, so functionally only the set of touched tiles matters,
// not how many times or in what order each one was registered.
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

// newOffsetPointIndex builds a minimal PointIndex covering a
// cellSize*2^deepestLevel square, with its bottom-left corner at
// (originX, originY) instead of (0, 0). For testing the lineTrace
// function
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
		// --- Group A: plain raycast, no buffer, levelDiff > 0 (tileSize > deepestRes) ---
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

		// --- Group A with buffer: same diagonals, but a positive buffer must
		// pull in extra tiles alongside the ones already found above. Uses a
		// bigger grid (deepestLevel 4, same tileSize 2) so the buffer doesn't
		// reach past the grid's own edge. ---
		{
			name:         "buffer, +x+y diagonal: extra tile near start",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 2}, geom.Point{14, 9}},
			buffer: 2,
			want:   [][2]uint{{0, 0}, {0, 1}, {1, 0}, {1, 1}, {1, 2}, {2, 0}, {2, 1}, {2, 2}, {3, 1}, {3, 2}},
		},
		{
			name:         "buffer, -x-y diagonal: extra tiles near start",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{14, 9}, geom.Point{2, 2}},
			buffer: 2,
			want:   [][2]uint{{0, 0}, {0, 1}, {1, 0}, {1, 1}, {1, 2}, {2, 0}, {2, 1}, {2, 2}, {3, 1}, {3, 2}},
		},
		{
			name:         "buffer, +x-y diagonal: extra tiles near start",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 10}, geom.Point{14, 2}},
			buffer: 2,
			want:   [][2]uint{{0, 1}, {0, 2}, {0, 3}, {1, 0}, {1, 1}, {1, 2}, {1, 3}, {2, 0}, {2, 1}, {2, 2}, {3, 0}, {3, 1}},
		},
		{
			name:         "buffer, -x+y diagonal: extra tiles near start",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{14, 2}, geom.Point{2, 10}},
			buffer: 2,
			want:   [][2]uint{{0, 1}, {0, 2}, {0, 3}, {1, 0}, {1, 1}, {1, 2}, {1, 3}, {2, 0}, {2, 1}, {2, 2}, {3, 0}, {3, 1}},
		},

		// --- Degenerate axis-aligned lines: documented pre-existing limitation ---
		// (dx=0 or dy=0 makes D == 0, so the traversal loop never advances;
		// only the start tile - and its buffered neighbors - are registered.)
		{
			name:         "degenerate horizontal line only registers start tile",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 5}, geom.Point{14, 5}},
			buffer: 0,
			want:   [][2]uint{{0, 1}, {1, 1}, {2, 1}, {3, 1}},
		},
		{
			name:         "degenerate vertical line only registers start tile",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{5, 2}, geom.Point{5, 14}},
			buffer: 0,
			want:   [][2]uint{{1, 0}, {1, 1}, {1, 2}, {1, 3}},
		},

		// --- Group B: buffer inflates the start-point registration ---
		{
			name:         "end in corner, no buffer, no registering of other tiles",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 2}, geom.Point{3, 3}},
			buffer: 0,
			want:   [][2]uint{{0, 0}},
		},
		{
			name:         "end in corner, with buffer, register tiles across edge",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{2, 2}, geom.Point{3, 3}},
			buffer: 1,
			want:   [][2]uint{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
		},
		{
			name:         "start near edge, buffer registers neighbour tile",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{1, 3}, geom.Point{2, 3}},
			buffer: 1,
			want:   [][2]uint{{0, 0}, {0, 1}},
		},

		// --- Group C: buffer makes the traversal loop run longer, reaching
		// extra tiles it wouldn't otherwise touch near the segment's end ---
		{
			name:         "no buffer: diagonal crossing a shared corner touches 4 tiles",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{6, 6}, geom.Point{10, 10}},
			buffer: 0,
			want:   [][2]uint{{1, 1}, {1, 2}, {2, 1}, {2, 2}},
		},
		{
			name:         "small buffer: diagonal crossing of buffer boundary touches 4 tiles",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{6, 6}, geom.Point{7, 7}},
			buffer: 1,
			want:   [][2]uint{{1, 1}, {1, 2}, {2, 1}, {2, 2}},
		},
		{
			name:         "buffer of line clips upper right corner of tile, not detected",
			deepestLevel: 4, l: 2, cellSize: 1.0,
			line:   geom.Line{geom.Point{12, 14}, geom.Point{14, 12}},
			buffer: 1,
			want:   [][2]uint{{2, 2}, {2, 3}, {3, 2}, {3, 3}}, // Arguably {2,2} should not be here
		},
		// --- Group E: non-zero grid origin (MinX/MinY offset bug fix) ---
		// Same relative geometry as the "+x+y diagonal" case above, translated
		// by (+100, +100): the resulting tile pattern must be identical,
		// proving intExtent.MinX()/MinY() are correctly taken into account.
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
			ix.lineTrace(tt.line, tt.l, tt.deepestLevel, 0, 0, tt.buffer, recordingRegister(&recorded))

			got := uniqueTileCoords(recorded)
			assert.Equal(t, tt.want, got)
		})
	}
}
