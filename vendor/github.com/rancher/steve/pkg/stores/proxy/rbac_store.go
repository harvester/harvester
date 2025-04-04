package proxy

import (
	"fmt"
	"sort"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/steve/pkg/stores/partition"
	"github.com/rancher/wrangler/v3/pkg/kv"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
)

var (
	passthroughPartitions = []partition.Partition{
		Partition{Passthrough: true},
	}
)

// Partition is an implementation of the partition.Partition interface that uses RBAC to determine how a set of resources should be segregated and accessed.
type Partition struct {
	Namespace   string
	All         bool
	Passthrough bool
	Names       sets.String
}

// Name returns the name of the partition, which for this type is the namespace.
func (p Partition) Name() string {
	return p.Namespace
}

// rbacPartitioner is an implementation of the partition.Partioner interface.
type rbacPartitioner struct {
	proxyStore *Store
}

// Lookup returns the default passthrough partition which is used only for retrieving single resources.
// Listing or watching resources require custom partitions.
func (p *rbacPartitioner) Lookup(apiOp *types.APIRequest, schema *types.APISchema, verb, id string) (partition.Partition, error) {
	switch verb {
	case "create":
		fallthrough
	case "get":
		fallthrough
	case "update":
		fallthrough
	case "delete":
		return passthroughPartitions[0], nil
	default:
		return nil, fmt.Errorf("partition list: invalid verb %s", verb)
	}
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
			return partitions[i].(Partition).Namespace < partitions[j].(Partition).Namespace
		})
		return partitions, nil
	default:
		return nil, fmt.Errorf("parition all: invalid verb %s", verb)
	}
}

// Store returns an UnstructuredStore suited to listing and watching resources by partition.
func (p *rbacPartitioner) Store(apiOp *types.APIRequest, partition partition.Partition) (partition.UnstructuredStore, error) {
	return &byNameOrNamespaceStore{
		Store:     p.proxyStore,
		partition: partition.(Partition),
	}, nil
}

type byNameOrNamespaceStore struct {
	*Store
	partition Partition
}

// List returns a list of resources by partition.
func (b *byNameOrNamespaceStore) List(apiOp *types.APIRequest, schema *types.APISchema) (*unstructured.UnstructuredList, []types.Warning, error) {
	if b.partition.Passthrough {
		return b.Store.List(apiOp, schema)
	}

	apiOp.Namespace = b.partition.Namespace
	if b.partition.All {
		return b.Store.List(apiOp, schema)
	}
	return b.Store.ByNames(apiOp, schema, b.partition.Names)
}

// Watch returns a channel of resources by partition.
func (b *byNameOrNamespaceStore) Watch(apiOp *types.APIRequest, schema *types.APISchema, wr types.WatchRequest) (chan watch.Event, error) {
	if b.partition.Passthrough {
		return b.Store.Watch(apiOp, schema, wr)
	}

	apiOp.Namespace = b.partition.Namespace
	if b.partition.All {
		return b.Store.Watch(apiOp, schema, wr)
	}
	return b.Store.WatchNames(apiOp, schema, wr, b.partition.Names)
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
			Partition{
				Namespace:   effectiveNamespace,
				All:         false,
				Passthrough: false,
				Names:       sets.NewString(name),
			},
		}
	}

	if effectiveNamespace != "" {
		if resources[effectiveNamespace].All {
			return []partition.Partition{
				Partition{
					Namespace:   effectiveNamespace,
					All:         false,
					Passthrough: false,
					Names:       sets.NewString(name),
				},
			}
		}
	}

	// For cluster-scoped resources, we will have parsed a "" out
	// of the ID field from RSplit, but accessListByVerb specifies "*" for
	// the namespace, so correct that here
	resourceNamespace := effectiveNamespace
	if resourceNamespace == "" {
		resourceNamespace = accesscontrol.All
	}

	nameset, ok := resources[resourceNamespace]
	if ok && nameset.Names.Has(name) {
		return []partition.Partition{
			Partition{
				Namespace:   effectiveNamespace,
				All:         false,
				Passthrough: false,
				Names:       sets.NewString(name),
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
			Partition{
				Namespace: apiOp.Namespace,
				Names:     resources[apiOp.Namespace].Names,
			},
		}, false
	}

	var result []partition.Partition

	if attributes.Namespaced(schema) {
		for k, v := range resources {
			result = append(result, Partition{
				Namespace: k,
				All:       v.All,
				Names:     v.Names,
			})
		}
	} else {
		for _, v := range resources {
			result = append(result, Partition{
				All:   v.All,
				Names: v.Names,
			})
		}
	}

	return result, false
}
