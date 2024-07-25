package sqlpartition

import (
	"fmt"
	"sort"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/lasso/pkg/cache/sql/partition"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/wrangler/v3/pkg/kv"
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
			ns, name := kv.RSplit(id, "/")
			return []partition.Partition{
				{
					Namespace:   ns,
					All:         false,
					Passthrough: false,
					Names:       sets.New[string](name),
				},
			}, nil
		}
		partitions, passthrough := isPassthrough(apiOp, schema, verb)
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

// isPassthrough determines whether a request can be passed through directly to the underlying store
// or if the results need to be partitioned by namespace and name based on the requester's access.
func isPassthrough(apiOp *types.APIRequest, schema *types.APISchema, verb string) ([]partition.Partition, bool) {
	accessListByVerb, _ := attributes.Access(schema).(accesscontrol.AccessListByVerb)
	if accessListByVerb.All(verb) {
		return nil, true
	}

	resources := accessListByVerb.Granted(verb)
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
