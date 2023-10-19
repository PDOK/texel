package snap

import (
	"fmt"
	"slices"
	"strings"

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
func kmpDedupe(newRing [][2]float64) [][2]float64 {
	// start with a copy of newRing
	dedupedRing := make([][2]float64, len(newRing))
	copy(dedupedRing, newRing)
	// build map of indices to remove, sorted by starting index of each stretch to remove
	indicesToRemove := sortedmap.New(len(newRing), func(x, y interface{}) bool {
		return x.([2]int)[0] < y.([2]int)[0]
	})
	// given a segment (of arbitrary length n) s with its reverse s':
	// find an index where newRing contains, in order, s followed by one or more times s'+s
	for n := (len(newRing) / 3) + 2; n > 1; n-- {
		for j := 0; j < len(newRing)-n; {
			segment := newRing[j : j+n]
			matches := kmpSearchAll(newRing, segment)
			reverseSegment := make([][2]float64, len(segment))
			copy(reverseSegment, segment)
			slices.Reverse(reverseSegment)
			reverseMatches := kmpSearchAll(newRing, reverseSegment)
			if len(matches) > 1 && (len(matches)-len(reverseMatches)) == 1 {
				segmentKey := fmt.Sprintf("%v", segment)
				segmentKey = segmentKey[1 : len(segmentKey)-1]
				exists := keyOrSubstringOfKey(indicesToRemove, segmentKey)
				if !exists {
					segmentRec := sortedmap.Record{
						Key: segmentKey,
						Val: [2]int{matches[0] + len(segment), matches[len(matches)-1] + len(segment)},
					}
					indicesToRemove.Insert(segmentRec.Key, segmentRec.Val)
				}
				// skip past repeated section
				j += (len(matches) + len(reverseMatches)) * len(segment)
			} else if len(matches) > 1 && len(matches) == len(reverseMatches) {
				j += 2 * len(matches) * len(segment)
			} else {
				j++
			}
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

// checks if a key already exists, or is a substring of a key
// (e.g., if mapToCheck has key 'A B C', then keyToCheck 'A B' returns true)
func keyOrSubstringOfKey(mapToCheck *sortedmap.SortedMap, keyToCheck string) bool {
	for _, key := range mapToCheck.Keys() {
		if keyToCheck == key.(string) || strings.Contains(key.(string), keyToCheck) {
			return true
		}
	}
	return false
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
		if len(corpus) < len(find) {
			// corpus is smaller than find, no further matches possible
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
