package csi

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	longhornclient "github.com/longhorn/longhorn-manager/client"
)

type Manager struct {
	ids *IdentityServer
	ns  *NodeServer
	cs  *ControllerServer
}

func init() {}

func GetCSIManager() *Manager {
	return &Manager{}
}

func (m *Manager) Run(driverName, nodeID, endpoint, identityVersion, managerURL string) error {
	logrus.Infof("CSI Driver: %v version: %v, manager URL %v", driverName, identityVersion, managerURL)

	// Longhorn API Client
	clientOpts := &longhornclient.ClientOpts{Url: managerURL}
	apiClient, err := longhornclient.NewRancherClient(clientOpts)
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Longhorn API client")
	}

	// Create GRPC servers
	m.ids = NewIdentityServer(driverName, identityVersion)
	m.ns = NewNodeServer(apiClient, nodeID)
	m.cs = NewControllerServer(apiClient, nodeID)
	s := NewNonBlockingGRPCServer()
	s.Start(endpoint, m.ids, m.cs, m.ns)
	s.Wait()

	return nil
}
