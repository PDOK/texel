package snap

import (
	"fmt"
	"math"
	"slices"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/processing"
	"github.com/umpc/go-sortedmap"
)

func snapPolygon(polygon *geom.Polygon, tileMatrix TileMatrix) *geom.Polygon {
	ix := NewPointIndexFromTileMatrix(tileMatrix)
	ix.InsertPolygon(polygon)
	newPolygon := addPointsAndSnap(ix, polygon)

	return newPolygon
}

//nolint:cyclop
func addPointsAndSnap(ix *PointIndex, polygon *geom.Polygon) *geom.Polygon {
	newPolygon := make([][][2]float64, 0, len(*polygon))
	// Could use polygon.AsSegments(), but it skips rings with <3 segments and starts with the last segment.
	for ringIdx, ring := range polygon.LinearRings() {
		ringLen := len(ring)
		newRing := make([][2]float64, 0, ringLen*2) // TODO better estimation of new amount of points
		for vertexIdx, vertex := range ring {
			// LinearRings(): "The last point in the linear ring will not match the first point."
			// So also including that one.
			nextVertexIdx := (vertexIdx + 1) % ringLen
			segment := geom.Line{vertex, ring[nextVertexIdx]}
			newVertices := ix.SnapClosestPoints(segment)
			newVerticesCount := len(newVertices)
			if newVerticesCount > 0 {
				// 0 if len is 1, 1 otherwise
				minus := min(newVerticesCount-1, 1)
				// remove last vertex if there is more than 1 vertex, as the first vertex in the next segment will be the same
				newVertices = newVertices[:newVerticesCount-minus]
				// remove first element if it is equal to the last element added to newRing
				if len(newRing) > 0 && newVertices[0] == newRing[len(newRing)-1] {
					newVertices = newVertices[1:]
				}
				newRing = append(newRing, newVertices...)
			} else {
				panic(fmt.Sprintf("no points found for %v", segment))
			}
		}
		// LinearRings(): "The last point in the linear ring will not match the first point."
		if len(newRing) > 1 && newRing[0] == newRing[len(newRing)-1] {
			newRing = newRing[:len(newRing)-1]
		}
		switch len(newRing) {
		case 0:
			if ringIdx == 0 {
				// outer ring has become too small
				return nil
			}
		case 1, 2:
			if ringIdx == 0 {
				// keep outer ring as point or line
				newPolygon = append(newPolygon, newRing)
			}
		default:
			// deduplicate points in the ring, check winding order, then add to the polygon
			deduplicatedRing, rightmostLowestPoint := kmpDeduplicate(newRing)
			// inner rings (ringIdx != 0) should be clockwise
			shouldBeClockwise := ringIdx != 0
			// winding order is reversed if incorrect
			ensureCorrectWindingOrder(deduplicatedRing, rightmostLowestPoint, shouldBeClockwise)
			newPolygon = append(newPolygon, deduplicatedRing)
		}
	}
	return (*geom.Polygon)(&newPolygon)
}

// validate winding order (CCW for outer rings, CW for inner rings)
// if winding order is incorrect, ring is reversed to correct winding order
func ensureCorrectWindingOrder(ring [][2]float64, rightmostLowestPoint [2]float64, shouldBeClockwise bool) {
	// modulo function that returns least positive remainder (i.e., mod(-1, 5) returns 4)
	mod := func(a, b int) int {
		return (a%b + b) % b
	}
	i := slices.Index(ring, rightmostLowestPoint)
	// check angle between the vectors goint into and coming out of the rightmost lowest point
	points := [3][2]float64{ring[mod(i-1, len(ring))], ring[i], ring[mod(i+1, len(ring))]}
	if isClockwise(points, shouldBeClockwise) != shouldBeClockwise {
		slices.Reverse(ring)
	}
}

// determines whether a pair of vectors turns clockwise by examining the relationship of their relative angle to the angles of the vectors
func isClockwise(points [3][2]float64, shouldBeClockwise bool) bool {
	// modulo function that returns least positive remainder (i.e., mod(-1, 5) returns 4)
	mod := func(a, b float64) float64 {
		return math.Mod(math.Mod(a, b)+b, b)
	}
	vector1 := vector2d{x: (points[1][0] - points[0][0]), y: (points[1][1] - points[0][1])}
	vector2 := vector2d{x: (points[2][0] - points[1][0]), y: (points[2][1] - points[1][1])}
	relativeAngle := math.Acos(vector1.dot(vector2)/(vector1.magnitude()*vector2.magnitude())) * (180 / math.Pi)
	if math.Round(relativeAngle) == 0.0 || math.Round(relativeAngle) == 180.0 {
		return shouldBeClockwise
	}
	return math.Round(mod((vector2.angle()-relativeAngle), 360)) != math.Round(vector1.angle())
}

// deduplication using an implementation of the Knuth-Morris-Pratt algorithm
//
//nolint:cyclop,funlen
func kmpDeduplicate(newRing [][2]float64) ([][2]float64, [2]float64) {
	rightmostLowestPoint := [2]float64{math.MinInt, math.MaxInt}
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
		// check if vertex is rightmost lowest point (either y is lower, or y is equal and x is higher)
		if vertex[1] < rightmostLowestPoint[1] {
			rightmostLowestPoint[0] = vertex[0]
			rightmostLowestPoint[1] = vertex[1]
		} else if vertex[1] == rightmostLowestPoint[1] && vertex[0] > rightmostLowestPoint[0] {
			rightmostLowestPoint[0] = vertex[0]
		}
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
	return deduplicatedRing, rightmostLowestPoint
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

// SnapToPointCloud snaps polygons' points to a tile's internal pixel grid
// and adds points to lines to prevent intersections.
func SnapToPointCloud(source processing.Source, target processing.Target, tileMatrix TileMatrix) {
	processing.ProcessFeatures(source, target, func(p geom.Polygon) *geom.Polygon {
		return snapPolygon(&p, tileMatrix)
	})
}
