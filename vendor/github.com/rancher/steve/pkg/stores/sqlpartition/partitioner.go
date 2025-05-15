package sqlpartition

import (
	"fmt"
	"sort"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/steve/pkg/sqlcache/partition"
	"github.com/rancher/wrangler/v3/pkg/kv"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
)

var (
	passthroughPartitions = []partition.Partition{
		{Passthrough: true},
	}
)

// UnstructuredStore is like types.Store but deals in k8s unstructured objects instead of apiserver types.
// This interface exists in order for store to be mocked in tests
type UnstructuredStore interface {
	ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (*unstructured.Unstructured, []types.Warning, error)
	Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (*unstructured.Unstructured, []types.Warning, error)
	Update(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (*unstructured.Unstructured, []types.Warning, error)
	Delete(apiOp *types.APIRequest, schema *types.APISchema, id string) (*unstructured.Unstructured, []types.Warning, error)

	ListByPartitions(apiOp *types.APIRequest, schema *types.APISchema, partitions []partition.Partition) ([]unstructured.Unstructured, int, string, error)
	WatchByPartitions(apiOp *types.APIRequest, schema *types.APISchema, wr types.WatchRequest, partitions []partition.Partition) (chan watch.Event, error)
}

// rbacPartitioner is an implementation of the sqlpartition.Partitioner interface.
type rbacPartitioner struct {
	proxyStore UnstructuredStore
}

// All returns a slice of partitions applicable to the API schema and the user's access level.
// For watching individual resources or for blanket access permissions, it returns the passthrough partition.
// For more granular permissions, it returns a slice of partitions matching an allowed namespace or resource names.
func (p *rbacPartitioner) All(apiOp *types.APIRequest, schema *types.APISchema, verb, id string) ([]partition.Partition, error) {
	switch verb {
	case "list":
		fallthrough
	case "watch":
		if id != "" {
			partitions := generatePartitionsByID(apiOp, schema, verb, id)
			return partitions, nil
		}
		partitions, passthrough := generateAggregatePartitions(apiOp, schema, verb)
		if passthrough {
			return passthroughPartitions, nil
		}
		sort.Slice(partitions, func(i, j int) bool {
			return partitions[i].Namespace < partitions[j].Namespace
		})
		return partitions, nil
	default:
		return nil, fmt.Errorf("parition all: invalid verb %s", verb)
	}
}

// Store returns an Store suited to listing and watching resources by partition.
func (p *rbacPartitioner) Store() UnstructuredStore {
	return p.proxyStore
}

// generatePartitionsById determines whether a requester can access a particular resource
// and if so, returns the corresponding partitions
func generatePartitionsByID(apiOp *types.APIRequest, schema *types.APISchema, verb string, id string) []partition.Partition {
	accessListByVerb, _ := attributes.Access(schema).(accesscontrol.AccessListByVerb)
	resources := accessListByVerb.Granted(verb)

	idNamespace, name := kv.RSplit(id, "/")
	apiNamespace := apiOp.Namespace
	effectiveNamespace := idNamespace

	// If a non-empty namespace was provided, be sure to select that for filtering and permissions checks
	if idNamespace == "" && apiNamespace != "" {
		effectiveNamespace = apiNamespace
	}

	// The external API is flexible, and permits specifying a namespace as a separate key or embedded
	// within the ID of the object. Both of these cases should be valid:
	//   {"namespace": "n1", "id": "r1"}
	//   {"id": "n1/r1"}
	// however, the following conflicting request is not valid, but was previously accepted:
	//   {"namespace": "n1", "id": "n2/r1"}
	// To avoid breaking UI plugins that may inadvertently rely on the feature, we issue a deprecation
	// warning for now. We still need to pick one of the namespaces for permission verification purposes.
	if idNamespace != "" && apiNamespace != "" && idNamespace != apiNamespace {
		logrus.Warningf("DEPRECATION: Conflicting namespaces '%v' and '%v' requested. "+
			"Selecting '%v' as the effective namespace. Future steve versions will reject this request.",
			idNamespace, apiNamespace, effectiveNamespace)
	}

	if accessListByVerb.All(verb) {
		return []partition.Partition{
			{
				Namespace:   effectiveNamespace,
				All:         false,
				Passthrough: false,
				Names:       sets.New(name),
			},
		}
	}

	if effectiveNamespace != "" {
		if resources[effectiveNamespace].All {
			return []partition.Partition{
				{
					Namespace:   effectiveNamespace,
					All:         false,
					Passthrough: false,
					Names:       sets.New(name),
				},
			}
		}
	}

	// For cluster-scoped resources, we will have parsed a "" out
	// of the ID field from RSplit, but accessListByVerb specifies "*" for
	// the nameset, so correct that here
	resourceNamespace := effectiveNamespace
	if resourceNamespace == "" {
		resourceNamespace = accesscontrol.All
	}

	nameset, ok := resources[resourceNamespace]
	if ok && nameset.Names.Has(name) {
		return []partition.Partition{
			{
				Namespace:   effectiveNamespace,
				All:         false,
				Passthrough: false,
				Names:       sets.New(name),
			},
		}
	}

	return nil
}

// generateAggregatePartitions determines whether a request can be passed through directly to the underlying store
// or if the results need to be partitioned by namespace and name based on the requester's access.
func generateAggregatePartitions(apiOp *types.APIRequest, schema *types.APISchema, verb string) ([]partition.Partition, bool) {
	accessListByVerb, _ := attributes.Access(schema).(accesscontrol.AccessListByVerb)
	resources := accessListByVerb.Granted(verb)

	if accessListByVerb.All(verb) {
		return nil, true
	}

	if apiOp.Namespace != "" {
		if resources[apiOp.Namespace].All {
			return nil, true
		}
		return []partition.Partition{
			{
				Namespace: apiOp.Namespace,
				Names:     sets.Set[string](resources[apiOp.Namespace].Names),
			},
		}, false
	}

	var result []partition.Partition

	if attributes.Namespaced(schema) {
		for k, v := range resources {
			result = append(result, partition.Partition{
				Namespace: k,
				All:       v.All,
				Names:     sets.Set[string](v.Names),
			})
		}
	} else {
		for _, v := range resources {
			result = append(result, partition.Partition{
				All:   v.All,
				Names: sets.Set[string](v.Names),
			})
		}
	}

	return result, false
}
