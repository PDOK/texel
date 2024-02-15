package snap

import (
	"fmt"
	"github.com/tobshub/go-sortedmap"
	"log"
	"math"
	"slices"
	"sort"

	"github.com/go-spatial/geom/winding"

	"github.com/go-spatial/geom/encoding/wkt"
	"github.com/muesli/reflow/truncate"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/intgeom"
	"github.com/pdok/texel/tms20"
	orderedmap "github.com/wk8/go-ordered-map/v2"
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/maps"
)

const (
	keepPointsAndLines      = true // TODO do something with polys that collapsed into points and lines
	internalPixelResolution = 16
)

// SnapPolygon snaps polygons' points to a tile's internal pixel grid
// and adds points to lines to prevent intersections.
//
//nolint:revive
func SnapPolygon(polygon geom.Polygon, tileMatrixSet tms20.TileMatrixSet, tmIDs []tms20.TMID) map[tms20.TMID][]geom.Polygon {
	deepestID := slices.Max(tmIDs)
	ix := newPointIndexFromTileMatrixSet(tileMatrixSet, deepestID)
	tmIDsByLevels := tileMatrixIDsByLevels(tileMatrixSet, tmIDs)
	levels := make([]Level, 0, len(tmIDsByLevels))
	for level := range tmIDsByLevels {
		levels = append(levels, level)
	}

	ix.InsertPolygon(polygon)
	newPolygonsPerLevel := addPointsAndSnap(ix, polygon, levels)

	newPolygonsPerTileMatrixID := make(map[tms20.TMID][]geom.Polygon, len(newPolygonsPerLevel))
	for level, newPolygons := range newPolygonsPerLevel {
		newPolygonsPerTileMatrixID[tmIDsByLevels[level]] = newPolygons
	}

	return newPolygonsPerTileMatrixID
}

func newPointIndexFromTileMatrixSet(tileMatrixSet tms20.TileMatrixSet, deepestTMID tms20.TMID) *PointIndex {
	// TODO ensure that the tile matrix set is actually a quad tree, starting at 0. just assuming now.
	rootTM := tileMatrixSet.TileMatrices[0]
	levelDiff := uint(math.Log2(float64(rootTM.TileWidth))) + uint(math.Log2(float64(internalPixelResolution)))
	maxDepth := uint(deepestTMID) + levelDiff
	bottomLeft, topRight, err := tileMatrixSet.MatrixBoundingBox(0)
	if err != nil {
		panic(fmt.Errorf(`could not make PointIndex from TileMatrixSet %v: %w'`, tileMatrixSet.ID, err))
	}
	intBottomLeft := intgeom.FromGeomPoint(bottomLeft)
	intTopRight := intgeom.FromGeomPoint(topRight)
	intExtent := intgeom.Extent{intBottomLeft.X(), intBottomLeft.Y(), intTopRight.X(), intTopRight.Y()}
	ix := PointIndex{
		Quadrant: Quadrant{
			intExtent: intExtent,
			z:         0,
		},
		maxDepth:    maxDepth,
		quadrants:   make(map[Level]map[Z]Quadrant, maxDepth+1),
		hitOnce:     make(map[uint]map[intgeom.Point][]int),
		hitMultiple: make(map[uint]map[intgeom.Point][]int),
	}
	_, ix.intCentroid = getQuadrantExtentAndCentroid(0, 0, 0, intExtent)

	return &ix
}

func tileMatrixIDsByLevels(tms tms20.TileMatrixSet, tmIDs []tms20.TMID) map[Level]tms20.TMID {
	rootTM := tms.TileMatrices[0]
	levelDiff := uint(math.Log2(float64(rootTM.TileWidth))) + uint(math.Log2(float64(internalPixelResolution)))
	tmIDsByLevels := make(map[Level]tms20.TMID, len(tmIDs))
	for _, tmID := range tmIDs {
		// assuming 2^(tmID) = tm.MatrixWidth = tm.MatrixHeight
		level := uint(tmID) + levelDiff
		tmIDsByLevels[level] = tmID
	}
	return tmIDsByLevels
}

//nolint:cyclop
func addPointsAndSnap(ix *PointIndex, polygon geom.Polygon, levels []Level) map[Level][]geom.Polygon {
	levelMap := asKeys(levels)
	newOuters := make(map[Level][][][2]float64, len(levels))
	newInners := make(map[Level][][][2]float64, len(levels))
	newPointsAndLines := make(map[Level][][][2]float64, len(levels))

	// Could use polygon.AsSegments(), but it skips rings with <3 segments and starts with the last segment.
	for ringIdx, ring := range polygon.LinearRings() {
		if len(levelMap) == 0 { // level could have been obsoleted
			continue
		}
		isOuter := ringIdx == 0
		// winding order is reversed if incorrect
		ring = ensureCorrectWindingOrder(ring, !isOuter)
		ringLen := len(ring)
		newRing := make(map[Level][][2]float64, len(levelMap))
		for level := range levelMap {
			newRing[level] = make([][2]float64, 0, 2*ringLen) // TODO better estimation of new amount of points for a ring
		}

		// walk through the vertices and append to the new ring (on all levels)
		for vertexIdx, vertex := range ring {
			// LinearRings(): "The last point in the linear ring will not match the first point."
			// So also including that one.
			nextVertexIdx := (vertexIdx + 1) % ringLen
			segment := geom.Line{vertex, ring[nextVertexIdx]}
			newVertices := ix.SnapClosestPoints(segment, levelMap, ringIdx)
			for level := range levelMap {
				cleanedNewVertices := cleanupNewVertices(newVertices[level], segment, level, lastElement(newRing[level]))
				newRing[level] = append(newRing[level], cleanedNewVertices...)
			}
		}

		// walk through the new ring and append to the polygon (on all levels)
		for level := range levelMap {
			outerRings, innerRings, pointsAndLines := cleanupNewRing(newRing[level], isOuter, ix.hitMultiple[level], ringIdx)
			// Check if outer ring has become too small
			if isOuter && len(outerRings) == 0 && (!keepPointsAndLines || len(pointsAndLines) == 0) {
				delete(levelMap, level) // If too small, delete it
				continue
			}
			for _, outerRing := range outerRings {
				newOuters[level] = append(newOuters[level], outerRing)
			}
			newInners[level] = append(newInners[level], innerRings...)
			if keepPointsAndLines {
				newPointsAndLines[level] = append(newPointsAndLines[level], pointsAndLines...)
			}
		}
	}

	newPolygons := make(map[Level][][][][2]float64, len(levels))
	for l := range levelMap {
		newOuters[l], newInners[l] = dedupeAndSortBySizeInnersOuters(newOuters[l], newInners[l])
		newPolygonsForLevel := matchInnersToPolygons(newPolygonsForLevel, newInners[l], len(polygon) > 1)
		if len(newPolygonsForLevel) > 1 {
			newPolygons[l] = newPolygonsForLevel
		}
	}

	// points and lines at the end, as outer rings
	for level, pointsAndLines := range newPointsAndLines {
		for _, pointOrLine := range pointsAndLines {
			newOuters[level] = append(newOuters[level], [][][2]float64{pointOrLine})
		}
	}
	return floatPolygonsToGeomPolygonsForAllLevels(newOuters)
}

func dedupeAndSortBySizeInnersOuters(outers [][][2]float64, inners [][][2]float64) ([][][2]float64, [][][2]float64) {
	// ToDo: optimize by deleting rings from allRings on the fly
	allRings := append(outers, inners...)
	lenOuters := len(outers)
	var indexesToDelete []int
	for i := 0; i < len(allRings); i++ {
		iIsOuter := i < lenOuters
		var equalOutersIndexes []int
		var equalInnersIndexes []int
		if iIsOuter {
			equalOutersIndexes = append(equalOutersIndexes, i)
		} else {
			equalInnersIndexes = append(equalInnersIndexes, i)
		}
	compareTwoRings:
		for j := i + 1; j < len(allRings); j++ {
			jIsOuter := j < lenOuters
			// check length
			iLen := len(allRings[i])
			jLen := len(allRings[j])
			if iLen != jLen {
				continue
			}
			idx := slices.Index(allRings[j], allRings[i][0])
			if idx < 0 {
				continue
			}
			differentWindingOrder := iIsOuter && !jIsOuter
			// Check if rings are equal
			for k := 0; k < iLen; k++ {
				if !differentWindingOrder && allRings[i][k] != allRings[j][idx+k%iLen] {
					continue compareTwoRings
				}
				if differentWindingOrder && allRings[i][k] != allRings[j][idx-k%iLen] {
					continue compareTwoRings
				}
			}
			// they are the same!
			if jIsOuter {
				equalOutersIndexes = append(equalOutersIndexes, j)
			} else {
				equalInnersIndexes = append(equalInnersIndexes, j)
			}
		}
		// hier toevoegen aan indexesToDelete. mits meer dan 1 herhaling
		difference := int(math.Abs(float64(len(equalOutersIndexes)) - float64(len(equalInnersIndexes))))
		if difference == 0 {
			indexesToDelete = append(indexesToDelete, equalOutersIndexes[1:]...)
			indexesToDelete = append(indexesToDelete, equalInnersIndexes[1:]...)
		}
		if difference > 0 {
			numToDelete := min(len(equalOutersIndexes), len(equalInnersIndexes))
			indexesToDelete = append(indexesToDelete, equalOutersIndexes[0:numToDelete-1]...)
			indexesToDelete = append(indexesToDelete, equalInnersIndexes[0:numToDelete-1]...)
		}
	}
	newOuters := make([][][2]float64, 0, lenOuters)
	newInners := make([][][2]float64, 0, len(inners))
	for i, outer := range outers {
		if slices.Contains(indexesToDelete, i) {
			continue
		}
		newOuters = append(newOuters, outer)
	}
	for i, inner := range inners {
		if slices.Contains(indexesToDelete, i+lenOuters) {
			continue
		}
		newInners = append(newInners, inner)
	}
	return newOuters, newInners
}

func matchInnersToPolygons(polygons [][][][2]float64, innerRings [][][2]float64, hasInners bool) [][][][2]float64 {
	lenPolygons := len(polygons)
	if len(innerRings) == 0 {
		return polygons
	}

	var polyISortedByOuterAreaDesc []int
	var innersTurnedOuters [][][2]float64
matchInners:
	for _, innerRing := range innerRings {
		containsPerPolyI := orderedmap.New[int, uint](orderedmap.WithCapacity[int, uint](lenPolygons)) // TODO don't need ordered map anymore?
		// this is pretty nested, but usually breaks early
		for _, vertex := range innerRing {
			for polyI := range polygons {
				contains, _ := ringContains(polygons[polyI][0], vertex)
				// it doesn't matter if on boundary or not, if not on boundary there could still be multiple (nested) matching polygons
				if contains {
					containsPerPolyI.Set(polyI, containsPerPolyI.Value(polyI)+1)
				}
			}
			matchingPolyI, _, matchCount := findLastKeyWithMaxValue(containsPerPolyI)
			if matchCount == 1 {
				polygons[matchingPolyI] = append(polygons[matchingPolyI], innerRing)
				continue matchInners
			}
		}
		if containsPerPolyI.Len() == 0 {
			// no (single) matching outer ring was found
			// presumably because the inner ring's winding order is incorrect and it should have been an outer
			// TODO is that presumption correct and is this really never a panic? // panicNoMatchingOuterForInnerRing(polygons, innerRing)
			// TODO should it be a candidate for other the other inner rings?
			log.Printf("no matching outer for inner ring found, turned inner into outer. original has inners: %v", hasInners)
			innersTurnedOuters = append(innersTurnedOuters, reverseClone(innerRing))
			continue
		}
		// multiple matching outer rings were found. use the smallest one
		// TODO dedupe poly outers (not here)
		if polyISortedByOuterAreaDesc == nil {
			polyISortedByOuterAreaDesc = sortPolyIdxsByOuterAreaDesc(polygons)
		}
		smallestMatchingPolyI := lastMatch(polyISortedByOuterAreaDesc, orderedMapKeys(containsPerPolyI))
		polygons[smallestMatchingPolyI] = append(polygons[smallestMatchingPolyI], innerRing)
	}
	for i := range innersTurnedOuters {
		polygons = append(polygons, [][][2]float64{innersTurnedOuters[i]})
	}
	return polygons
}

func sortPolyIdxsByOuterAreaDesc(polygons [][][][2]float64) []int {
	areas := sortedmap.New[int, float64](len(polygons), func(i, j float64) bool {
		return i > j // desc
	})
	for i := range polygons {
		if len(polygons[i]) == 0 {
			areas.Insert(i, 0.0)
		} else {
			areas.Insert(i, shoelace(polygons[i][0]))
		}
	}
	return areas.Keys()
}

// helper for matchInnersToPolygons to delete duplicate polygons
// comparing them only by their outers and asserting that a deleted polygon didn't have inner rings appended yet
// yes it's implemented as ~O(n^2),
// but it's expected that the (outer) rings are usually different even from the first point,
// making it still more efficient than using a hashmap of the entire rings
func dedupePolygonsByOuters(polygons [][][][2]float64) (int, [][][][2]float64) {
	numPolygons := len(polygons)
	numDeleted := 0
	for i := 0; i < numPolygons; i++ {
		ring := polygons[i][0] // only check the outer
	compareToOther:
		for j := i + 1; j < numPolygons; j++ {
			ringLen := len(ring)
			other := polygons[j][0]
			otherLen := len(other)
			if ringLen != otherLen {
				continue
			}
			for k := 0; k < min(ringLen, otherLen); k++ {
				if ring[k] != other[k] {
					continue compareToOther
				}
			}
			// delete
			if len(polygons[j]) > 1 {
				panicDeletedPolygonHasInnerRing(polygons[j])
			}
			polygons = append(polygons[:j], polygons[j+1:]...)
			j--
			numPolygons--
			numDeleted++
		}
	}
	return numDeleted, polygons
}

// from paulmach/orb, modified to also return whether it's on the boundary
// ringContains returns true if the point is inside the ring.
// Points on the boundary are also considered in. In which case the second returned var is true too.
func ringContains(ring [][2]float64, point [2]float64) (contains, onBoundary bool) {
	// TODO check first if the point is in the extent/bound/envelop

	c, on := rayIntersect(point, ring[0], ring[len(ring)-1])
	if on {
		return true, true
	}

	for i := 0; i < len(ring)-1; i++ {
		intersects, on := rayIntersect(point, ring[i], ring[i+1])
		if on {
			return true, true
		}

		if intersects {
			c = !c // https://en.wikipedia.org/wiki/Even-odd_rule
		}
	}

	return c, false
}

// from paulmach/orb
// Original implementation: http://rosettacode.org/wiki/Ray-casting_algorithm#Go
//
//nolint:cyclop,nestif
func rayIntersect(pt, start, end [2]float64) (intersects, on bool) {
	if start[xAx] > end[xAx] {
		start, end = end, start
	}

	if pt[xAx] == start[xAx] {
		if pt[yAx] == start[yAx] {
			// pt == start
			return false, true
		} else if start[xAx] == end[xAx] {
			// vertical segment (start -> end)
			// return true if within the line, check to see if start or end is greater.
			if start[yAx] > end[yAx] && start[yAx] >= pt[yAx] && pt[yAx] >= end[yAx] {
				return false, true
			}

			if end[yAx] > start[yAx] && end[yAx] >= pt[yAx] && pt[yAx] >= start[yAx] {
				return false, true
			}
		}

		// Move the y coordinate to deal with degenerate case
		pt[xAx] = math.Nextafter(pt[xAx], math.Inf(1))
	} else if pt[xAx] == end[xAx] {
		if pt[yAx] == end[yAx] {
			// matching the end point
			return false, true
		}

		pt[xAx] = math.Nextafter(pt[xAx], math.Inf(1))
	}

	if pt[xAx] < start[xAx] || pt[xAx] > end[xAx] {
		return false, false
	}

	if start[yAx] > end[yAx] {
		if pt[yAx] > start[yAx] {
			return false, false
		} else if pt[yAx] < end[yAx] {
			return true, false
		}
	} else {
		if pt[yAx] > end[yAx] {
			return false, false
		} else if pt[yAx] < start[yAx] {
			return true, false
		}
	}

	rs := (pt[yAx] - start[yAx]) / (pt[xAx] - start[xAx])
	ds := (end[yAx] - start[yAx]) / (end[xAx] - start[xAx])

	if rs == ds {
		return false, true
	}

	return rs <= ds, false
}

// cleanupNewVertices cleans up the closest points for a line that were just retrieved inside addPointsAndSnap
func cleanupNewVertices(newVertices [][2]float64, segment [2][2]float64, level Level, lastVertex *[2]float64) [][2]float64 {
	newVerticesCount := len(newVertices)
	if newVerticesCount == 0 { // should never happen, SnapClosestPoints should have returned at least one point
		panicNoPointsFoundForVertices(segment, level)
	}
	// 0 if len is 1, 1 otherwise
	minus := min(newVerticesCount-1, 1)
	// remove last vertex if there is more than 1 vertex, as the first vertex in the next segment will be the same
	newVertices = newVertices[:newVerticesCount-minus]
	// remove first element if it is equal to the last element added to newRing
	if lastVertex != nil && newVertices[0] == *lastVertex {
		newVertices = newVertices[1:]
	}
	return newVertices
}

// cleanupNewRing cleans up a ring (if not too small) that was just crafted inside addPointsAndSnap
func cleanupNewRing(newRing [][2]float64, isOuter bool, hitMultiple map[intgeom.Point][]int, ringIdx int) (outerRings, innerRings, pointsAndLines [][][2]float64) {
	newRingLen := len(newRing)
	// LinearRings(): "The last point in the linear ring will not match the first point."
	if newRingLen > 1 && newRing[0] == newRing[newRingLen-1] {
		newRing = newRing[:newRingLen-1]
		newRingLen--
	}
	// filter out too small rings
	if newRingLen < 3 {
		return nil, nil, [][][2]float64{newRing}
	}
	// deduplicate points in the ring
	newRing = kmpDeduplicate(newRing)
	newRingLen = len(newRing)
	// again filter out too small rings, after deduping
	if newRingLen < 3 {
		return nil, nil, [][][2]float64{newRing}
	}
	// split ring and return results
	return splitRing(newRing, isOuter, hitMultiple, ringIdx)
}

// if winding order is incorrect, ring is reversed to correct winding order
func ensureCorrectWindingOrder(ring [][2]float64, shouldBeClockwise bool) [][2]float64 {
	if !windingOrderIsCorrect(ring, shouldBeClockwise) {
		return reverseClone(ring)
	}
	return ring
}

// validate winding order (CCW for outer rings, CW for inner rings)
func windingOrderIsCorrect(ring [][2]float64, shouldBeClockwise bool) bool {
	wo := winding.Order{}.OfPoints(ring...)
	return wo.IsClockwise() && shouldBeClockwise || wo.IsCounterClockwise() && !shouldBeClockwise || wo.IsColinear()
}

// TODO: rewrite by using intgeoms for as long as possible
func isHitMultiple(hitMultiple map[intgeom.Point][]int, vertex [2]float64, ringIdx int) bool {
	intVertex := intgeom.FromGeomPoint(vertex)
	return slices.Contains(hitMultiple[intVertex], ringIdx) || // exact match
		slices.Contains(hitMultiple[intgeom.Point{intVertex[xAx] + 1, intVertex[yAx]}], ringIdx) || // fuzzy search
		slices.Contains(hitMultiple[intgeom.Point{intVertex[xAx] - 1, intVertex[yAx]}], ringIdx) ||
		slices.Contains(hitMultiple[intgeom.Point{intVertex[xAx], intVertex[yAx] + 1}], ringIdx) ||
		slices.Contains(hitMultiple[intgeom.Point{intVertex[xAx], intVertex[yAx] - 1}], ringIdx)
}

// split ring into multiple rings at any point where the ring goes through the point more than once
//
//nolint:cyclop,gocritic
func splitRing(ring [][2]float64, isOuter bool, hitMultiple map[intgeom.Point][]int, ringIdx int) (outerRings, innerRings, pointsAndLines [][][2]float64) {
	partialRingIdx := 0
	stack := orderedmap.New[int, [][2]float64]()
	stack.Set(partialRingIdx, [][2]float64{})
	completeRings := make(map[int][][2]float64)
	checkRing := append(ring, ring[0])
	for vertexIdx, vertex := range checkRing {
		if vertexIdx == 0 || !isHitMultiple(hitMultiple, vertex, ringIdx) {
			if partialRing, inited := stack.Get(partialRingIdx); !inited {
				stack.Set(partialRingIdx, make([][2]float64, 0, len(checkRing)))
			} else {
				stack.Set(partialRingIdx, append(partialRing, vertex))
			}
			if vertexIdx < len(checkRing)-1 {
				continue
			}
		} else {
			stack.Set(partialRingIdx, append(stack.Value(partialRingIdx), vertex))
		}
		tempRing := stack.Value(partialRingIdx)
		if tempRing[0] == tempRing[len(tempRing)-1] {
			// tempRing is already a complete ring
			completeRings[partialRingIdx] = tempRing[:len(tempRing)-1]
			stack.Delete(partialRingIdx)
		} else {
			// keep prepending partial rings from the stack until tempRing is a complete ring, or the end of the stack is reached
			partialsToRemove := []int{partialRingIdx}
			for r := stack.Newest().Prev(); r != nil; r = r.Prev() {
				stackIdx := r.Key
				partialRingFromStack := r.Value
				// add previous partial ring if it connects to the start of the temp ring
				if partialRingFromStack[len(partialRingFromStack)-1] == tempRing[0] {
					partialsToRemove = append(partialsToRemove, stackIdx)
					tempRing = append(partialRingFromStack, tempRing[1:]...)
				} else {
					break
				}
				// closed ring, clean up partials
				if tempRing[0] == tempRing[len(tempRing)-1] {
					completeRings[stackIdx] = tempRing[:len(tempRing)-1]
					for _, idx := range partialsToRemove {
						stack.Delete(idx)
					}
					break
				}
			}
		}
		if vertexIdx < len(checkRing)-1 {
			partialRingIdx++
			stack.Set(partialRingIdx, append(stack.Value(partialRingIdx), vertex))
		} else if stack.Len() > 0 {
			// if partial rings remain on stack when end of ring is reached, something has gone wrong
			panicPartialRingsRemainingOnStack(stack)
		}
	}
	completeRingKeys := maps.Keys(completeRings)
	sort.Ints(completeRingKeys)
	for _, completeRingKey := range completeRingKeys {
		completeRing := completeRings[completeRingKey]
		// rings with 0 area (defined as having fewer than 3 points) are separated
		switch {
		case len(completeRing) < 3:
			pointsAndLines = append(pointsAndLines, completeRing)
		case isOuter:
			// check winding order, add to inner rings if incorrect
			if !windingOrderIsCorrect(completeRing, false) {
				innerRings = append(innerRings, completeRing)
			} else {
				outerRings = append(outerRings, completeRing)
			}
		default:
			// check winding order, add to outer rings if incorrect
			if !windingOrderIsCorrect(completeRing, true) {
				outerRings = append(outerRings, completeRing)
			} else {
				innerRings = append(innerRings, completeRing)
			}
		}
	}
	// outer ring(s) incorrectly saved as inner ring(s) or vice versa due to winding order, swap
	if isOuter && len(outerRings) == 0 && len(innerRings) > 0 {
		for _, innerRing := range innerRings {
			slices.Reverse(innerRing) // in place, not used elsewhere
			outerRings = append(outerRings, innerRing)
		}
		innerRings = make([][][2]float64, 0)
	} else if !isOuter && len(innerRings) == 0 && len(outerRings) > 0 {
		for _, outerRing := range outerRings {
			slices.Reverse(outerRing) // in place, not used elsewhere
			innerRings = append(innerRings, outerRing)
		}
		outerRings = make([][][2]float64, 0)
	}
	return outerRings, innerRings, pointsAndLines
}

// deduplication using an implementation of the Knuth-Morris-Pratt algorithm
//
//nolint:cyclop,funlen
func kmpDeduplicate(ring [][2]float64) [][2]float64 {
	ringLen := len(ring)
	// sequences (from index uptoandincluding index) to remove, sorted by starting index, mapped to prevent dupes
	sequencesToRemove := sortedmap.New[string, [2]int](ringLen, func(a, b [2]int) bool {
		return a[xAx] < b[xAx]
	})
	// walk through ring until a step back is taken, then identify how many steps back are taken and search for repeats
	visitedPoints := [][2]float64{}
	for i := 0; i < ringLen; {
		vertex := ring[i]
		// not a step back, continue
		if len(visitedPoints) <= 1 || visitedPoints[len(visitedPoints)-2] != vertex {
			visitedPoints = append(visitedPoints, vertex)
			i++
			continue
		}
		// first step back taken, check backwards through visited points to build reverse segment
		reverseSegment := [][2]float64{visitedPoints[len(visitedPoints)-1], visitedPoints[len(visitedPoints)-2]}
		for j := 3; j <= len(visitedPoints); j++ {
			nextI := i + (j - 2)
			if nextI <= ringLen-1 && visitedPoints[len(visitedPoints)-j] == ring[nextI] {
				reverseSegment = append(reverseSegment, visitedPoints[len(visitedPoints)-j])
			} else {
				// end of segment
				break
			}
		}
		// create segment from reverse segment
		segment := make([][2]float64, len(reverseSegment))
		copy(segment, reverseSegment)
		slices.Reverse(segment) // in place, not used elsewhere
		// create search corpus: initialise with section of 3*segment length, then continuously
		// add 2*segment length until corpus contains a point that is not in segment
		start := i - len(segment)
		end := start + (3 * len(segment))
		k := 0
		corpus := ring[start:min(end, ringLen)]
		for {
			stop := false
			// check if (additional) corpus contains a point that is not in segment
			for _, vertex := range corpus[k:] {
				if !slices.Contains(segment, vertex) {
					stop = true
					break
				}
			}
			// corpus already runs until the end of ring
			if end > ringLen {
				stop = true
			}
			if stop {
				break
			}
			// expand corpus
			k = len(corpus)
			corpus = append(corpus, ring[end:min(end+(2*len(segment)), ringLen)]...)
			end += 2 * len(segment)
		}
		// search corpus for all matches of segment and reverseSegment
		matches := kmpSearchAll(corpus, segment)
		reverseMatches := kmpSearchAll(corpus, reverseSegment)
		switch {
		case len(matches) > 1 && (len(matches)-len(reverseMatches)) == 1:
			// zigzag found (segment occurs one time more often than its reverse)
			// mark all but one occurrance of segment for removal
			sequenceStart := start + len(segment)
			sequenceEnd := start + matches[len(matches)-1] + len(segment)
			sequencesToRemove.Insert(fmt.Sprint(segment), [2]int{sequenceStart, sequenceEnd})
			// skip past matched section and reset visitedPoints
			i = sequenceEnd
			visitedPoints = [][2]float64{}
		case len(matches) > 1 && len(matches) == len(reverseMatches):
			// multiple backtrace found (segment occurs more than once, and equally as many times as its reverse)
			// mark all but one occurrance of segment and one occurrance of its reverse for removal
			sequenceStart := start + (2 * len(segment)) - 1
			sequenceEnd := start + matches[len(matches)-1] + len(segment)
			sequencesToRemove.Insert(fmt.Sprint(segment), [2]int{sequenceStart, sequenceEnd})
			// skip past matched section and reset visitedPoints
			i = sequenceEnd
			visitedPoints = [][2]float64{}
		case len(matches) == 1 && len(reverseMatches) == 1:
			// backtrace found (segment and its reverse occur exactly once)
			// no removal necessary, skip past matched section and reset visitedPoints
			i = start + (2 * len(segment)) - 1
			visitedPoints = [][2]float64{}
		default:
			sequenceStart := start
			var sequenceEnd int
			var endPointIdx int
			if len(reverseMatches) > len(matches) {
				// segment occurs fewer times than its reverse -- could be an odd zigzag, or a backtrace followed by a triangle or a square
				// remove the initial backtrace, retain the remaining points (backtrace, triangle, or square)
				sequenceEnd = start + 2*(len(segment)-1)*len(matches)
				endPointIdx = start + reverseMatches[len(reverseMatches)-1] + len(segment)
			} else if len(matches) > 1 && (len(matches)-len(reverseMatches)) > 1 {
				// segment occurs more than one time more often then its reverse -- same as previous case, but with an initial zigzag instead of an initial backtrace
				sequenceEnd = start + 2*(len(segment)-1)*len(reverseMatches)
				endPointIdx = start + matches[len(matches)-1] + len(segment)
			}
			sequencesToRemove.Insert(fmt.Sprint(segment), [2]int{sequenceStart, sequenceEnd})
			// (checking if remaining points are on a straight line could be done here
			//  but is not necessary because snap always inserts it)
			// skip past matched section and reset visitedPoints
			i = endPointIdx - 1
			visitedPoints = [][2]float64{}
		}
	}
	return removeSequences(ring, sequencesToRemove)
}

func removeSequences(ring [][2]float64, sequencesToRemove *sortedmap.SortedMap[string, [2]int]) (newRing [][2]float64) {
	mmap := sequencesToRemove.Map()
	keepFrom := 0
	for _, key := range sequencesToRemove.Keys() {
		sequenceToRemove := mmap[key]
		keepTo := sequenceToRemove[0]
		newRing = append(newRing, ring[keepFrom:keepTo]...)
		keepFrom = sequenceToRemove[1]
	}
	newRing = append(newRing, ring[keepFrom:]...)
	return newRing
}

// repeatedly calls kmpSearch, returning all starting indexes of 'find' in 'corpus'
func kmpSearchAll(corpus, find [][2]float64) []int {
	matches := []int{}
	offset := 0
	for {
		match := kmpSearch(corpus, find)
		if match == len(corpus) {
			// no match found
			break
		}
		matches = append(matches, match+offset)
		offset += match + len(find)
		corpus = corpus[match+len(find):]
		if len(corpus) < len(find) {
			// corpus is smaller than find --> no further matches possible
			break
		}
	}
	return matches
}

// returns the index (0 based) of the start of 'find' in 'corpus', or returns the length of 'corpus' on failure
func kmpSearch(corpus, find [][2]float64) int {
	m, i := 0, 0
	table := make([]int, max(len(corpus), 2))
	kmpTable(find, table)
	for m+i < len(corpus) {
		if find[i] == corpus[m+i] {
			if i == len(find)-1 {
				return m
			}
			i++
		} else {
			if table[i] > -1 {
				i = table[i]
				m = m + i - table[i] //
			} else {
				i = 0
				m++
			}
		}
	}
	return len(corpus)
}

// populates the partial match table 'table' for 'find'
func kmpTable(find [][2]float64, table []int) {
	pos, cnd := 2, 0
	table[0], table[1] = -1, 0
	for pos < len(find) {
		switch {
		case find[pos-1] == find[cnd]:
			cnd++
			table[pos] = cnd
			pos++
		case cnd > 0:
			cnd = table[cnd]
		default:
			table[pos] = 0
			pos++
		}
	}
}

func lastElement[T any](elements []T) *T {
	length := len(elements)
	if length > 0 {
		return &elements[length-1]
	}
	return nil
}

func asKeys[T constraints.Ordered](elements []T) map[T]any {
	mapped := make(map[T]any, len(elements))
	for _, element := range elements {
		mapped[element] = struct{}{}
	}
	return mapped
}

func findLastKeyWithMaxValue[K comparable, V constraints.Ordered](m *orderedmap.OrderedMap[K, V]) (maxK K, maxV V, numWinners uint) {
	first := true
	for p := m.Newest(); p != nil; p = p.Prev() {
		if first || p.Value > maxV {
			maxK = p.Key
			maxV = p.Value
			numWinners = 1
			first = false
			continue
		}
		if p.Value == maxV {
			numWinners++
		}
	}
	return
}

func orderedMapKeys[K comparable, V any](m *orderedmap.OrderedMap[K, V]) []K {
	l := make([]K, m.Len())
	i := 0
	for p := m.Oldest(); p != nil; p = p.Next() {
		l[i] = p.Key
		i++
	}
	return l
}

func lastMatch[T comparable](haystack, needle []T) T {
	for i := len(haystack) - 1; i >= 0; i-- {
		if slices.Contains(needle, haystack[i]) {
			return haystack[i]
		}
	}
	var empty T
	return empty
}

func floatPolygonsToGeomPolygonsForAllLevels(floatersPerLevel map[Level][][][][2]float64) map[Level][]geom.Polygon {
	geomsPerLevel := make(map[Level][]geom.Polygon, len(floatersPerLevel))
	for l := range floatersPerLevel {
		geomsPerLevel[l] = floatPolygonsToGeomPolygons(floatersPerLevel[l])
	}
	return geomsPerLevel
}

func floatPolygonsToGeomPolygons(floaters [][][][2]float64) []geom.Polygon {
	geoms := make([]geom.Polygon, len(floaters))
	for i := range floaters {
		geoms[i] = floaters[i]
	}
	return geoms
}

func floatPolygonToGeomPolygon(floater [][][2]float64) geom.Polygon {
	return floater
}

func floatRingToGeomPolygon(floater [][2]float64) geom.Polygon {
	return geom.Polygon{floater}
}

func panicNoPointsFoundForVertices(segment [2][2]float64, level Level) {
	panic(fmt.Sprintf("no points found for %v on level %v", segment, level))
}

func panicPartialRingsRemainingOnStack(stack *orderedmap.OrderedMap[int, [][2]float64]) {
	panicMsg := fmt.Sprintf("reached end of ring with stack length %d, expected 0\nremaining stack:\n", stack.Len())
	for r := stack.Oldest(); r != nil; r = r.Next() {
		panicMsg = fmt.Sprintf("%s\tkey %d: %v\n", panicMsg, r.Key, r.Value)
	}
	panic(panicMsg)
}

func panicInnerRingsButNoOuterRings(level Level, polygon [][][2]float64, innerRings [][][2]float64) {
	panic(fmt.Errorf("inner rings but no outer rings, on level %v, for polygon:\n%v\n\ninnerrings:%v",
		level,
		truncatedWkt(floatPolygonToGeomPolygon(polygon), 100),
		wkt.MustEncode(geom.Polygon{innerRings[0]})))
}

func panicNoMatchingOuterForInnerRing(polygons [][][][2]float64, innerRing [][2]float64) {
	panicMsg := "no matching outer ring for inner ring.\ninner: " + wkt.MustEncode(floatRingToGeomPolygon(innerRing))
	panicMsg += "\nouters:"
	for _, polygon := range floatPolygonsToGeomPolygons(polygons) {
		panicMsg += "\n" + wkt.MustEncode(geom.Polygon{polygon[0]})
	}
	panic(panicMsg)
}

func panicMoreThanOneMatchingOuterRing(polygons [][][][2]float64, innerRing [][2]float64) {
	panicMsg := "more than one matching outer ring for inner ring.\ninner: " + wkt.MustEncode(floatRingToGeomPolygon(innerRing))
	panicMsg += "\nouters:"
	for _, polygon := range floatPolygonsToGeomPolygons(polygons) {
		panicMsg += "\n" + wkt.MustEncode(geom.Polygon{polygon[0]})
	}
	panic(panicMsg)
}

func panicDeletedPolygonHasInnerRing(polygon [][][2]float64) {
	panicMsg := fmt.Sprintf("a deleted dupe polygon had more than one ring, %v",
		truncatedWkt(floatPolygonToGeomPolygon(polygon), 100))
	panic(panicMsg)
}

func truncatedWkt(geom geom.Geometry, width uint) string {
	return truncate.StringWithTail(wkt.MustEncode(geom), width, "...")
}

func reverseClone[S ~[]E, E any](s S) S {
	if s == nil {
		return nil
	}
	l := len(s)
	c := make(S, l)
	for i := 0; i < l; i++ {
		c[l-1-i] = s[i]
	}
	return c
}

func roundFloat(f float64, p uint) float64 {
	r := math.Pow(10, float64(p))
	return math.Round(f*r) / r
}
