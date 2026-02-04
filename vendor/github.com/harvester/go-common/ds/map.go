package ds

// MapFilterFunc iterates over elements of map, returning a map of all elements
// the function f returns truthy for. The function f is invoked with three
// arguments: (value, key).
func MapFilterFunc[M ~map[K]V, K comparable, V any](m M, f func(V, K) bool) map[K]V {
	r := make(map[K]V, len(m))
	for k, v := range m {
		if f(v, k) {
			r[k] = v
		}
	}
	return r
}

// MapKeys returns the keys of the map m. The keys will be in an indeterminate
// order.
func MapKeys[M ~map[K]V, K comparable, V any](m M) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// MapValues returns the values of the map m. The values will be in an
// indeterminate order.
func MapValues[M ~map[K]V, K comparable, V any](m M) []V {
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}
