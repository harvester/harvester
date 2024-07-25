package name

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// GuessPluralName attempts to pluralize a noun.
func GuessPluralName(name string) string {
	if name == "" {
		return name
	}

	if strings.EqualFold(name, "Endpoints") {
		return name
	}

	if suffix(name, "s") || suffix(name, "ch") || suffix(name, "x") || suffix(name, "sh") {
		return name + "es"
	}

	if suffix(name, "f") || suffix(name, "fe") {
		return name + "ves"
	}

	if suffix(name, "y") && len(name) > 2 && !strings.ContainsAny(name[len(name)-2:len(name)-1], "[aeiou]") {
		return name[0:len(name)-1] + "ies"
	}

	return name + "s"
}

func suffix(str, end string) bool {
	return strings.HasSuffix(str, end)
}

// Limit the length of a string to count characters. If the string's length is
// greater or equal to count, it will be truncated and a hash will be appended
// to the end.
// Warning: runtime error for count <= 5: https://go.dev/play/p/UAbpZIOvIYo
func Limit(s string, count int) string {
	if len(s) < count {
		return s
	}
	return fmt.Sprintf("%s-%s", s[:count-6], Hex(s, 5))
}

// Hex gets the checksum of s, encodes it to hexadecimal and returns the first n characters of that hexadecimal.
// Warning: runtime error for  n > 32 or n < 0.
func Hex(s string, n int) string {
	h := md5.Sum([]byte(s))
	d := hex.EncodeToString(h[:])
	return d[:n]
}

// SafeConcatName concatenates the given strings and ensures the returned name is under 64 characters
// by cutting the string off at 57 characters and setting the last 6 with an encoded version of the concatenated string.
func SafeConcatName(name ...string) string {
	fullPath := strings.Join(name, "-")
	if len(fullPath) < 64 {
		return fullPath
	}
	digest := sha256.Sum256([]byte(fullPath))
	// since we cut the string in the middle, the last char may not be compatible with what is expected in k8s
	// we are checking and if necessary removing the last char
	c := fullPath[56]
	if 'a' <= c && c <= 'z' || '0' <= c && c <= '9' {
		return fullPath[0:57] + "-" + hex.EncodeToString(digest[0:])[0:5]
	}

	return fullPath[0:56] + "-" + hex.EncodeToString(digest[0:])[0:6]
}
