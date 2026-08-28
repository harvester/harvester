package ref

import (
	"github.com/harvester/harvester/pkg/util"
)

// Parse parses the steve api ID.
// Deprecated: use util.SplitNamespacedName instead.
func Parse(ref string) (namespace string, name string) {
	namespace, name, _ = util.SplitNamespacedName(ref)
	return
}

// Construct creates the steve api ID.
// Deprecated: use util.NamespacedName instead.
func Construct(namespace string, name string) string {
	return util.NamespacedName(namespace, name)
}
