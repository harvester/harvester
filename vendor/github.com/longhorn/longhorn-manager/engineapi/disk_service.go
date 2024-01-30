package engineapi

import (
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
		err = errors.Errorf("%v instance manager is in %v, not running state", im.Name, im.Status.CurrentState)
		return nil, err
	}

	hasIP := im.Status.IP != ""
	if !hasIP {
		err = errors.Errorf("%v instance manager status IP is missing", im.Name)
		return nil, err
	}

	endpoint := "tcp://" + imutil.GetURL(im.Status.IP, InstanceManagerDiskServiceDefaultPort)
	client, err := imclient.NewDiskServiceClient(endpoint, nil)
	if err != nil {
		return nil, err
	}

	return &DiskService{
		logger:     logger,
		grpcClient: client,
	}, nil
}

type DiskService struct {
	logger     logrus.FieldLogger
	grpcClient *imclient.DiskServiceClient
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

func (s *DiskService) DiskCreate(diskType, diskName, diskUUID, diskPath string, blockSize int64) (*imapi.DiskInfo, error) {
	return s.grpcClient.DiskCreate(diskType, diskName, diskUUID, diskPath, blockSize)
}

func (s *DiskService) DiskGet(diskType, diskName, diskPath string) (*imapi.DiskInfo, error) {
	return s.grpcClient.DiskGet(diskType, diskName, diskPath)
}

func (s *DiskService) DiskDelete(diskType, diskName, diskUUID string) error {
	return s.grpcClient.DiskDelete(diskType, diskName, diskUUID)
}

func (s *DiskService) DiskReplicaInstanceList(diskType, diskName string) (map[string]*imapi.ReplicaStorageInstance, error) {
	return s.grpcClient.DiskReplicaInstanceList(diskType, diskName)
}

func (s *DiskService) DiskReplicaInstanceDelete(diskType, diskName, diskUUID, replciaInstanceName string) error {
	return s.grpcClient.DiskReplicaInstanceDelete(diskType, diskName, diskUUID, replciaInstanceName)
}
