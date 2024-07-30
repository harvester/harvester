// Package sqlpartition implements a store which converts a request to partitions based on the user's rbac for
// the resource. For example, a user may request all items of resource A, but only have permissions for resource A in
// namespaces x,y,z. The partitions will then store that information and be passed to the next store.
package sqlpartition

import (
	"context"

	"github.com/rancher/apiserver/pkg/types"
	lassopartition "github.com/rancher/lasso/pkg/cache/sql/partition"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/stores/partition"
)

// Partitioner is an interface for interacting with partitions.
type Partitioner interface {
	All(apiOp *types.APIRequest, schema *types.APISchema, verb, id string) ([]lassopartition.Partition, error)
	Store() UnstructuredStore
}

type SchemaColumnSetter interface {
	SetColumns(ctx context.Context, schema *types.APISchema) error
}

// Store implements types.proxyStore for partitions.
type Store struct {
	Partitioner Partitioner
	asl         accesscontrol.AccessSetLookup
}

// NewStore creates a types.proxyStore implementation with a partitioner
func NewStore(store UnstructuredStore, asl accesscontrol.AccessSetLookup) *Store {
	s := &Store{
		Partitioner: &rbacPartitioner{
			proxyStore: store,
		},
		asl: asl,
	}

	return s
}

// Delete deletes an object from a store.
func (s *Store) Delete(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	target := s.Partitioner.Store()

	obj, warnings, err := target.Delete(apiOp, schema, id)
	if err != nil {
		return types.APIObject{}, err
	}
	return partition.ToAPI(schema, obj, warnings), nil
}

// ByID looks up a single object by its ID.
func (s *Store) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	target := s.Partitioner.Store()

	obj, warnings, err := target.ByID(apiOp, schema, id)
	if err != nil {
		return types.APIObject{}, err
	}
	return partition.ToAPI(schema, obj, warnings), nil
}

// List returns a list of objects across all applicable partitions.
// If pagination parameters are used, it returns a segment of the list.
func (s *Store) List(apiOp *types.APIRequest, schema *types.APISchema) (types.APIObjectList, error) {
	var (
		result types.APIObjectList
	)

	partitions, err := s.Partitioner.All(apiOp, schema, "list", "")
	if err != nil {
		return result, err
	}

	store := s.Partitioner.Store()

	list, total, continueToken, err := store.ListByPartitions(apiOp, schema, partitions)
	if err != nil {
		return result, err
	}

	result.Count = total

	for _, item := range list {
		item := item.DeepCopy()
		result.Objects = append(result.Objects, partition.ToAPI(schema, item, nil))
	}

	result.Revision = ""
	result.Continue = continueToken
	return result, nil
}

// Create creates a single object in the store.
func (s *Store) Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	target := s.Partitioner.Store()

	obj, warnings, err := target.Create(apiOp, schema, data)
	if err != nil {
		return types.APIObject{}, err
	}
	return partition.ToAPI(schema, obj, warnings), nil
}

// Update updates a single object in the store.
func (s *Store) Update(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (types.APIObject, error) {
	target := s.Partitioner.Store()

	obj, warnings, err := target.Update(apiOp, schema, data, id)
	if err != nil {
		return types.APIObject{}, err
	}
	return partition.ToAPI(schema, obj, warnings), nil
}

// Watch returns a channel of events for a list or resource.
func (s *Store) Watch(apiOp *types.APIRequest, schema *types.APISchema, wr types.WatchRequest) (chan types.APIEvent, error) {
	partitions, err := s.Partitioner.All(apiOp, schema, "watch", wr.ID)
	if err != nil {
		return nil, err
	}

	store := s.Partitioner.Store()

	response := make(chan types.APIEvent)
	c, err := store.WatchByPartitions(apiOp, schema, wr, partitions)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(response)

		for i := range c {
			response <- partition.ToAPIEvent(nil, schema, i)
		}
	}()

	return response, nil
}
