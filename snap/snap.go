package snap

import (
	"fmt"
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
	for ringI, ring := range polygon.LinearRings() {
		ringLen := len(ring)
		newRing := make([][2]float64, 0, ringLen*2) // TODO better estimation of new amount of points
		for vertexI, vertex := range ring {
			// LinearRings(): "The last point in the linear ring will not match the first point."
			// So also including that one.
			nextVertexI := (vertexI + 1) % ringLen
			segment := geom.Line{vertex, ring[nextVertexI]}
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
			if ringI == 0 {
				// outer ring has become too small
				return nil
			}
		case 1, 2:
			if ringI == 0 {
				// keep outer ring as point or line
				newPolygon = append(newPolygon, newRing)
			}
		default:
			// deduplicate points in the ring, then add to the polygon
			newPolygon = append(newPolygon, kmpDedupe(newRing))
		}
	}
	return (*geom.Polygon)(&newPolygon)
}

// deduplication using an implementation of the Knuth-Morris-Pratt algorithm
//
//nolint:cyclop,funlen,nestif
func kmpDedupe(newRing [][2]float64) [][2]float64 {
	// start with a copy of newRing
	dedupedRing := make([][2]float64, len(newRing))
	copy(dedupedRing, newRing)
	// build map of indices to remove, sorted by starting index of each stretch to remove
	indicesToRemove := sortedmap.New(len(newRing), func(x, y interface{}) bool {
		return x.([2]int)[0] < y.([2]int)[0]
	})
	// walk through new ring until a step back is taken
	// then identify how many steps back are taken and search for zigzags
	visitedPoints := [][2]float64{}
	for i := 0; i < len(newRing); {
		vertex := newRing[i]
		if len(visitedPoints) > 1 && visitedPoints[len(visitedPoints)-2] == vertex {
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
			// create corpus, initialise with section of length 3*segment length, starting from i
			// then continuously add 2*segment length until corpus contains a point that is not in segment
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
			// search corpus for zigzag or backtrace
			matches := kmpSearchAll(corpus, segment)
			reverseMatches := kmpSearchAll(corpus, reverseSegment)
			if len(matches) > 1 && (len(matches)-len(reverseMatches)) == 1 {
				// zigzag found (segment occurs one time more than its reverse)
				// mark all but one occurrance of segment for removal
				segmentKey := fmt.Sprintf("%v", segment)
				segmentKey = segmentKey[1 : len(segmentKey)-1]
				stretchStart := start + len(segment)
				stretchEnd := start + matches[len(matches)-1] + len(segment)
				segmentRec := sortedmap.Record{
					Key: segmentKey,
					Val: [2]int{stretchStart, stretchEnd},
				}
				indicesToRemove.Insert(segmentRec.Key, segmentRec.Val)
				// skip past matched section and reset visitedPoints
				i = stretchEnd
				visitedPoints = [][2]float64{}
			} else if len(matches) > 1 && len(matches) == len(reverseMatches) {
				// multiple backtrace found (segment occurs more than once, and equally as many times as its reverse)
				// mark all but one occurrance of segment and one occurrance of its reverse for removal
				segmentKey := fmt.Sprintf("%v", segment)
				segmentKey = segmentKey[1 : len(segmentKey)-1]
				stretchStart := start + (2 * len(segment)) - 1
				stretchEnd := start + matches[len(matches)-1] + len(segment)
				segmentRec := sortedmap.Record{
					Key: segmentKey,
					Val: [2]int{stretchStart, stretchEnd},
				}
				indicesToRemove.Insert(segmentRec.Key, segmentRec.Val)
				// skip past matched section and reset visitedPoints
				i = stretchEnd
				visitedPoints = [][2]float64{}
			} else if len(matches) == 1 && len(reverseMatches) == 1 {
				// backtrace found (segment and its reverse occur exactly once)
				// no removal necessary, skip past matched section and reset visitedPoints
				i = start + (2 * len(segment)) - 1
				visitedPoints = [][2]float64{}
			} else if len(reverseMatches) > len(matches) {
				// zigzag with one or more stray points in between (segment occurs fewer times than its reverse)
				tmpCorpus := make([][2]float64, len(corpus))
				copy(tmpCorpus, corpus)
				tmpCorpus = append(tmpCorpus[:len(segment)], tmpCorpus[reverseMatches[0]+len(segment):]...)
				zigzag := false
				vp := [][2]float64{}
				for ic := 0; ic < len(tmpCorpus); {
					vt := tmpCorpus[ic]
					if len(vp) > 1 && vp[len(vp)-2] == vt {
						rs := [][2]float64{vp[len(vp)-1], vp[len(vp)-2]}
						for jc := 3; jc <= len(vp); jc++ {
							nextIc := ic + (jc - 2)
							if nextIc <= len(tmpCorpus)-1 && vp[len(vp)-jc] == tmpCorpus[nextIc] {
								rs = append(rs, vp[len(vp)-jc])
							} else {
								// end of segment
								break
							}
						}
						// create segment from reverse segment
						s := make([][2]float64, len(rs))
						copy(s, rs)
						slices.Reverse(s)
						if len(kmpSearchAll(tmpCorpus, s)) == 1 && len(kmpSearchAll(tmpCorpus, rs)) == 1 {
							zigzag = true
							segmentKey := fmt.Sprintf("%v", segment)
							segmentKey = segmentKey[1 : len(segmentKey)-1]
							stretchStart := start + len(segment)
							stretchEnd := stretchStart + len(segment) - 1
							segmentRec := sortedmap.Record{
								Key: segmentKey,
								Val: [2]int{stretchStart, stretchEnd},
							}
							indicesToRemove.Insert(segmentRec.Key, segmentRec.Val)
						}
						break
					}
					vp = append(vp, vt)
					ic++
				}
				// unable to fix the zigzag
				if !zigzag {
					panic(fmt.Sprintf("not a broken zigzag: %v\n", corpus))
				}
				// skip past matched section and reset visitedPoints
				i = start + reverseMatches[len(reverseMatches)-1] + len(segment)
				visitedPoints = [][2]float64{}
			}
		} else {
			visitedPoints = append(visitedPoints, vertex)
			i++
		}
	}
	offset := 0
	for _, key := range indicesToRemove.Keys() {
		stretch := indicesToRemove.Map()[key].([2]int)
		dedupedRing = append(dedupedRing[:stretch[0]-offset], dedupedRing[stretch[1]-offset:]...)
		offset += stretch[1] - stretch[0]
	}
	return dedupedRing
}

// kmpSearchAll repeatedly calls kmpSearch, returning all starting indexes of 'find' in 'corpus'
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
		if len(corpus) < len(find) { //|| corpus[1] != find[1] {
			// corpus is smaller than find --> no further matches possible
			// or second point of remaining corpus doesn't match second point of find
			break
		}
	}
	return matches
}

// kmpSearch returns the index (0 based) of the start of the string 'find' in 'corpus', or returns the length of 'corpus' on failure.
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

// kmpTable populates the partial match table 'table' for the string 'find'.
func kmpTable(find [][2]float64, table []int) {
	pos, cnd := 2, 0
	table[0], table[1] = -1, 0
	for pos < len(find) {
		if find[pos-1] == find[cnd] {
			cnd++
			table[pos] = cnd
			pos++
		} else if cnd > 0 {
			cnd = table[cnd]
		} else {
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
