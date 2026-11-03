package server

import (
	"slices"
	"time"

	"github.com/rancher/steve/pkg/sqlcache/informer/factory"
	"github.com/rancher/steve/pkg/stores/sqlproxy"
)

const (
	// How often the events table of every cached resource type is trimmed.
	sqlCacheGCInterval = 15 * time.Minute
	// How many events are kept per resource type when the events table is trimmed.
	sqlCacheGCKeepCount = 1000

	// Key of sqlproxy.TypeSpecificIndexedFields, in the form <group>_<version>_<kind>.
	vmSQLCacheType = "kubevirt.io_v1_VirtualMachine"

	// Annotation set by the UI, it holds the name that is shown instead of metadata.name.
	vmDisplayNameAnnotation = "harvesterhci.io/vmDisplayName"
)

var sqlCacheFactoryOptions = factory.CacheFactoryOptions{
	GCInterval:  sqlCacheGCInterval,
	GCKeepCount: sqlCacheGCKeepCount,
}

// sqlCacheIndexedFields are the fields, on top of the ones Steve indexes by default, that can be
// used to filter and sort the list of a resource type. A request that filters or sorts by a field
// that is not indexed is rejected with HTTP 422.
//
// Only plain strings can be indexed. Values that have to be computed, like the CPU or memory of a
// VM, cannot be indexed.
var sqlCacheIndexedFields = map[string][][]string{
	vmSQLCacheType: {
		{"metadata", "annotations", vmDisplayNameAnnotation},
		{"status", "printableStatus"},
	},
}

// registerSQLCacheIndexedFields hands the fields to Steve. It has to be called before the Steve
// server is created, because the SQL tables are built when a resource type is first requested.
func registerSQLCacheIndexedFields() {
	for cacheType, fields := range sqlCacheIndexedFields {
		registered := sqlproxy.TypeSpecificIndexedFields[cacheType]
		for _, field := range fields {
			if slices.ContainsFunc(registered, func(r []string) bool { return slices.Equal(r, field) }) {
				continue
			}
			registered = append(registered, field)
		}
		sqlproxy.TypeSpecificIndexedFields[cacheType] = registered
	}
}
