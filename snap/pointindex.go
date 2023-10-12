package snap

import (
	"fmt"
	"io"
	"math"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/wkt"
	"github.com/go-spatial/geom/planar"
)

const (
	xAx = 0
	yAx = 1
	// left        = 0b00
	right = 0b01
	// bottom      = 0b00
	top = 0b10
	// bottomleft  = bottom | left  // 0b00
	// bottomright = bottom | right // 0b01
	// topleft     = top | left     // 0b10
	// topright    = top | right    // 0b11
)

// PointIndex is a pointcloud annex quadtree to enable snapping lines to a grid accounting for those points.
// Quadrants:
//
//	|-------|
//	| 2 | 3 |
//	|-------|
//	| 0 | 1 |
//	|-------|
//
// Edges:
//
//	          exc
//	   maxX    2    maxY
//	       |-------|
//	       | 3 | 2 |
//	inc  3 |-------| 1  exc
//	       | 0 | 1 |
//	       |-------|
//	   minX    0    maxX
//	          inc
type PointIndex struct {
	rootMatrix *TileMatrix
	level      uint
	x          int         // from the left
	y          int         // from the bottom
	extent     geom.Extent // maxX and MaxY are exclusive
	centroid   geom.Point
	hasPoints  bool
	maxDepth   uint // 0 means this is a leaf
	quadrants  [4]*PointIndex
}

func NewPointIndexFromTileMatrix(tm TileMatrix) *PointIndex {
	// TODO support actual TMS or slippy grid
	maxDepth := int(float64(tm.Level) + math.Log2(float64(tm.TileSize)) + math.Log2(float64(tm.PixelSize)))
	extent, centroid := getQuadrantExtentAndCentroid(&tm, 0, 0, 0)
	ix := PointIndex{
		rootMatrix: &tm,
		level:      0, // TODO maybe adjust for actual matrixId
		x:          0,
		y:          0,
		maxDepth:   uint(maxDepth),
		extent:     extent,
		centroid:   centroid,
	}
	return &ix
}

// InsertPolygon inserts all points from a Polygon
func (ix *PointIndex) InsertPolygon(polygon *geom.Polygon) {
	for _, ring := range polygon.LinearRings() {
		for _, vertex := range ring {
			ix.InsertPoint(vertex)
		}
	}
}

// InsertPoint inserts a Point by its absolute coord
func (ix *PointIndex) InsertPoint(point geom.Point) {
	deepestSize := pow2(ix.maxDepth)
	deepestRes := ix.extent.XSpan() / float64(deepestSize)
	deepestX := int(math.Floor((point.X() - ix.extent.MinX()) / deepestRes))
	deepestY := int(math.Floor((point.Y() - ix.extent.MinY()) / deepestRes))
	ix.InsertCoord(deepestX, deepestY)
}

// InsertCoord inserts a Point by its x/y coord on the deepest level
func (ix *PointIndex) InsertCoord(deepestX int, deepestY int) {
	// TODO panic if outside grid
	deepestSize := int(pow2(ix.maxDepth))
	if deepestX < 0 || deepestY < 0 || deepestX > deepestSize-1 || deepestY > deepestSize-1 {
		return // the point is outside the extent
	}
	ix.insertCoord(deepestX, deepestY)
}

// insertCoord adds a point into this pc, assuming the point is inside its extent
func (ix *PointIndex) insertCoord(deepestX int, deepestY int) {
	ix.hasPoints = true
	if ix.isLeaf() {
		return
	}

	// insert into one of the quadrants
	indexSize := int(pow2(ix.maxDepth))
	xAtDeepest := ix.x * indexSize
	yAtDeepest := ix.y * indexSize

	isRight := bool2int(deepestX >= xAtDeepest+indexSize/2)
	isTop := bool2int(deepestY >= yAtDeepest+indexSize/2) << 1
	quadrantI := isRight | isTop
	ix.ensureQuadrant(quadrantI).insertCoord(deepestX, deepestY)
}

func (ix *PointIndex) ensureQuadrant(quadrantI int) *PointIndex {
	if ix.quadrants[quadrantI] == nil {
		x := ix.x*2 + oneIfRight(quadrantI)
		y := ix.y*2 + oneIfTop(quadrantI)
		level := ix.level + 1
		extent, centroid := getQuadrantExtentAndCentroid(ix.rootMatrix, level, x, y)
		ix.quadrants[quadrantI] = &PointIndex{
			rootMatrix: ix.rootMatrix,
			level:      level,
			x:          x,
			y:          y,
			extent:     extent,
			centroid:   centroid,
			maxDepth:   ix.maxDepth - 1,
		}
	}
	return ix.quadrants[quadrantI]
}

func (ix *PointIndex) isLeaf() bool {
	return ix.maxDepth == 0
}

func getQuadrantExtentAndCentroid(rootMatrix *TileMatrix, level uint, x, y int) (geom.Extent, geom.Point) {
	span := rootMatrix.GridSize() / float64(pow2(level))
	return geom.Extent{
			rootMatrix.MinX + float64(x)*span,     // minx
			rootMatrix.MinY() + float64(y)*span,   // miny
			rootMatrix.MinX + float64(x+1)*span,   // maxx
			rootMatrix.MinY() + float64(y+1)*span, // maxy
		}, geom.Point{
			rootMatrix.MinX + (float64(x)+0.5)*span, // <-- here is the plus 0.5 internal pixel size
			rootMatrix.MinY() + (float64(y)+0.5)*span,
		}
}

func (ix *PointIndex) SnapClosestPoints(line geom.Line) [][2]float64 {
	pointIndices := ix.snapClosestPoints(line, ix.maxDepth, false)
	points := make([][2]float64, len(pointIndices))
	for i, ixWithPoint := range pointIndices {
		points[i] = ixWithPoint.centroid
	}
	return points
}

//nolint:cyclop
func (ix *PointIndex) snapClosestPoints(line geom.Line, depth uint, certainlyIntersects bool) []*PointIndex {
	if !ix.hasPoints {
		return nil
	}
	if !certainlyIntersects && !ix.lineIntersects(line) {
		return nil
	}
	if depth == 0 {
		return []*PointIndex{ix}
	}
	var quadrantsToCheck []quadrantToCheck

	pt1InfiniteQuadrantI := ix.getInfiniteQuadrant(line[0])
	pt1IsInsideQuadrant := ix.containsPoint(line[0])
	pt2InfiniteQuadrantI := ix.getInfiniteQuadrant(line[1])
	pt2IsInsideQuadrant := ix.containsPoint(line[1])

	if pt1InfiniteQuadrantI == pt2InfiniteQuadrantI {
		// line intersects at most this quadrant
		if pt1IsInsideQuadrant && pt2IsInsideQuadrant {
			// line intersects for sure this quadrant (only)
			quadrantsToCheck = []quadrantToCheck{{pt1InfiniteQuadrantI, true, false}}
		} else {
			quadrantsToCheck = []quadrantToCheck{{pt1InfiniteQuadrantI, false, false}} // TODO check only outside edges
		}
	} else if quadrantsAreAdjacent(pt1InfiniteQuadrantI, pt2InfiniteQuadrantI) {
		// line intersects at most these two quadrants
		if pt1IsInsideQuadrant && pt2IsInsideQuadrant {
			quadrantsToCheck = []quadrantToCheck{
				{pt1InfiniteQuadrantI, true, false},
				{pt2InfiniteQuadrantI, true, false},
			}
		} else {
			quadrantsToCheck = []quadrantToCheck{
				{pt1InfiniteQuadrantI, false, false},
				{pt2InfiniteQuadrantI, false, false},
			} // TODO only need to check the outside edges of the quadrant
		}
	} else {
		// line intersects at most three quadrants, but don't know which ones
		if pt1IsInsideQuadrant {
			if pt2IsInsideQuadrant { // both points inside quadrant
				quadrantsToCheck = []quadrantToCheck{
					{pt1InfiniteQuadrantI, true, false},
					{adjacentQuadrantX(pt1InfiniteQuadrantI), false, true},
					{adjacentQuadrantY(pt1InfiniteQuadrantI), false, true},
					{pt2InfiniteQuadrantI, true, false},
				}
			} else { // pt1 inside, pt2 outside
				quadrantsToCheck = []quadrantToCheck{
					{pt1InfiniteQuadrantI, true, false},
					{adjacentQuadrantX(pt1InfiniteQuadrantI), false, true},
					{adjacentQuadrantY(pt1InfiniteQuadrantI), false, true},
					{pt2InfiniteQuadrantI, false, false},
				}
			}
		} else if pt2IsInsideQuadrant { // pt1 outside, pt2 inside
			quadrantsToCheck = []quadrantToCheck{
				{pt1InfiniteQuadrantI, false, false},
				{adjacentQuadrantX(pt1InfiniteQuadrantI), false, true},
				{adjacentQuadrantY(pt1InfiniteQuadrantI), false, true},
				{pt2InfiniteQuadrantI, true, false},
			}
		} else { // neither inside (worst case)
			quadrantsToCheck = []quadrantToCheck{
				{pt1InfiniteQuadrantI, false, false},
				{adjacentQuadrantX(pt1InfiniteQuadrantI), false, true},
				{adjacentQuadrantY(pt1InfiniteQuadrantI), false, true},
				{pt2InfiniteQuadrantI, false, false},
			}
		}
	}

	// Check all the possible quadrants
	var intersectedQuadrantsWithPoints []*PointIndex
	mutexed := false
	for _, quadrantToCheck := range quadrantsToCheck {
		if quadrantToCheck.mutex && mutexed {
			continue
		}
		var found []*PointIndex
		quadrant := ix.quadrants[quadrantToCheck.i]
		if quadrant != nil {
			found = quadrant.snapClosestPoints(line, depth-1, quadrantToCheck.certain)
		}
		if quadrantToCheck.mutex && len(found) > 0 {
			mutexed = true
		}
		intersectedQuadrantsWithPoints = append(intersectedQuadrantsWithPoints, found...)
	}
	return intersectedQuadrantsWithPoints
}

// containsPoint checks whether a point is contained in the extent.
func (ix *PointIndex) containsPoint(pt geom.Point) bool {
	// Differs from geom.(Extent)containsPoint() by not including the right and top edges
	return ix.extent.MinX() <= pt[0] && pt[0] < ix.extent.MaxX() &&
		ix.extent.MinY() <= pt[1] && pt[1] < ix.extent.MaxY()
}

// A quadrant to check for an intersecting line and having points
type quadrantToCheck struct {
	i       int  // quadrant number
	certain bool // whether the line certainly intersects this quadrant (true) or needs to be checked (false)
	mutex   bool // if the line intersects this one, the other cannot be intersected
	// TODO account for edge cases: lines on, or stopping at the exclusive extent edges
	// TODO not only exclusive lines but also last/first point on a line
}

// getInfiniteQuadrant determines in which infinite quadrant the point lies
func (ix *PointIndex) getInfiniteQuadrant(point geom.Point) int {
	centroid := ix.centroid
	isRight := bool2int(point.X() >= centroid.X())
	isTop := bool2int(point.Y() >= centroid.Y()) << 1
	return isRight | isTop
}

func quadrantsAreAdjacent(quadrantIA, quadrantIB int) bool {
	diff := quadrantIA ^ quadrantIB
	return diff == 0b01 || diff == 0b10
}

func adjacentQuadrantX(quadrantI int) int {
	return quadrantI ^ 0b01
}

func adjacentQuadrantY(quadrantI int) int {
	return quadrantI ^ 0b10
}

// lineIntersects tests whether a line intersects with the extent.
// TODO this can probably be faster by reusing the edges for the other three quadrants and/or only testing relevant edges (hints)
func (ix *PointIndex) lineIntersects(line geom.Line) bool {
	// First see if a point is inside (cheap test).
	pt1IsInsideQuadrant := ix.containsPoint(line[0])
	pt2IsInsideQuadrant := ix.containsPoint(line[1])
	if pt1IsInsideQuadrant || pt2IsInsideQuadrant {
		return true
	}

	for edgeI, edge := range ix.extent.Edges(nil) {
		intersection, intersects := planar.SegmentIntersect(line, edge)
		// Checking for intersection cq crossing is not enough. The right and top edges are exclusive.
		// So there are exceptions ...:
		if intersects { //nolint:nestif
			if isExclusiveEdge(edgeI) {
				if line[0] == intersection || line[1] == intersection {
					// The tip of a line coming from the outside touches the (exclusive) edge.
					continue
				}
			} else {
				// The tip of a line coming from the outside touches the exclusive tip of an inclusive edge.
				exclusivePoint := getExclusiveTip(edgeI, edge)
				if line[0] == exclusivePoint || line[1] == exclusivePoint {
					continue
				}
			}
			return true
		} else if !isExclusiveEdge(edgeI) && lineOverlapsInclusiveEdge(line, edgeI, edge) {
			// No intersection but overlap on an inclusive edge.
			return true
		}
	}
	return false
}

func isExclusiveEdge(edgeI int) bool {
	i := edgeI % 4
	return i == 1 || i == 2
}

// getExclusiveTip returns the tip point of an inclusive edge that is not-inclusive
func getExclusiveTip(edgeI int, edge geom.Line) geom.Point {
	i := edgeI % 4
	if i == 0 {
		return edge[1]
	} else if i == 3 {
		return edge[0]
	}
	panic(fmt.Sprintf("not an inclusive edge: %v", edgeI))
}

// lineOverlapsInclusiveEdge helps to check if a line overlaps an inclusive edge (excluding the exclusive tip)
func lineOverlapsInclusiveEdge(line geom.Line, edgeI int, edge geom.Line) bool {
	var constAx, varAx int
	switch {
	case edge[0][xAx] == edge[1][xAx]:
		constAx = xAx
		varAx = yAx
	case edge[0][yAx] == edge[1][yAx]:
		constAx = yAx
		varAx = xAx
	default:
		panic(fmt.Sprintf("not a straight edge: %v", edge))
	}
	eConstOrd := edge[0][constAx]
	if line[0][constAx] != eConstOrd || line[1][constAx] != eConstOrd {
		return false // not a straight line and/or not on same line as the edge, so no overlap
	}
	eOrd1 := edge[0][varAx]
	eOrd2 := edge[1][varAx]

	exclusiveTip := getExclusiveTip(edgeI, edge)
	// if exclusiveTip[constAx] != eConstOrd || !betweenInc(exclusiveTip[varAx], eOrd1, eOrd2) {
	// 	 panic(fmt.Sprintf("exclusive point not on edge: %v, %v", exclusiveTip, edge))
	// }
	lOrd1 := line[0][varAx]
	lOrd2 := line[1][varAx]
	return lOrd1 != lOrd2 && (betweenInc(lOrd1, eOrd1, eOrd2) && line[0] != exclusiveTip || betweenInc(lOrd2, eOrd1, eOrd2) && line[1] != exclusiveTip)
}

func betweenInc(f, p, q float64) bool {
	if p <= q {
		return p <= f && f <= q
	}
	return q <= f && f <= p
}

func pow2(n uint) uint {
	return 1 << n
}

func bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
}

func oneIfLeft(quadrantI int) int {
	return oneIfRight(quadrantI) ^ 1
}
func oneIfRight(quadrantI int) int {
	return quadrantI & right
}
func oneIfBottom(quadrantI int) int {
	return oneIfTop(quadrantI) ^ 1
}
func oneIfTop(quadrantI int) int {
	return (quadrantI & top) >> 1
}

// toWkt creates a WKT representation of the pointcloud. For debugging/visualising.
func (ix *PointIndex) toWkt(writer io.Writer) {
	_ = wkt.Encode(writer, ix.extent)
	_, _ = fmt.Fprintf(writer, "\n")
	if ix.isLeaf() && ix.hasPoints {
		centroid := ix.centroid
		_ = wkt.Encode(writer, centroid)
		_, _ = fmt.Fprintf(writer, "\n")
	}
	for _, quadrant := range ix.quadrants {
		if quadrant != nil {
			quadrant.toWkt(writer)
		}
	}
}
