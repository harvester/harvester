package ds

import "slices"

// SliceContentCmp compares two slices and returns true if they have the same content with any order.
func SliceContentCmp[T comparable](x, y []T) bool {
	if len(x) == 0 && len(y) == 0 {
		return true
	}
	if len(x) != len(y) {
		return false
	}

	yMap := make(map[T]int, len(y))
	for _, item := range y {
		yMap[item]++
	}

	for _, xItem := range x {
		if counter, exist := yMap[xItem]; exist {
			if counter == 0 {
				return false
			}
			yMap[xItem]--
		} else {
			return false
		}
	}
	return true
}

// SliceDedupe removes duplicated items and return a non-deplucated items slice.
func SliceDedupe[T comparable](x []T) []T {
	if len(x) == 0 {
		return x
	}

	result := make([]T, 0)
	for _, item := range x {
		if !slices.Contains(result, item) {
			result = append(result, item)
		}
	}
	return result
}

// SliceMapFunc creates a new slice of values by running each element in the
// slice through function f. The function f is invoked with two arguments:
// (value, index).
func SliceMapFunc[S ~[]E, E any](s S, f func(E, int) E) S {
	r := make(S, len(s))
	for i, v := range s {
		r[i] = f(v, i)
	}
	return r
}

// SliceFindDuplicates takes a slice of comparable elements and returns a new slice containing
// only the elements that appear more than once in the input slice. Since a map is used
// internally to track duplicates, the order of the elements in the output slice is not guaranteed.
func SliceFindDuplicates[S ~[]E, E comparable](s S) S {
	elementCount := make(map[E]int)
	duplicates := []E{}

	for _, element := range s {
		elementCount[element]++
	}

	for element, count := range elementCount {
		if count > 1 {
			duplicates = append(duplicates, element)
		}
	}

	return duplicates
}
