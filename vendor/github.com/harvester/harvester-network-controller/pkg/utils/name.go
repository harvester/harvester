package utils

import (
	"fmt"
	"hash/crc32"
	"strings"
)

const maxLengthOfName = 63

// Name function joints prefix with all other strings and crc32 checksum
func Name(prefix string, s ...string) string {
	name := prefix + strings.Join(s, "-")
	digest := crc32.ChecksumIEEE([]byte(name))
	suffix := fmt.Sprintf("%08x", digest)
	// The name contains no more than 63 characters
	maxLength := maxLengthOfName - 1 - len(suffix)
	if len(name) > maxLength {
		name = name[:maxLength]
	}

	return name + "-" + suffix
}
