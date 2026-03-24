package client

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/longhorn/types/pkg/generated/enginerpc"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/longhorn/longhorn-engine/pkg/interceptor"
	"github.com/longhorn/longhorn-engine/pkg/types"
	"github.com/longhorn/longhorn-engine/pkg/util"
)

const (
	GRPCServiceCommonTimeout = 3 * time.Minute
	GRPCServiceLongTimeout   = 24 * time.Hour
)

type ReplicaServiceContext struct {
	cc      *grpc.ClientConn
	service enginerpc.ReplicaServiceClient
	once    util.Once
}

func (c *ReplicaServiceContext) Close() error {
	if c.cc == nil {
		return nil
	}
	return c.cc.Close()
}

type SyncServiceContext struct {
	cc      *grpc.ClientConn
	service enginerpc.SyncAgentServiceClient
	once    util.Once
}

func (c *SyncServiceContext) Close() error {
	if c.cc == nil {
		return nil
	}
	return c.cc.Close()
}

type ReplicaClient struct {
	host                string
	replicaServiceURL   string
	syncAgentServiceURL string
	volumeName          string
	instanceName        string

	replicaServiceContext ReplicaServiceContext
	syncServiceContext    SyncServiceContext
}

func (c *ReplicaClient) Close() error {
	_ = c.replicaServiceContext.Close()
	_ = c.syncServiceContext.Close()
	return nil
}

func NewReplicaClient(address, volumeName, instanceName string) (*ReplicaClient, error) {
	replicaServiceURL := util.GetGRPCAddress(address)
	host, strPort, err := net.SplitHostPort(replicaServiceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid replica address %s, must have a port in it", replicaServiceURL)
	}

	port, err := strconv.Atoi(strPort)
	if err != nil {
		return nil, err
	}
	syncAgentServiceURL := net.JoinHostPort(host, strconv.Itoa(port+2))

	return &ReplicaClient{
		host:                host,
		replicaServiceURL:   replicaServiceURL,
		syncAgentServiceURL: syncAgentServiceURL,
		volumeName:          volumeName,
		instanceName:        instanceName,
	}, nil
}

// getReplicaServiceClient lazily initialize the service client, this is to reduce the connection count
// for the longhorn-manager which executes these command as binaries invocations
func (c *ReplicaClient) getReplicaServiceClient() (enginerpc.ReplicaServiceClient, error) {
	err := c.replicaServiceContext.once.Do(func() error {
		cc, err := grpc.NewClient(c.replicaServiceURL, grpc.WithTransportCredentials(insecure.NewCredentials()),
			interceptor.WithIdentityValidationClientInterceptor(c.volumeName, c.instanceName))
		if err != nil {
			return err
		}

		// this is safe since we only do it one time while we have the lock in once.doSlow()
		c.replicaServiceContext.cc = cc
		c.replicaServiceContext.service = enginerpc.NewReplicaServiceClient(cc)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c.replicaServiceContext.service, nil
}

// getSyncServiceClient lazily initialize the service client, this is to reduce the connection count
// for the longhorn-manager which executes these command as binaries invocations
func (c *ReplicaClient) getSyncServiceClient() (enginerpc.SyncAgentServiceClient, error) {
	err := c.syncServiceContext.once.Do(func() error {
		cc, err := grpc.NewClient(c.syncAgentServiceURL, grpc.WithTransportCredentials(insecure.NewCredentials()),
			interceptor.WithIdentityValidationClientInterceptor(c.volumeName, c.instanceName))
		if err != nil {
			return err
		}

		// this is safe since we only do it one time while we have the lock in once.doSlow()
		c.syncServiceContext.cc = cc
		c.syncServiceContext.service = enginerpc.NewSyncAgentServiceClient(cc)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c.syncServiceContext.service, nil
}

func GetDiskInfo(info *enginerpc.DiskInfo) *types.DiskInfo {
	diskInfo := &types.DiskInfo{
		Name:        info.Name,
		Parent:      info.Parent,
		Children:    info.Children,
		Removed:     info.Removed,
		UserCreated: info.UserCreated,
		Created:     info.Created,
		Size:        info.Size,
		Labels:      info.Labels,
	}

	if diskInfo.Labels == nil {
		diskInfo.Labels = map[string]string{}
	}

	return diskInfo
}

func GetReplicaInfo(r *enginerpc.Replica) *types.ReplicaInfo {
	replicaInfo := &types.ReplicaInfo{
		Dirty:                     r.Dirty,
		Rebuilding:                r.Rebuilding,
		Head:                      r.Head,
		Parent:                    r.Parent,
		Size:                      r.Size,
		SectorSize:                r.SectorSize,
		BackingFile:               r.BackingFile,
		State:                     r.State,
		Chain:                     r.Chain,
		Disks:                     map[string]types.DiskInfo{},
		RemainSnapshots:           int(r.RemainSnapshots),
		RevisionCounter:           r.RevisionCounter,
		LastModifyTime:            r.LastModifyTime,
		HeadFileSize:              r.HeadFileSize,
		RevisionCounterDisabled:   r.RevisionCounterDisabled,
		UnmapMarkDiskChainRemoved: r.UnmapMarkDiskChainRemoved,
		SnapshotCountUsage:        int(r.SnapshotCountUsage),
		SnapshotCountTotal:        int(r.SnapshotCountTotal),
		SnapshotSizeUsage:         r.SnapshotSizeUsage,
	}

	for diskName, diskInfo := range r.Disks {
		replicaInfo.Disks[diskName] = *GetDiskInfo(diskInfo)
	}

	return replicaInfo
}

func syncFileInfoListToSyncAgentGRPCFormat(list []types.SyncFileInfo) []*enginerpc.SyncFileInfo {
	res := []*enginerpc.SyncFileInfo{}
	for _, info := range list {
		res = append(res, syncFileInfoToSyncAgentGRPCFormat(info))
	}
	return res
}

func syncFileInfoToSyncAgentGRPCFormat(info types.SyncFileInfo) *enginerpc.SyncFileInfo {
	return &enginerpc.SyncFileInfo{
		FromFileName: info.FromFileName,
		ToFileName:   info.ToFileName,
		ActualSize:   info.ActualSize,
	}
}

func (c *ReplicaClient) GetReplica() (*types.ReplicaInfo, error) {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	resp, err := replicaServiceClient.ReplicaGet(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get replica %v", c.replicaServiceURL)
	}

	return GetReplicaInfo(resp.Replica), nil
}

func (c *ReplicaClient) OpenReplica() error {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := replicaServiceClient.ReplicaOpen(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrapf(err, "failed to open replica %v", c.replicaServiceURL)
	}

	return nil
}

func (c *ReplicaClient) CloseReplica() error {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := replicaServiceClient.ReplicaClose(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrapf(err, "failed to close replica %v", c.replicaServiceURL)
	}

	return nil
}

func (c *ReplicaClient) ReloadReplica() (*types.ReplicaInfo, error) {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	resp, err := replicaServiceClient.ReplicaReload(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to reload replica %v", c.replicaServiceURL)
	}

	return GetReplicaInfo(resp.Replica), nil
}

func (c *ReplicaClient) ExpandReplica(size int64) (*types.ReplicaInfo, error) {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	resp, err := replicaServiceClient.ReplicaExpand(ctx, &enginerpc.ReplicaExpandRequest{
		Size: size,
	})
	if err != nil {
		return nil, types.WrapError(types.UnmarshalGRPCError(err), "failed to expand replica %v", c.replicaServiceURL)
	}

	return GetReplicaInfo(resp.Replica), nil
}

func (c *ReplicaClient) Revert(name, created string) error {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := replicaServiceClient.ReplicaRevert(ctx, &enginerpc.ReplicaRevertRequest{
		Name:    name,
		Created: created,
	}); err != nil {
		return errors.Wrapf(err, "failed to revert replica %v", c.replicaServiceURL)
	}

	return nil
}

func (c *ReplicaClient) RemoveDisk(disk string, force bool) error {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := replicaServiceClient.DiskRemove(ctx, &enginerpc.DiskRemoveRequest{
		Name:  disk,
		Force: force,
	}); err != nil {
		return errors.Wrapf(err, "failed to remove disk %v for replica %v", disk, c.replicaServiceURL)
	}

	return nil
}

func (c *ReplicaClient) ReplaceDisk(target, source string) error {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := replicaServiceClient.DiskReplace(ctx, &enginerpc.DiskReplaceRequest{
		Target: target,
		Source: source,
	}); err != nil {
		return errors.Wrapf(err, "failed to replace disk %v with %v for replica %v", target, source, c.replicaServiceURL)
	}

	return nil
}

func (c *ReplicaClient) PrepareRemoveDisk(disk string) ([]*types.PrepareRemoveAction, error) {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	reply, err := replicaServiceClient.DiskPrepareRemove(ctx, &enginerpc.DiskPrepareRemoveRequest{
		Name: disk,
	})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare removing disk %v for replica %v", disk, c.replicaServiceURL)
	}

	operations := []*types.PrepareRemoveAction{}
	for _, op := range reply.Operations {
		operations = append(operations, &types.PrepareRemoveAction{
			Action: op.Action,
			Source: op.Source,
			Target: op.Target,
		})
	}

	return operations, nil
}

func (c *ReplicaClient) MarkDiskAsRemoved(disk string) error {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := replicaServiceClient.DiskMarkAsRemoved(ctx, &enginerpc.DiskMarkAsRemovedRequest{
		Name: disk,
	}); err != nil {
		return errors.Wrapf(err, "failed to mark disk %v as removed for replica %v", disk, c.replicaServiceURL)
	}

	return nil
}

func (c *ReplicaClient) SetRebuilding(rebuilding bool) error {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := replicaServiceClient.RebuildingSet(ctx, &enginerpc.RebuildingSetRequest{
		Rebuilding: rebuilding,
	}); err != nil {
		return errors.Wrapf(err, "failed to set rebuilding to %v for replica %v", rebuilding, c.replicaServiceURL)
	}

	return nil
}

func (c *ReplicaClient) SetUnmapMarkDiskChainRemoved(enabled bool) error {
	replicaServiceClient, err := c.getReplicaServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := replicaServiceClient.UnmapMarkDiskChainRemovedSet(ctx, &enginerpc.UnmapMarkDiskChainRemovedSetRequest{
		Enabled: enabled,
	}); err != nil {
		return errors.Wrapf(err, "failed to set UnmapMarkDiskChainRemoved flag to %v for replica %v", enabled, c.replicaServiceURL)
	}

	return nil
}

func (c *ReplicaClient) RemoveFile(file string) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.FileRemove(ctx, &enginerpc.FileRemoveRequest{
		FileName: file,
	}); err != nil {
		return errors.Wrapf(err, "failed to remove file %v", file)
	}

	return nil
}

func (c *ReplicaClient) RenameFile(oldFileName, newFileName string) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.FileRename(ctx, &enginerpc.FileRenameRequest{
		OldFileName: oldFileName,
		NewFileName: newFileName,
	}); err != nil {
		return errors.Wrapf(err, "failed to rename or replace old file %v with new file %v", oldFileName, newFileName)
	}

	return nil
}

func (c *ReplicaClient) SendFile(from, host string, port int32, fileSyncHTTPClientTimeout int, fastSync bool, grpcTimeoutSeconds int64) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	grpcTimeout := GRPCServiceLongTimeout
	if grpcTimeoutSeconds > 0 {
		grpcTimeout = time.Second * time.Duration(grpcTimeoutSeconds)
	}
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.FileSend(ctx, &enginerpc.FileSendRequest{
		FromFileName:              from,
		Host:                      host,
		Port:                      port,
		FastSync:                  fastSync,
		FileSyncHttpClientTimeout: int32(fileSyncHTTPClientTimeout),
	}); err != nil {
		return errors.Wrapf(err, "failed to send file %v to %v:%v", from, host, port)
	}

	return nil
}

func (c *ReplicaClient) ExportVolume(snapshotName, host string, port int32, exportBackingImageIfExist bool, fileSyncHTTPClientTimeout int) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceLongTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.VolumeExport(ctx, &enginerpc.VolumeExportRequest{
		SnapshotFileName:          snapshotName,
		Host:                      host,
		Port:                      port,
		ExportBackingImageIfExist: exportBackingImageIfExist,
		FileSyncHttpClientTimeout: int32(fileSyncHTTPClientTimeout),
	}); err != nil {
		return errors.Wrapf(err, "failed to export snapshot %v to %v:%v", snapshotName, host, port)
	}
	return nil
}

func (c *ReplicaClient) LaunchReceiver(toFilePath string) (string, int32, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return "", 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	reply, err := syncAgentServiceClient.ReceiverLaunch(ctx, &enginerpc.ReceiverLaunchRequest{
		ToFileName: toFilePath,
	})
	if err != nil {
		return "", 0, errors.Wrapf(err, "failed to launch receiver for %v", toFilePath)
	}

	return c.host, reply.Port, nil
}

func (c *ReplicaClient) SyncFiles(fromAddress string, list []types.SyncFileInfo, fileSyncHTTPClientTimeout int, fastSync bool, grpcTimeoutSeconds int64, localSync *types.FileLocalSync) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	grpcTimeout := GRPCServiceLongTimeout
	if grpcTimeoutSeconds > 0 {
		grpcTimeout = time.Second * time.Duration(grpcTimeoutSeconds)
	}
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	fileSyncRequest := &enginerpc.FilesSyncRequest{
		FromAddress:               fromAddress,
		ToHost:                    c.host,
		SyncFileInfoList:          syncFileInfoListToSyncAgentGRPCFormat(list),
		FastSync:                  fastSync,
		FileSyncHttpClientTimeout: int32(fileSyncHTTPClientTimeout),
		GrpcTimeoutSeconds:        grpcTimeoutSeconds,
	}

	if localSync != nil {
		fileSyncRequest.LocalSync = &enginerpc.FileLocalSync{
			SourcePath: localSync.SourcePath,
			TargetPath: localSync.TargetPath,
		}
	}

	if _, err := syncAgentServiceClient.FilesSync(ctx, fileSyncRequest); err != nil {
		return errors.Wrapf(err, "failed to sync files %+v from %v", list, fromAddress)
	}

	return nil
}

func (c *ReplicaClient) CreateBackup(backupName, snapshot, dest, volume, backingImageName, backingImageChecksum,
	compressionMethod string, concurrentLimit int, storageClassName string, labels []string, credential map[string]string, parameters map[string]string) (*enginerpc.BackupCreateResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	resp, err := syncAgentServiceClient.BackupCreate(ctx, &enginerpc.BackupCreateRequest{
		SnapshotFileName:     snapshot,
		BackupTarget:         dest,
		VolumeName:           volume,
		BackingImageName:     backingImageName,
		BackingImageChecksum: backingImageChecksum,
		CompressionMethod:    compressionMethod,
		ConcurrentLimit:      int32(concurrentLimit),
		StorageClassName:     storageClassName,
		Labels:               labels,
		Credential:           credential,
		BackupName:           backupName,
		Parameters:           parameters,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create backup to %v for volume %v", dest, volume)
	}

	return resp, nil
}

func (c *ReplicaClient) BackupStatus(backupName string) (*enginerpc.BackupStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	resp, err := syncAgentServiceClient.BackupStatus(ctx, &enginerpc.BackupStatusRequest{
		Backup: backupName,
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ReplicaClient) RmBackup(backup string) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.BackupRemove(ctx, &enginerpc.BackupRemoveRequest{
		Backup: backup,
	}); err != nil {
		return errors.Wrapf(err, "failed to remove backup %v", backup)
	}

	return nil
}

func (c *ReplicaClient) RestoreBackup(backup, snapshotDiskName string, credential map[string]string, concurrentLimit int) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.BackupRestore(ctx, &enginerpc.BackupRestoreRequest{
		Backup:           backup,
		SnapshotDiskName: snapshotDiskName,
		Credential:       credential,
		ConcurrentLimit:  int32(concurrentLimit),
	}); err != nil {
		return errors.Wrapf(err, "failed to restore backup data %v to snapshot file %v", backup, snapshotDiskName)
	}

	return nil
}

func (c *ReplicaClient) Reset() error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.Reset(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to clean up restore info in Sync Agent Server")
	}

	return nil
}

func (c *ReplicaClient) RestoreStatus() (*enginerpc.RestoreStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	resp, err := syncAgentServiceClient.RestoreStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get restore status")
	}

	return resp, nil
}

func (c *ReplicaClient) SnapshotPurge() error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.SnapshotPurge(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to start snapshot purge")
	}

	return nil
}

func (c *ReplicaClient) SnapshotPurgeStatus() (*enginerpc.SnapshotPurgeStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	status, err := syncAgentServiceClient.SnapshotPurgeStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get snapshot purge status")
	}

	return status, nil
}

func (c *ReplicaClient) ReplicaRebuildStatus() (*enginerpc.ReplicaRebuildStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	status, err := syncAgentServiceClient.ReplicaRebuildStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get replica rebuild status")
	}

	return status, nil
}

func (c *ReplicaClient) CloneSnapshot(fromAddress, fromVolumeName, snapshotFileName string, exportBackingImageIfExist bool, fileSyncHTTPClientTimeout int, grpcTimeoutSeconds int64) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	grpcTimeout := GRPCServiceLongTimeout
	if grpcTimeoutSeconds > 0 {
		grpcTimeout = time.Second * time.Duration(grpcTimeoutSeconds)
	}
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.SnapshotClone(ctx, &enginerpc.SnapshotCloneRequest{
		FromAddress:               fromAddress,
		ToHost:                    c.host,
		SnapshotFileName:          snapshotFileName,
		ExportBackingImageIfExist: exportBackingImageIfExist,
		FileSyncHttpClientTimeout: int32(fileSyncHTTPClientTimeout),
		FromVolumeName:            fromVolumeName,
	}); err != nil {
		return errors.Wrapf(err, "failed to clone snapshot %v from replica %v to host %v", snapshotFileName, fromAddress, c.host)
	}

	return nil
}

func (c *ReplicaClient) SnapshotCloneStatus() (*enginerpc.SnapshotCloneStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	status, err := syncAgentServiceClient.SnapshotCloneStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get snapshot clone status")
	}
	return status, nil
}

func (c *ReplicaClient) SnapshotHash(snapshotName string, rehash bool) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.SnapshotHash(ctx, &enginerpc.SnapshotHashRequest{
		SnapshotName: snapshotName,
		Rehash:       rehash,
	}); err != nil {
		return errors.Wrap(err, "failed to start hashing snapshot")
	}

	return nil
}

func (c *ReplicaClient) SnapshotHashStatus(snapshotName string) (*enginerpc.SnapshotHashStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	status, err := syncAgentServiceClient.SnapshotHashStatus(ctx, &enginerpc.SnapshotHashStatusRequest{
		SnapshotName: snapshotName,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get snapshot hash status")
	}
	return status, nil
}

func (c *ReplicaClient) SnapshotHashCancel(snapshotName string) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.SnapshotHashCancel(ctx, &enginerpc.SnapshotHashCancelRequest{
		SnapshotName: snapshotName,
	}); err != nil {
		return errors.Wrapf(err, "failed to cancel snapshot %v hash task", snapshotName)
	}

	return nil
}

func (c *ReplicaClient) SnapshotHashLockState() (bool, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	resp, err := syncAgentServiceClient.SnapshotHashLockState(ctx, &emptypb.Empty{})
	if err != nil {
		return false, errors.Wrapf(err, "failed to get snapshot hash lock state")
	}

	return resp.IsLocked, nil
}
