package engineapi

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	imapi "github.com/longhorn/longhorn-instance-manager/pkg/api"
	imclient "github.com/longhorn/longhorn-instance-manager/pkg/client"
	imutil "github.com/longhorn/longhorn-instance-manager/pkg/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func NewDiskServiceClient(im *longhorn.InstanceManager, logger logrus.FieldLogger) (c *DiskService, err error) {
	defer func() {
		err = errors.Wrap(err, "failed to get disk service client")
	}()

	isInstanceManagerRunning := im.Status.CurrentState == longhorn.InstanceManagerStateRunning
	if !isInstanceManagerRunning {
		err = errors.Errorf("%v instance manager is in %v state, not running state", im.Name, im.Status.CurrentState)
		return nil, err
	}

	hasIP := im.Status.IP != ""
	if !hasIP {
		err = errors.Errorf("%v instance manager status IP is missing", im.Name)
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	endpoint := "tcp://" + imutil.GetURL(im.Status.IP, InstanceManagerDiskServiceDefaultPort)
	client, err := imclient.NewDiskServiceClient(ctx, cancel, endpoint, nil)
	if err != nil {
		return nil, err
	}

	return &DiskService{
		logger:              logger,
		grpcClient:          client,
		instanceManagerName: im.Name,
	}, nil
}

type DiskService struct {
	logger              logrus.FieldLogger
	grpcClient          *imclient.DiskServiceClient
	instanceManagerName string
}

func (s *DiskService) Close() {
	if s.grpcClient == nil {
		s.logger.WithError(errors.New("gRPC client not exist")).Warn("Failed to close disk service client")
		return
	}

	if err := s.grpcClient.Close(); err != nil {
		s.logger.WithError(err).Warn("Failed to close disk service client")
	}
}

func (s *DiskService) DiskCreate(diskType, diskName, diskUUID, diskPath, diskDriver string, blockSize int64) (*imapi.DiskInfo, error) {
	return s.grpcClient.DiskCreate(diskType, diskName, diskUUID, diskPath, diskDriver, blockSize)
}

func (s *DiskService) DiskGet(diskType, diskName, diskPath, diskDriver string) (*imapi.DiskInfo, error) {
	return s.grpcClient.DiskGet(diskType, diskName, diskPath, diskDriver)
}

func (s *DiskService) DiskDelete(diskType, diskName, diskUUID, diskPath, diskDriver string) error {
	return s.grpcClient.DiskDelete(diskType, diskName, diskUUID, diskPath, diskDriver)
}

func (s *DiskService) DiskReplicaInstanceList(diskType, diskName, diskDriver string) (map[string]*imapi.ReplicaStorageInstance, error) {
	return s.grpcClient.DiskReplicaInstanceList(diskType, diskName, diskDriver)
}

func (s *DiskService) DiskReplicaInstanceDelete(diskType, diskName, diskUUID, diskDriver, replciaInstanceName string) error {
	return s.grpcClient.DiskReplicaInstanceDelete(diskType, diskName, diskUUID, diskDriver, replciaInstanceName)
}

func (s *DiskService) GetInstanceManagerName() string {
	return s.instanceManagerName
}
