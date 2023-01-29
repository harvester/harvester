package metrics

import (
	"time"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/metrics"
)

type Store struct {
	Store types.Store
}

func NewMetricsStore(store types.Store) *Store {
	return &Store{
		Store: store,
	}
}

func (s *Store) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	storeStart := time.Now()
	apiObject, err := s.Store.ByID(apiOp, schema, id)
	m.RecordProxyStoreResponseTime(err, float64(time.Since(storeStart).Milliseconds()))
	return apiObject, err
}

func (s *Store) List(apiOp *types.APIRequest, schema *types.APISchema) (types.APIObjectList, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	storeStart := time.Now()
	apiObjectList, err := s.Store.List(apiOp, schema)
	m.RecordProxyStoreResponseTime(err, float64(time.Since(storeStart).Milliseconds()))
	return apiObjectList, err
}

func (s *Store) Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	storeStart := time.Now()
	apiObject, err := s.Store.Create(apiOp, schema, data)
	m.RecordProxyStoreResponseTime(err, float64(time.Since(storeStart).Milliseconds()))
	return apiObject, err
}

func (s *Store) Update(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (types.APIObject, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	storeStart := time.Now()
	apiObject, err := s.Store.Update(apiOp, schema, data, id)
	m.RecordProxyStoreResponseTime(err, float64(time.Since(storeStart).Milliseconds()))
	return apiObject, err
}

func (s *Store) Delete(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	storeStart := time.Now()
	apiObject, err := s.Store.Delete(apiOp, schema, id)
	m.RecordProxyStoreResponseTime(err, float64(time.Since(storeStart).Milliseconds()))
	return apiObject, err
}

func (s *Store) Watch(apiOp *types.APIRequest, schema *types.APISchema, w types.WatchRequest) (chan types.APIEvent, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	storeStart := time.Now()
	apiEvent, err := s.Store.Watch(apiOp, schema, w)
	m.RecordProxyStoreResponseTime(err, float64(time.Since(storeStart).Milliseconds()))
	return apiEvent, err
}