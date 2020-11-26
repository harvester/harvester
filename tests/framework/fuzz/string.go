package fuzz

import (
	"k8s.io/apimachinery/pkg/util/rand"
)

// String returns a random string with given size.
func String(size int) string {
	return rand.String(size)
}
