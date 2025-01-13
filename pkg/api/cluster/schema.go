package cluster

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/store/empty"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/server"

	"github.com/harvester/harvester/pkg/config"
)

const (
	deviceCapacity   = "deviceCapacity"
	localClusterName = "local"
	machineTypes     = "machineTypes"
)

type Cluster struct {
	Name string
}

// RegisterSchema returns a fake cluster schema, based on examples in
// github.com/rancher/apiserver which serves as a store to expose a cluster object
// this cluster object is not backed by an actual CRD and is only needed to
// satisfy ability to define actions or linkHandlers
// The cluster link handlers allow querying details of device allocation
// from all nodes in the cluster
func RegisterSchema(scaled *config.Scaled, server *server.Server, _ config.Options) error {
	handler := Handler{
		nodeCache: scaled.CoreFactory.Core().V1().Node().Cache(),
		vmCache:   scaled.VirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
	}

	fakeClusterStore := &Store{
		&empty.Store{},
	}
	server.BaseSchemas.MustImportAndCustomize(Cluster{}, func(schema *types.APISchema) {
		schema.Store = fakeClusterStore
		schema.ResourceMethods = []string{"GET"}
		schema.CollectionMethods = []string{"GET"}
		schema.LinkHandlers = map[string]http.Handler{
			deviceCapacity: handler,
			machineTypes:   handler,
		}
	})
	return nil
}
