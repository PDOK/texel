package snap

import (
	"container/list"
	"fmt"
	"math"
	"slices"
	"sort"

	orderedmap "github.com/wk8/go-ordered-map/v2"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/intgeom"
	"github.com/pdok/texel/tms20"
	"github.com/umpc/go-sortedmap"
	"golang.org/x/exp/constraints"
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
	newPolygons := make(map[Level][][][][2]float64, len(levels))
	newPointsAndLines := make(map[Level][][][2]float64, len(levels))

	// Could use polygon.AsSegments(), but it skips rings with <3 segments and starts with the last segment.
	for ringIdx, ring := range polygon.LinearRings() {
		if len(levelMap) == 0 { // level could have been obsoleted
			continue
		}
		isOuter := ringIdx == 0
		// winding order is reversed if incorrect
		ensureCorrectWindingOrder(ring, !isOuter)
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
			if isOuter && len(outerRings) == 0 && (!keepPointsAndLines || len(pointsAndLines) == 0) {
				delete(levelMap, level) // outer ring has become too small
				continue
			}
			for _, outerRing := range outerRings { // (there are only outer rings if isOuter)
				newPolygons[level] = append(newPolygons[level], [][][2]float64{outerRing})
			}
			if len(newPolygons[level]) == 0 && len(innerRings) > 0 { // should never happen
				panic(fmt.Errorf("inner rings but no outer rings, for %v on level %v", polygon, level))
			}
			newPolygons[level] = matchInnersToPolygon(newPolygons[level], innerRings)
			if keepPointsAndLines {
				newPointsAndLines[level] = append(newPointsAndLines[level], pointsAndLines...)
			}
		}
	}
	// points and lines at the end, as outer rings
	for level, pointsAndLines := range newPointsAndLines {
		for _, pointOrLine := range pointsAndLines {
			newPolygons[level] = append(newPolygons[level], [][][2]float64{pointOrLine})
		}
	}
	return floatPolygonsToGeomPolygons(newPolygons)
}

func matchInnersToPolygon(polygons [][][][2]float64, innerRings [][][2]float64) [][][][2]float64 {
	lenPolygons := len(polygons)
	if lenPolygons == 0 {
		return polygons
	} else if lenPolygons == 1 {
		polygons[0] = append(polygons[0], innerRings...)
		return polygons
	}
matchInners:
	for _, innerRing := range innerRings {
		containsPerPolyI := make(map[int]uint, lenPolygons)
		// this is pretty nested, but usually breaks early
		for _, vertex := range innerRing {
			for polyI := range polygons {
				contains, onBoundary := ringContains(polygons[polyI][0], vertex)
				if contains {
					if !onBoundary {
						polygons[polyI] = append(polygons[polyI], innerRing)
						continue matchInners
					}
					containsPerPolyI[polyI]++
				}
			}
			matchingPolyI, _, matchCount := findKeyWithMaxValue(containsPerPolyI)
			if matchCount == 1 {
				polygons[matchingPolyI] = append(polygons[matchingPolyI], innerRing)
				continue matchInners
			}
		}
		// no (single) matching outer ring was found (should never happen)
		if len(containsPerPolyI) == 0 {
			panic(fmt.Errorf("no matching outer ring for inner ring.\npolygons: %v\ninner: %v", polygons, innerRing))
		}
		panic(fmt.Errorf("more than one matching outer ring for inner ring.\npolygons: %v\ninner: %v", polygons, innerRing))
	}
	return polygons
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
func rayIntersect(p, s, e [2]float64) (intersects, on bool) {
	if s[0] > e[0] {
		s, e = e, s
	}

	if p[0] == s[0] {
		if p[1] == s[1] {
			// p == start
			return false, true
		} else if s[0] == e[0] {
			// vertical segment (s -> e)
			// return true if within the line, check to see if start or end is greater.
			if s[1] > e[1] && s[1] >= p[1] && p[1] >= e[1] {
				return false, true
			}

			if e[1] > s[1] && e[1] >= p[1] && p[1] >= s[1] {
				return false, true
			}
		}

		// Move the y coordinate to deal with degenerate case
		p[0] = math.Nextafter(p[0], math.Inf(1))
	} else if p[0] == e[0] {
		if p[1] == e[1] {
			// matching the end point
			return false, true
		}

		p[0] = math.Nextafter(p[0], math.Inf(1))
	}

	if p[0] < s[0] || p[0] > e[0] {
		return false, false
	}

	if s[1] > e[1] {
		if p[1] > s[1] {
			return false, false
		} else if p[1] < e[1] {
			return true, false
		}
	} else {
		if p[1] > e[1] {
			return false, false
		} else if p[1] < s[1] {
			return true, false
		}
	}

	rs := (p[1] - s[1]) / (p[0] - s[0])
	ds := (e[1] - s[1]) / (e[0] - s[0])

	if rs == ds {
		return false, true
	}

	return rs <= ds, false
}

// cleanupNewVertices cleans up the closest points for a line that were just retrieved inside addPointsAndSnap
func cleanupNewVertices(newVertices [][2]float64, segment [2][2]float64, level Level, lastVertex *[2]float64) [][2]float64 {
	newVerticesCount := len(newVertices)
	if newVerticesCount == 0 { // should never happen, SnapClosestPoints should have returned at least one point
		panic(fmt.Sprintf("no points found for %v on level %v", segment, level))
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

func floatPolygonsToGeomPolygons(floaterss map[Level][][][][2]float64) map[Level][]geom.Polygon {
	geomsPerLevel := make(map[Level][]geom.Polygon, len(floaterss))
	for l := range floaterss {
		geomsPerLevel[l] = make([]geom.Polygon, len(floaterss[l]))
		for i := range floaterss[l] {
			geomsPerLevel[l][i] = floaterss[l][i]
		}
	}
	return geomsPerLevel
}

// if winding order is incorrect, ring is reversed to correct winding order
func ensureCorrectWindingOrder(ring [][2]float64, shouldBeClockwise bool) {
	if !windingOrderIsCorrect(ring, shouldBeClockwise) {
		slices.Reverse(ring)
	}
}

// validate winding order (CCW for outer rings, CW for inner rings)
func windingOrderIsCorrect(ring [][2]float64, shouldBeClockwise bool) bool {
	// modulo function that returns least positive remainder (i.e., mod(-1, 5) returns 4)
	mod := func(a, b int) int {
		return (a%b + b) % b
	}
	i, _ := findRightmostLowestPoint(ring)
	// check angle between the vectors goint into and coming out of the rightmost lowest point
	points := [3][2]float64{ring[mod(i-1, len(ring))], ring[i], ring[mod(i+1, len(ring))]}
	return isClockwise(points) == shouldBeClockwise
}

// TODO: rewrite by using intgeoms for as long as possible
func isHitMultiple(hitMultiple map[intgeom.Point][]int, vertex [2]float64, ringIdx int) bool {
	intVertex := intgeom.FromGeomPoint(vertex)
	return slices.Contains(hitMultiple[intVertex], ringIdx) || // exact match
		slices.Contains(hitMultiple[intgeom.Point{intVertex[0] + 1, intVertex[1]}], ringIdx) || // fuzzy search
		slices.Contains(hitMultiple[intgeom.Point{intVertex[0] - 1, intVertex[1]}], ringIdx) ||
		slices.Contains(hitMultiple[intgeom.Point{intVertex[0], intVertex[1] + 1}], ringIdx) ||
		slices.Contains(hitMultiple[intgeom.Point{intVertex[0], intVertex[1] - 1}], ringIdx)
}

// split ring into multiple rings at any point where the ring goes through the point more than once
//
//nolint:cyclop,funlen,gocritic
func splitRing(ring [][2]float64, isOuter bool, hitMultiple map[intgeom.Point][]int, ringIdx int) (outerRings, innerRings, pointsAndLines [][][2]float64) {
	partialRingIdx := 0
	stack := orderedmap.New[int, [][2]float64]()
	stack.Set(partialRingIdx, [][2]float64{})
	stackKeys := list.New()
	stackKeys.PushBack(partialRingIdx)
	completeRings := make(map[int][][2]float64)
	completeRingKeys := []int{}
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
			completeRingKeys = append(completeRingKeys, partialRingIdx)
			stack.Delete(partialRingIdx)
			removeByValue(stackKeys, partialRingIdx)
		} else {
			// keep prepending partial rings from the stack until tempRing is a complete ring, or the end of the stack is reached
			partialsToRemove := []int{partialRingIdx}
			for r := stackKeys.Back().Prev(); r != nil; r = r.Prev() {
				stackIdx := r.Value.(int)
				partialRingFromStack := stack.Value(stackIdx)
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
					completeRingKeys = append(completeRingKeys, stackIdx)
					for _, idx := range partialsToRemove {
						stack.Delete(idx)
						removeByValue(stackKeys, idx)
					}
					break
				}
			}
		}
		if vertexIdx < len(checkRing)-1 {
			partialRingIdx++
			stackKeys.PushBack(partialRingIdx)
			stack.Set(partialRingIdx, append(stack.Value(partialRingIdx), vertex))
		} else if stack.Len() > 0 {
			// if partial rings remain on stack when end of ring is reached, something has gone wrong
			panicMsg := fmt.Sprintf("reached end of ring with stack length %d, expected 0\nremaining stack:\n", stack.Len())
			for r := stackKeys.Front(); r != nil; r = r.Next() {
				stackIdx := r.Value.(int)
				panicMsg = fmt.Sprintf("%s\tkey %d: %v\n", panicMsg, stackIdx, stack.Value(stackIdx))
			}
			panic(panicMsg)
		}
	}
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
	return outerRings, innerRings, pointsAndLines
}

// determines whether a pair of vectors turns clockwise by examining the relationship of their relative angle to the angles of the vectors
func isClockwise(points [3][2]float64) bool {
	// modulo function that returns least positive remainder (i.e., mod(-1, 5) returns 4)
	mod := func(a, b float64) float64 {
		return math.Mod(math.Mod(a, b)+b, b)
	}
	vector1 := vector2d{x: (points[1][0] - points[0][0]), y: (points[1][1] - points[0][1])}
	vector2 := vector2d{x: (points[2][0] - points[1][0]), y: (points[2][1] - points[1][1])}
	relativeAngle := math.Acos(vector1.dot(vector2)/(vector1.magnitude()*vector2.magnitude())) * (180 / math.Pi)
	return math.Round(mod((vector2.angle()-relativeAngle), 360)) != math.Round(vector1.angle())
}

// deduplication using an implementation of the Knuth-Morris-Pratt algorithm
//
//nolint:cyclop,funlen
func kmpDeduplicate(newRing [][2]float64) [][2]float64 {
	deduplicatedRing := make([][2]float64, len(newRing))
	copy(deduplicatedRing, newRing)
	// map of indices to remove, sorted by starting index of each sequence to remove
	indicesToRemove := sortedmap.New(len(newRing), func(x, y interface{}) bool {
		return x.([2]int)[0] < y.([2]int)[0]
	})
	// walk through newRing until a step back is taken, then identify how many steps back are taken and search for repeats
	visitedPoints := [][2]float64{}
	for i := 0; i < len(newRing); {
		vertex := newRing[i]
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
			if nextI <= len(newRing)-1 && visitedPoints[len(visitedPoints)-j] == newRing[nextI] {
				reverseSegment = append(reverseSegment, visitedPoints[len(visitedPoints)-j])
			} else {
				// end of segment
				break
			}
		}
		// create segment from reverse segment
		segment := make([][2]float64, len(reverseSegment))
		copy(segment, reverseSegment)
		slices.Reverse(segment)
		// create search corpus: initialise with section of 3*segment length, then continuously
		// add 2*segment length until corpus contains a point that is not in segment
		start := i - len(segment)
		end := start + (3 * len(segment))
		k := 0
		corpus := newRing[start:min(end, len(newRing))]
		for {
			stop := false
			// check if (additional) corpus contains a point that is not in segment
			for _, vertex := range corpus[k:] {
				if !slices.Contains(segment, vertex) {
					stop = true
					break
				}
			}
			// corpus already runs until the end of newRing
			if end > len(newRing) {
				stop = true
			}
			if stop {
				break
			}
			// expand corpus
			k = len(corpus)
			corpus = append(corpus, newRing[end:min(end+(2*len(segment)), len(newRing))]...)
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
			segmentRec := sortedmap.Record{
				Key: fmt.Sprintf("%v", segment),
				Val: [2]int{sequenceStart, sequenceEnd},
			}
			indicesToRemove.Insert(segmentRec.Key, segmentRec.Val)
			// skip past matched section and reset visitedPoints
			i = sequenceEnd
			visitedPoints = [][2]float64{}
		case len(matches) > 1 && len(matches) == len(reverseMatches):
			// multiple backtrace found (segment occurs more than once, and equally as many times as its reverse)
			// mark all but one occurrance of segment and one occurrance of its reverse for removal
			sequenceStart := start + (2 * len(segment)) - 1
			sequenceEnd := start + matches[len(matches)-1] + len(segment)
			segmentRec := sortedmap.Record{
				Key: fmt.Sprintf("%v", segment),
				Val: [2]int{sequenceStart, sequenceEnd},
			}
			indicesToRemove.Insert(segmentRec.Key, segmentRec.Val)
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
			segmentRec := sortedmap.Record{
				Key: fmt.Sprintf("%v", segment),
				Val: [2]int{sequenceStart, sequenceEnd},
			}
			indicesToRemove.Insert(segmentRec.Key, segmentRec.Val)
			// check if remaining points are on a straight line, retain only start and end points if so
			startPointX := newRing[sequenceEnd][0]
			startPointY := newRing[sequenceEnd][1]
			onLine := true
			furthestDistance := 0.0
			furthestPointIdx := sequenceEnd
			for n := sequenceEnd + 1; n < endPointIdx; n++ {
				if newRing[n][0] != startPointX && newRing[n][1] != startPointY {
					onLine = false
					break
				}
				pointDistance := math.Sqrt(math.Abs(newRing[n][0]-startPointX) + math.Abs(newRing[n][1]-startPointY))
				if pointDistance > furthestDistance {
					furthestDistance = pointDistance
					furthestPointIdx = n
				}
			}
			if onLine {
				// remove any intermediate points on the return from the end point to the start point
				sequenceStart := furthestPointIdx + 1
				sequenceEnd := endPointIdx - 1
				segmentRec := sortedmap.Record{
					Key: fmt.Sprintf("%v", newRing[furthestPointIdx:furthestPointIdx+1]),
					Val: [2]int{sequenceStart, sequenceEnd},
				}
				indicesToRemove.Insert(segmentRec.Key, segmentRec.Val)
			}
			// skip past matched section and reset visitedPoints
			i = endPointIdx - 1
			visitedPoints = [][2]float64{}
		}
	}
	offset := 0
	for _, key := range indicesToRemove.Keys() {
		sequence := indicesToRemove.Map()[key].([2]int)
		deduplicatedRing = append(deduplicatedRing[:sequence[0]-offset], deduplicatedRing[sequence[1]-offset:]...)
		offset += sequence[1] - sequence[0]
	}
	return deduplicatedRing
}

func findRightmostLowestPoint(ring [][2]float64) (int, [2]float64) {
	rightmostLowestPoint := [2]float64{math.MinInt, math.MaxInt}
	j := 0
	for i, vertex := range ring {
		// check if vertex is rightmost lowest point (either y is lower, or y is equal and x is higher)
		if vertex[1] < rightmostLowestPoint[1] {
			rightmostLowestPoint[0] = vertex[0]
			rightmostLowestPoint[1] = vertex[1]
			j = i
		} else if vertex[1] == rightmostLowestPoint[1] && vertex[0] > rightmostLowestPoint[0] {
			rightmostLowestPoint[0] = vertex[0]
			j = i
		}
	}
	return j, rightmostLowestPoint
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

// helper function to remove element from linked list by value
func removeByValue(l *list.List, r int) {
	for e := l.Front(); e != nil; e = e.Next() {
		if e.Value.(int) == r {
			l.Remove(e)
		}
	}
}

func findKeyWithMaxValue[K comparable, V constraints.Ordered](m map[K]V) (maxK K, maxV V, winners uint) {
	first := true
	for k, v := range m {
		if first || v > maxV {
			maxK = k
			maxV = v
			winners = 1
			first = false
			continue
		}
		if v == maxV {
			winners++
		}
	}
	return
}
