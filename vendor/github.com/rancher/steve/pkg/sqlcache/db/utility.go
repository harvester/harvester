package db

import "strings"

// Sanitize returns a string  that can be used in SQL as a name
func Sanitize(s string) string {
	return strings.ReplaceAll(s, "\"", "")
}
