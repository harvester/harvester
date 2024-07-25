/*
Package partition represents listing parameters. They can be used to specify which namespaces a caller would like included
in a response, or which specific objects they are looking for.
*/
package partition

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

// Partition represents filtering of a request's results
type Partition struct {
	// if true, do not apply any filtering, return all results. Overrides all other fields
	Passthrough bool

	// if non-empty, only resources in the specified namespaces will be returned
	Namespace string

	// if true, return all results, while still honoring Namespace. Overrides Names
	All bool

	// if non-empty, only resources with matching names will be returned
	Names sets.Set[string]
}
