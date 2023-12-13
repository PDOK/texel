package snap

import (
	"fmt"
	"io"
	"slices"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/wkt"
	"github.com/pdok/texel/intgeom"
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
	intHalf = 500000000
	intOne  = 1000000000
)

type Quadrant struct {
	z           Z
	intExtent   intgeom.Extent // maxX and MaxY are exclusive
	intCentroid intgeom.Point
}

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
	Quadrant
	maxDepth    Level
	quadrants   map[Level]map[Z]Quadrant
	hitOnce     map[Z]map[intgeom.Point][]int
	hitMultiple map[Z]map[intgeom.Point][]int
}

type Level = uint
type Q = int // quadrant index (0, 1, 2 or 3)

// InsertPolygon inserts all points from a Polygon
func (ix *PointIndex) InsertPolygon(polygon geom.Polygon) {
	// initialize the quadrants map
	pointsCount := 0
	for _, ring := range polygon.LinearRings() {
		pointsCount += len(ring)
	}
	var level uint
	for level = 0; level <= ix.maxDepth; level++ {
		if ix.quadrants[level] == nil {
			ix.quadrants[level] = make(map[Z]Quadrant, pointsCount) // TODO smaller for the shallower levels
		}
		// TODO expand for another polygon?
	}

	for _, ring := range polygon.LinearRings() {
		for _, vertex := range ring {
			ix.InsertPoint(vertex)
		}
	}
}

// InsertPoint inserts a Point by its absolute coord
func (ix *PointIndex) InsertPoint(point geom.Point) {
	deepestSize := pow2(ix.maxDepth)
	intDeepestRes := ix.intExtent.XSpan() / int64(deepestSize)
	intPoint := intgeom.FromGeomPoint(point)
	deepestX := int((intPoint.X() - ix.intExtent.MinX()) / intDeepestRes)
	deepestY := int((intPoint.Y() - ix.intExtent.MinY()) / intDeepestRes)
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
	var l Level
	for l = 0; l <= ix.maxDepth; l++ {
		x := uint(deepestX) / pow2(ix.maxDepth-l)
		y := uint(deepestY) / pow2(ix.maxDepth-l)
		z := mustToZ(x, y)
		if ix.quadrants[l] == nil { // probably already initialized by InsertPolygon
			ix.quadrants[l] = make(map[Z]Quadrant)
		}
		extent, centroid := getQuadrantExtentAndCentroid(l, x, y, ix.intExtent)
		ix.quadrants[l][z] = Quadrant{
			z:           z,
			intExtent:   extent,
			intCentroid: centroid,
		}
	}
}

func getQuadrantExtentAndCentroid(level Level, x, y uint, intRootExtent intgeom.Extent) (intgeom.Extent, intgeom.Point) {
	intQuadrantSpan := intRootExtent.XSpan() / int64(pow2(level))
	intMinX := intRootExtent.MinX()
	intMinY := intRootExtent.MinY()
	intExtent := intgeom.Extent{
		intMinX + int64(x)*intQuadrantSpan,   // minx
		intMinY + int64(y)*intQuadrantSpan,   // miny
		intMinX + int64(x+1)*intQuadrantSpan, // maxx
		intMinY + int64(y+1)*intQuadrantSpan, // maxy
	}
	intCentroid := intgeom.Point{
		intMinX + (int64(x))*intQuadrantSpan + intQuadrantSpan/2, // <-- here is the plus 0.5 internal pixel size
		intMinY + (int64(y))*intQuadrantSpan + intQuadrantSpan/2,
	}
	return intExtent, intCentroid
}

// SnapClosestPoints returns the points (centroids) in the index that are intersected by a line
// on multiple levels
func (ix *PointIndex) SnapClosestPoints(line geom.Line, levelMap map[Level]any, ringID int) map[Level][][2]float64 {
	intLine := intgeom.FromGeomLine(line)
	quadrantsPerLevel := ix.snapClosestPoints(intLine, levelMap)

	pointsPerLevel := make(map[Level][][2]float64, len(levelMap))
	for level, quadrants := range quadrantsPerLevel {
		if len(quadrants) == 0 {
			continue
		}
		if ix.hitOnce[level] == nil {
			ix.hitOnce[level] = make(map[intgeom.Point][]int)
		}
		if ix.hitMultiple[level] == nil {
			ix.hitMultiple[level] = make(map[intgeom.Point][]int)
		}
		points := make([][2]float64, len(quadrants))
		for i, quadrant := range quadrants {
			points[i] = quadrant.intCentroid.ToGeomPoint()
			// ignore first point to avoid superfluous duplicates
			if i > 0 {
				checkPointHits(ix, quadrant.intCentroid, ringID, level)
			}
		}
		pointsPerLevel[level] = points
	}
	return pointsPerLevel
}

func (ix *PointIndex) snapClosestPoints(intLine intgeom.Line, levelMap map[Level]any) map[Level][]Quadrant {
	if !lineIntersects(intLine, ix.intExtent) {
		return nil
	}
	quadrantsIntersectedPerLevel := make(map[Level][]Quadrant, len(levelMap))
	parents := []Quadrant{ix.Quadrant}
	if _, includeLevelZero := levelMap[0]; includeLevelZero {
		quadrantsIntersectedPerLevel[0] = parents
	}

	var level Level
	for level = 1; level <= ix.maxDepth; level++ {
		quadrantsIntersected := make([]Quadrant, 0, 10) // TODO good estimate of expected count, based on line length / quadrant span * geom points?
		for _, parent := range parents {
			quadrantZs := getQuadrantZs(parent.z)
			quadrantsWithPoints := make(map[Q]Quadrant, 4)
			for q, quadrantZ := range quadrantZs {
				if quadrant, exists := ix.quadrants[level][quadrantZ]; exists {
					quadrantsWithPoints[q] = quadrant
				}
			}
			for _, q := range findIntersectingQuadrants(intLine, quadrantsWithPoints, parent) {
				quadrantsIntersected = append(quadrantsIntersected, quadrantsWithPoints[q])
			}
		}
		parents = quadrantsIntersected
		if _, isLevelIncluded := levelMap[level]; isLevelIncluded {
			quadrantsIntersectedPerLevel[level] = quadrantsIntersected
		}
	}
	return quadrantsIntersectedPerLevel
}

//nolint:cyclop
func findIntersectingQuadrants(intLine intgeom.Line, quadrants map[Q]Quadrant, parent Quadrant) []Q {
	pt1InfiniteQuadrantI := getInfiniteQuadrant(intLine[0], parent.intCentroid)
	pt1IsInsideQuadrant := containsPoint(intLine[0], parent.intExtent)
	pt2InfiniteQuadrantI := getInfiniteQuadrant(intLine[1], parent.intCentroid)
	pt2IsInsideQuadrant := containsPoint(intLine[1], parent.intExtent)

	var quadrantsToCheck []quadrantToCheck
	switch {
	case pt1InfiniteQuadrantI == pt2InfiniteQuadrantI:
		if pt1IsInsideQuadrant && pt2IsInsideQuadrant {
			// line intersects for sure this quadrant (only)
			quadrantsToCheck = []quadrantToCheck{{pt1InfiniteQuadrantI, true, false}}
		} else {
			quadrantsToCheck = []quadrantToCheck{{pt1InfiniteQuadrantI, false, false}} // TODO check only outside edges
		}
	case quadrantsAreAdjacent(pt1InfiniteQuadrantI, pt2InfiniteQuadrantI):
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
	default:
		switch {
		case pt1IsInsideQuadrant:
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
		case pt2IsInsideQuadrant:
			quadrantsToCheck = []quadrantToCheck{
				{pt1InfiniteQuadrantI, false, false},
				{adjacentQuadrantX(pt1InfiniteQuadrantI), false, true},
				{adjacentQuadrantY(pt1InfiniteQuadrantI), false, true},
				{pt2InfiniteQuadrantI, true, false},
			}
		default:
			quadrantsToCheck = []quadrantToCheck{
				{pt1InfiniteQuadrantI, false, false},
				{adjacentQuadrantX(pt1InfiniteQuadrantI), false, true},
				{adjacentQuadrantY(pt1InfiniteQuadrantI), false, true},
				{pt2InfiniteQuadrantI, false, false},
			}
		}
	}

	found := make([]Q, 0, 4)
	// Check all the possible quadrants
	mutexed := false
	for _, quadrantToCheck := range quadrantsToCheck {
		if quadrantToCheck.mutex && mutexed {
			continue
		}
		quadrant, hasPoints := quadrants[quadrantToCheck.i]
		if !hasPoints {
			continue
		}
		if quadrantToCheck.certain || lineIntersects(intLine, quadrant.intExtent) {
			found = append(found, quadrantToCheck.i)
			if quadrantToCheck.mutex {
				mutexed = true
			}
		}
	}
	return found
}

func getQuadrantZs(parentZ Z) [4]Z {
	parentX, parentY := fromZ(parentZ)
	quadrantZs := [4]Z{}
	for i := 0; i < 4; i++ {
		x := parentX*2 + uint(oneIfRight(i))
		y := parentY*2 + uint(oneIfTop(i))
		z := mustToZ(x, y)
		quadrantZs[i] = z
	}
	return quadrantZs
}

// containsPoint checks whether a point is contained in a quadrant's extent.
func containsPoint(intPt intgeom.Point, intExtent intgeom.Extent) bool {
	// Differs from geom.(Extent)containsPoint() by not including the right and top edges
	return intExtent.MinX() <= intPt[0] && intPt[0] < intExtent.MaxX() &&
		intExtent.MinY() <= intPt[1] && intPt[1] < intExtent.MaxY()
}

// A quadrant to check for an intersecting line and having points
type quadrantToCheck struct {
	i       int  // quadrant number
	certain bool // whether the line certainly intersects this quadrant (true) or needs to be checked (false)
	mutex   bool // if the line intersects this one, the other cannot be intersected
}

// getInfiniteQuadrant determines in which (infinite) quadrant a point lies
func getInfiniteQuadrant(intPt intgeom.Point, intCentroid intgeom.Point) int {
	isRight := bool2int(intPt[0] >= intCentroid[0])
	isTop := bool2int(intPt[1] >= intCentroid[1]) << 1
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

// lineIntersects tests whether a line intersects with an extent.
// TODO this can probably be faster by reusing the edges for the other three quadrants and/or only testing relevant edges (hints)
func lineIntersects(intLine intgeom.Line, intExtent intgeom.Extent) bool {
	// First see if a point is inside (cheap test).
	pt1IsInsideQuadrant := containsPoint(intLine[0], intExtent)
	pt2IsInsideQuadrant := containsPoint(intLine[1], intExtent)
	if pt1IsInsideQuadrant || pt2IsInsideQuadrant {
		return true
	}

	for edgeI, intEdge := range intExtent.Edges(nil) {
		intersection, intersects := intgeom.SegmentIntersect(intLine, intEdge)
		// Checking for intersection cq crossing is not enough. The right and top edges are exclusive.
		// So there are exceptions ...:
		if intersects { //nolint:nestif
			if isExclusiveEdge(edgeI) {
				if intLine[0] == intersection || intLine[1] == intersection {
					// The tip of a line coming from the outside touches the (exclusive) edge.
					continue
				}
			} else {
				// The tip of a line coming from the outside touches the exclusive tip of an inclusive edge.
				exclusivePoint := getExclusiveTip(edgeI, intEdge)
				if intLine[0] == exclusivePoint || intLine[1] == exclusivePoint {
					continue
				}
			}
			return true
		} else if !isExclusiveEdge(edgeI) && lineOverlapsInclusiveEdge(intLine, edgeI, intEdge) {
			// No intersection but overlap on an inclusive edge.
			return true
		}
	}
	return false
}

func checkPointHits(ix *PointIndex, vertex intgeom.Point, ringId int, level uint) {
	levelHitOnce := ix.hitOnce[level]
	levelHitMultiple := ix.hitMultiple[level]
	if len(levelHitOnce[vertex]) > 0 {
		if !slices.Contains(levelHitOnce[vertex], ringId) {
			// point has been hit before, but not by this ring
			levelHitOnce[vertex] = append(levelHitOnce[vertex], ringId)
		} else if !slices.Contains(levelHitMultiple[vertex], ringId) {
			// point has been hit before by this ring, add to hitMultiple (if not already present)
			levelHitMultiple[vertex] = append(levelHitMultiple[vertex], ringId)
		}
	} else {
		// first hit of this point by any ring
		levelHitOnce[vertex] = append(levelHitOnce[vertex], ringId)
	}
}

func isExclusiveEdge(edgeI int) bool {
	i := edgeI % 4
	return i == 1 || i == 2
}

// getExclusiveTip returns the tip point of an inclusive edge that is not-inclusive
func getExclusiveTip(edgeI int, edge intgeom.Line) intgeom.Point {
	i := edgeI % 4
	if i == 0 {
		return edge[1]
	} else if i == 3 {
		return edge[0]
	}
	panic(fmt.Sprintf("not an inclusive edge: %v", edgeI))
}

// lineOverlapsInclusiveEdge helps to check if a line overlaps an inclusive edge (excluding the exclusive tip)
func lineOverlapsInclusiveEdge(intLine intgeom.Line, edgeI int, intEdge intgeom.Line) bool {
	var constAx, varAx int
	switch {
	case intEdge[0][xAx] == intEdge[1][xAx]:
		constAx = xAx
		varAx = yAx
	case intEdge[0][yAx] == intEdge[1][yAx]:
		constAx = yAx
		varAx = xAx
	default:
		panic(fmt.Sprintf("not a straight edge: %v", intEdge))
	}
	eConstOrd := intEdge[0][constAx]
	if intLine[0][constAx] != eConstOrd || intLine[1][constAx] != eConstOrd {
		return false // not a straight line and/or not on same line as the edge, so no overlap
	}
	eOrd1 := intEdge[0][varAx]
	eOrd2 := intEdge[1][varAx]

	exclusiveTip := getExclusiveTip(edgeI, intEdge)
	// if exclusiveTip[constAx] != eConstOrd || !betweenInc(exclusiveTip[varAx], eOrd1, eOrd2) {
	// 	 panic(fmt.Sprintf("exclusive point not on edge: %v, %v", exclusiveTip, edge))
	// }
	lOrd1 := intLine[0][varAx]
	lOrd2 := intLine[1][varAx]
	return lOrd1 != lOrd2 && (betweenInc(lOrd1, eOrd1, eOrd2) && intLine[0] != exclusiveTip || betweenInc(lOrd2, eOrd1, eOrd2) && intLine[1] != exclusiveTip)
}

func betweenInc(f, p, q int64) bool {
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

func oneIfRight(quadrantI int) int {
	return quadrantI & right
}

func oneIfTop(quadrantI int) int {
	return (quadrantI & top) >> 1
}

// toWkt creates a WKT representation of the pointcloud. For debugging/visualising.
func (ix *PointIndex) toWkt(writer io.Writer) {
	for level, quadrants := range ix.quadrants {
		for _, quadrant := range quadrants {
			_ = wkt.Encode(writer, quadrant.intExtent.ToGeomExtent())
			_, _ = fmt.Fprintf(writer, "\n")
			if level == ix.maxDepth {
				_ = wkt.Encode(writer, quadrant.intCentroid.ToGeomPoint())
				_, _ = fmt.Fprintf(writer, "\n")
			}
		}
	}
}
