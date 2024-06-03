package pointindex

import (
	"errors"
	"fmt"
	"golang.org/x/exp/maps"
	"io"
	"math"
	"slices"
	"strconv"

	"github.com/pdok/texel/mathhelp"
	"github.com/pdok/texel/morton"
	"github.com/pdok/texel/tms20"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/wkt"
	"github.com/pdok/texel/intgeom"
)

const (
	VectorTileInternalPixelResolution = 16
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

type Quadrant struct {
	z           morton.Z
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
	deepestLevel Level
	// Number of quadrants (in one direction) on the deepest level (= 2 ^ deepestLevel)
	deepestSize uint
	deepestRes  intgeom.M
	quadrants   map[Level]map[morton.Z]Quadrant
	hitOnce     map[Level]map[intgeom.Point][]int
	hitMultiple map[Level]map[intgeom.Point][]int
}

type Level = uint
type Q = int // quadrant index (0, 1, 2 or 3)

func FromTileMatrixSet(tileMatrixSet tms20.TileMatrixSet, deepestTMID tms20.TMID) *PointIndex {
	if err := isQuadTree(tileMatrixSet); err != nil {
		panic(err)
	}
	rootTM := tileMatrixSet.TileMatrices[0]
	levelDiff := uint(math.Log2(float64(rootTM.TileWidth))) + uint(math.Log2(float64(VectorTileInternalPixelResolution)))
	deepestLevel := uint(deepestTMID) + levelDiff
	bottomLeft, topRight, err := tileMatrixSet.MatrixBoundingBox(0)
	if err != nil {
		panic(fmt.Errorf(`could not make PointIndex from TileMatrixSet %v: %w'`, tileMatrixSet.ID, err))
	}
	intBottomLeft := intgeom.FromGeomPoint(bottomLeft)
	intTopRight := intgeom.FromGeomPoint(topRight)
	intExtent := intgeom.Extent{intBottomLeft.X(), intBottomLeft.Y(), intTopRight.X(), intTopRight.Y()}
	deepestSize := mathhelp.Pow2(deepestLevel)
	ix := PointIndex{
		Quadrant: Quadrant{
			intExtent: intExtent,
			z:         0,
		},
		deepestLevel: deepestLevel,
		deepestSize:  deepestSize,
		deepestRes:   intExtent.XSpan() / int64(deepestSize),
		quadrants:    make(map[Level]map[morton.Z]Quadrant, deepestLevel+1),
		hitOnce:      make(map[uint]map[intgeom.Point][]int),
		hitMultiple:  make(map[uint]map[intgeom.Point][]int),
	}
	_, ix.intCentroid = ix.getQuadrantExtentAndCentroid(0, 0, 0, intExtent)

	return &ix
}

// InsertPolygon inserts all points from a Polygon
func (ix *PointIndex) InsertPolygon(polygon geom.Polygon) {
	// initialize the quadrants map
	pointsCount := 0
	for _, ring := range polygon.LinearRings() {
		pointsCount += len(ring)
	}
	var level uint
	for level = 0; level <= ix.deepestLevel; level++ {
		if ix.quadrants[level] == nil {
			ix.quadrants[level] = make(map[morton.Z]Quadrant, pointsCount) // TODO smaller for the shallower levels
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
	intPoint := intgeom.FromGeomPoint(point)
	deepestX := int((intPoint.X() - ix.intExtent.MinX()) / ix.deepestRes)
	deepestY := int((intPoint.Y() - ix.intExtent.MinY()) / ix.deepestRes)
	ix.InsertCoord(deepestX, deepestY)
}

// InsertCoord inserts a Point by its x/y coord on the deepest level
func (ix *PointIndex) InsertCoord(deepestX int, deepestY int) {
	if deepestX < 0 || deepestY < 0 || deepestX > int(ix.deepestSize)-1 || deepestY > int(ix.deepestSize)-1 {
		// should never happen
		panic(fmt.Errorf("trying to insert a coord (%v, %v) outside the grid/extent (0, %v; 0, %v)", deepestX, deepestY, ix.deepestSize, ix.deepestSize))
	}
	ix.insertCoord(deepestX, deepestY)
}

// insertCoord adds a point into this pc, assuming the point is inside its extent
func (ix *PointIndex) insertCoord(deepestX int, deepestY int) {
	var l Level
	for l = 0; l <= ix.deepestLevel; l++ {
		x := uint(deepestX) / mathhelp.Pow2(ix.deepestLevel-l)
		y := uint(deepestY) / mathhelp.Pow2(ix.deepestLevel-l)
		z := morton.MustToZ(x, y)
		if ix.quadrants[l] == nil { // probably already initialized by InsertPolygon
			ix.quadrants[l] = make(map[morton.Z]Quadrant)
		}
		extent, centroid := ix.getQuadrantExtentAndCentroid(l, x, y, ix.intExtent)
		ix.quadrants[l][z] = Quadrant{
			z:           z,
			intExtent:   extent,
			intCentroid: centroid,
		}
	}
}

func (ix *PointIndex) getQuadrantExtentAndCentroid(level Level, x, y uint, intRootExtent intgeom.Extent) (intgeom.Extent, intgeom.Point) {
	intQuadrantSpan := int64(mathhelp.Pow2(ix.deepestLevel-level)) * ix.deepestRes
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
	if len(levelMap) == 0 || !lineIntersects(intLine, ix.intExtent) {
		return nil
	}
	quadrantsIntersectedPerLevel := make(map[Level][]Quadrant, len(levelMap))
	parents := []Quadrant{ix.Quadrant}
	if _, includeLevelZero := levelMap[0]; includeLevelZero {
		quadrantsIntersectedPerLevel[0] = parents
	}

	var level Level
	for level = 1; level <= ix.deepestLevel; level++ {
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

func getQuadrantZs(parentZ morton.Z) [4]morton.Z {
	parentX, parentY := morton.FromZ(parentZ)
	quadrantZs := [4]morton.Z{}
	for i := 0; i < 4; i++ {
		x := parentX*2 + uint(oneIfRight(i))
		y := parentY*2 + uint(oneIfTop(i))
		z := morton.MustToZ(x, y)
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
	isRight := mathhelp.Bool2int(intPt[0] >= intCentroid[0])
	isTop := mathhelp.Bool2int(intPt[1] >= intCentroid[1]) << 1
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

func (ix *PointIndex) GetHitMultiple(l Level) map[intgeom.Point][]int {
	return ix.hitMultiple[l]
}

func checkPointHits(ix *PointIndex, vertex intgeom.Point, ringID int, level uint) {
	levelHitOnce := ix.hitOnce[level]
	levelHitMultiple := ix.hitMultiple[level]
	if len(levelHitOnce[vertex]) > 0 {
		if !slices.Contains(levelHitOnce[vertex], ringID) {
			// point has been hit before, but not by this ring
			levelHitOnce[vertex] = append(levelHitOnce[vertex], ringID)
		} else if !slices.Contains(levelHitMultiple[vertex], ringID) {
			// point has been hit before by this ring, add to hitMultiple (if not already present)
			levelHitMultiple[vertex] = append(levelHitMultiple[vertex], ringID)
		}
	} else {
		// first hit of this point by any ring
		levelHitOnce[vertex] = append(levelHitOnce[vertex], ringID)
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
	lOrd1 := intLine[0][varAx]
	lOrd2 := intLine[1][varAx]
	return lOrd1 != lOrd2 && (mathhelp.BetweenInc(lOrd1, eOrd1, eOrd2) && intLine[0] != exclusiveTip || mathhelp.BetweenInc(lOrd2, eOrd1, eOrd2) && intLine[1] != exclusiveTip)
}

func oneIfRight(quadrantI int) int {
	return quadrantI & right
}

func oneIfTop(quadrantI int) int {
	return (quadrantI & top) >> 1
}

// ToWkt creates a WKT representation of the pointcloud. For debugging/visualising.
func (ix *PointIndex) ToWkt(writer io.Writer) {
	for level, quadrants := range ix.quadrants {
		for _, quadrant := range quadrants {
			_ = wkt.Encode(writer, quadrant.intExtent.ToGeomExtent())
			_, _ = fmt.Fprintf(writer, "\n")
			if level == ix.deepestLevel {
				_ = wkt.Encode(writer, quadrant.intCentroid.ToGeomPoint())
				_, _ = fmt.Fprintf(writer, "\n")
			}
		}
	}
}

func isQuadTree(tms tms20.TileMatrixSet) error {
	var previousTMID int
	var previousTM *tms20.TileMatrix
	tmIDs := maps.Keys(tms.TileMatrices)
	slices.Sort(tmIDs)
	for _, tmID := range tmIDs {
		tm := tms.TileMatrices[tmID]
		if tm.MatrixHeight != tm.MatrixWidth {
			return errors.New("tile matrix height should be same as width: " + tm.ID)
		}
		if tm.TileHeight != tm.TileWidth {
			return errors.New("tiles should be square: " + tm.ID)
		}
		tmIDStringToInt, err := strconv.Atoi(tm.ID)
		if err != nil {
			return err
		}
		if tmIDStringToInt != tmID {
			return errors.New("tile matrix ID should string representation of its index in the array: " + tm.ID)
		}
		if len(tm.VariableMatrixWidths) != 0 {
			return errors.New("variable matrix widths are not supported: " + tm.ID)
		}
		if previousTM != nil {
			if tmID != previousTMID+1 {
				return errors.New("tile matrix IDs should be a range with step 1 starting with 0")
			}
			if *tm.PointOfOrigin != *previousTM.PointOfOrigin {
				return errors.New("tile matrixes should have the same point of origin: " + tm.ID)
			}
			if tm.CornerOfOrigin != previousTM.CornerOfOrigin {
				return errors.New("tile matrixes should have the same corner of origin: " + tm.ID)
			}
		}

		previousTMID = tmID
		previousTM = &tm
	}
	return nil
}

// DeviationStats calcs some stats to show the result of using ints internally
// if the res (span/size) is not "round" (with intgeom.Precision), a deviation will arise
//
//nolint:nakedret
func DeviationStats(tms tms20.TileMatrixSet, deepestTMID tms20.TMID) (stats string, deviationInUnits, deviationInPixels float64, err error) {
	bottomLeft, topRight, err := tms.MatrixBoundingBox(0)
	if err != nil {
		return
	}
	ix := FromTileMatrixSet(tms, deepestTMID)
	p := uint(intgeom.Precision + 1)
	ps := strconv.Itoa(int(p))
	stats += fmt.Sprintf("deepest level: %d\n", ix.deepestLevel)
	stats += fmt.Sprintf("deepest pixels count on one axis: %d\n", ix.deepestSize)
	stats += fmt.Sprintf("float minx, miny: %."+ps+"f, %."+ps+"f\n", bottomLeft.X(), bottomLeft.Y())
	stats += fmt.Sprintf("float maxx, maxy: %."+ps+"f, %."+ps+"f\n", topRight.X(), topRight.Y())
	floatSpanX := topRight.X() - bottomLeft.X()
	stats += fmt.Sprintf("float span X: %."+ps+"f\n", floatSpanX)
	stats += fmt.Sprintf("int64 span X: %s\n", intgeom.PrintWithDecimals(ix.intExtent.XSpan(), p))
	floatRes := floatSpanX / float64(ix.deepestSize)
	stats += fmt.Sprintf("float reso: %."+ps+"f\n", floatRes)
	intRes := ix.intExtent.XSpan() / int64(ix.deepestSize)
	stats += fmt.Sprintf("int64 reso: %s\n", intgeom.PrintWithDecimals(intRes, p))

	floatRecalcMaxX := floatRes * float64(ix.deepestSize)
	stats += fmt.Sprintf("float recalc maxX: %."+ps+"f\n", floatRecalcMaxX)
	intRecalcMaxX := intgeom.ToGeomOrd(intRes * int64(ix.deepestSize))
	stats += fmt.Sprintf("int64 recalc maxX: %."+ps+"f\n", intRecalcMaxX)

	deviationInUnits = floatRecalcMaxX - intRecalcMaxX
	deviationInPixels = deviationInUnits / floatRes
	stats += fmt.Sprintf("deviation (in units) maxX : %."+ps+"f\n", deviationInUnits)
	stats += fmt.Sprintf("deviation (in pixels) maxX: %."+ps+"f\n", deviationInPixels)

	return
}
