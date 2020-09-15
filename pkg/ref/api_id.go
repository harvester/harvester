package ref

import (
	"fmt"
	"strings"
)

// Parse parses the steve api ID.
func Parse(ref string) (namespace string, name string) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

// Construct creates the steve api ID.
func Construct(namespace string, name string) string {
	if namespace == "" {
		return name
	}
	return fmt.Sprintf("%s/%s", namespace, name)
}
