package engineapi

import (
	"fmt"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"

	smclient "github.com/longhorn/longhorn-share-manager/pkg/client"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type ShareManagerClient struct {
	grpcClient *smclient.ShareManagerClient
}

func NewShareManagerClient(sm *longhorn.ShareManager, pod *corev1.Pod) (*ShareManagerClient, error) {
	client, err := smclient.NewShareManagerClient(fmt.Sprintf("%s:%d", pod.Status.PodIP, ShareManagerDefaultPort))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create Share Manager client for %v", sm.Name)
	}

	return &ShareManagerClient{
		grpcClient: client,
	}, nil
}

func (c *ShareManagerClient) Close() error {
	if c.grpcClient == nil {
		return nil
	}

	return c.grpcClient.Close()
}

func (c *ShareManagerClient) FilesystemTrim(encryptedDevice bool) error {
	return c.grpcClient.FilesystemTrim(encryptedDevice)
}

func (c *ShareManagerClient) FilesystemResize() error {
	return c.grpcClient.FilesystemResize()
}

func (c *ShareManagerClient) Unmount() error {
	return c.grpcClient.Unmount()
}

func (c *ShareManagerClient) Mount() error {
	return c.grpcClient.Mount()
}
