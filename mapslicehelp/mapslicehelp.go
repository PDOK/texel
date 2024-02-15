package mapslicehelp

import (
	"slices"

	"github.com/tobshub/go-sortedmap"

	orderedmap "github.com/wk8/go-ordered-map/v2"
	"golang.org/x/exp/constraints"
)

func LastElement[T any](elements []T) *T {
	length := len(elements)
	if length > 0 {
		return &elements[length-1]
	}
	return nil
}

func AsKeys[T constraints.Ordered](elements []T) map[T]any {
	mapped := make(map[T]any, len(elements))
	for _, element := range elements {
		mapped[element] = struct{}{}
	}
	return mapped
}

func FindLastKeyWithMaxValue[K comparable, V constraints.Ordered](m *orderedmap.OrderedMap[K, V]) (maxK K, maxV V, numWinners uint) {
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

func OrderedMapKeys[K comparable, V any](m *orderedmap.OrderedMap[K, V]) []K {
	l := make([]K, m.Len())
	i := 0
	for p := m.Oldest(); p != nil; p = p.Next() {
		l[i] = p.Key
		i++
	}
	return l
}

func RemoveSequences[V any, K comparable](s []V, sequencesToRemove *sortedmap.SortedMap[K, [2]int]) (newS []V) {
	mmap := sequencesToRemove.Map()
	keepFrom := 0
	for _, key := range sequencesToRemove.Keys() {
		sequenceToRemove := mmap[key]
		keepTo := sequenceToRemove[0]
		newS = append(newS, s[keepFrom:keepTo]...)
		keepFrom = sequenceToRemove[1]
	}
	newS = append(newS, s[keepFrom:]...)
	return newS
}

func LastMatch[T comparable](haystack, needle []T) T {
	for i := len(haystack) - 1; i >= 0; i-- {
		if slices.Contains(needle, haystack[i]) {
			return haystack[i]
		}
	}
	var empty T
	return empty
}

func ReverseClone[S ~[]E, E any](s S) S {
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

func DeleteFromSliceByIndex[V any, X any](s []V, indexesToDelete map[int]X, indexOffset int) []V {
	r := make([]V, 0, len(s))
	for i := range s {
		if _, skip := indexesToDelete[i+indexOffset]; skip {
			continue
		}
		r = append(r, s[i])
	}
	return r
}

func CountVals[K, V comparable](m *orderedmap.OrderedMap[K, V], v V) int {
	n := 0
	for p := m.Oldest(); p != nil; p = p.Next() {
		if p.Value == v {
			n++
		}
	}
	return n
}
