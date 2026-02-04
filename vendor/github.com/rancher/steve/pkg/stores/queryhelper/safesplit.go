package queryhelper

import "strings"

// SafeSplit breaks a regular "a.b.c" path to the string slice ["a", "b", "c"]
// but if the final part is in square brackets, like "metadata.labels[cattleprod.io/moo]",
// it returns ["metadata", "labels", "cattleprod.io/moo"]
func SafeSplit(fieldPath string) []string {
	squareBracketLocation := strings.Index(fieldPath, "[")
	if squareBracketLocation == -1 {
		return strings.Split(fieldPath, ".")
	}
	s := strings.Split(fieldPath[0:squareBracketLocation], ".")
	s = append(s, fieldPath[squareBracketLocation+1:len(fieldPath)-1])
	return s
}
