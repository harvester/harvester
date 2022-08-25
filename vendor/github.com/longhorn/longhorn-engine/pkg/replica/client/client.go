package client

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/longhorn/longhorn-engine/pkg/types"
	"github.com/longhorn/longhorn-engine/pkg/util"
	"github.com/longhorn/longhorn-engine/proto/ptypes"
)

const (
	GRPCServiceCommonTimeout = 3 * time.Minute
	GRPCServiceLongTimeout   = 24 * time.Hour
)

type ReplicaServiceContext struct {
	cc      *grpc.ClientConn
	service ptypes.ReplicaServiceClient
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
	service ptypes.SyncAgentServiceClient
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

	replicaServiceContext ReplicaServiceContext
	syncServiceContext    SyncServiceContext
}

func (c *ReplicaClient) Close() error {
	_ = c.replicaServiceContext.Close()
	_ = c.syncServiceContext.Close()
	return nil
}

func NewReplicaClient(address string) (*ReplicaClient, error) {
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
	}, nil
}

// getReplicaServiceClient lazily initialize the service client, this is to reduce the connection count
// for the longhorn-manager which executes these command as binaries invocations
func (c *ReplicaClient) getReplicaServiceClient() (ptypes.ReplicaServiceClient, error) {
	err := c.replicaServiceContext.once.Do(func() error {
		cc, err := grpc.Dial(c.replicaServiceURL, grpc.WithInsecure())
		if err != nil {
			return err
		}

		// this is safe since we only do it one time while we have the lock in once.doSlow()
		c.replicaServiceContext.cc = cc
		c.replicaServiceContext.service = ptypes.NewReplicaServiceClient(cc)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c.replicaServiceContext.service, nil
}

// getSyncServiceClient lazily initialize the service client, this is to reduce the connection count
// for the longhorn-manager which executes these command as binaries invocations
func (c *ReplicaClient) getSyncServiceClient() (ptypes.SyncAgentServiceClient, error) {
	err := c.syncServiceContext.once.Do(func() error {
		cc, err := grpc.Dial(c.syncAgentServiceURL, grpc.WithInsecure())
		if err != nil {
			return err
		}

		// this is safe since we only do it one time while we have the lock in once.doSlow()
		c.syncServiceContext.cc = cc
		c.syncServiceContext.service = ptypes.NewSyncAgentServiceClient(cc)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c.syncServiceContext.service, nil
}

func GetDiskInfo(info *ptypes.DiskInfo) *types.DiskInfo {
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

func GetReplicaInfo(r *ptypes.Replica) *types.ReplicaInfo {
	replicaInfo := &types.ReplicaInfo{
		Dirty:                   r.Dirty,
		Rebuilding:              r.Rebuilding,
		Head:                    r.Head,
		Parent:                  r.Parent,
		Size:                    r.Size,
		SectorSize:              r.SectorSize,
		BackingFile:             r.BackingFile,
		State:                   r.State,
		Chain:                   r.Chain,
		Disks:                   map[string]types.DiskInfo{},
		RemainSnapshots:         int(r.RemainSnapshots),
		RevisionCounter:         r.RevisionCounter,
		LastModifyTime:          r.LastModifyTime,
		HeadFileSize:            r.HeadFileSize,
		RevisionCounterDisabled: r.RevisionCounterDisabled,
	}

	for diskName, diskInfo := range r.Disks {
		replicaInfo.Disks[diskName] = *GetDiskInfo(diskInfo)
	}

	return replicaInfo
}

func syncFileInfoListToSyncAgentGRPCFormat(list []types.SyncFileInfo) []*ptypes.SyncFileInfo {
	res := []*ptypes.SyncFileInfo{}
	for _, info := range list {
		res = append(res, syncFileInfoToSyncAgentGRPCFormat(info))
	}
	return res
}

func syncFileInfoToSyncAgentGRPCFormat(info types.SyncFileInfo) *ptypes.SyncFileInfo {
	return &ptypes.SyncFileInfo{
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

	resp, err := replicaServiceClient.ReplicaGet(ctx, &empty.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to get replica %v: %v", c.replicaServiceURL, err)
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

	if _, err := replicaServiceClient.ReplicaOpen(ctx, &empty.Empty{}); err != nil {
		return fmt.Errorf("failed to open replica %v: %v", c.replicaServiceURL, err)
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

	if _, err := replicaServiceClient.ReplicaClose(ctx, &empty.Empty{}); err != nil {
		return fmt.Errorf("failed to close replica %v: %v", c.replicaServiceURL, err)
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

	resp, err := replicaServiceClient.ReplicaReload(ctx, &empty.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to reload replica %v: %v", c.replicaServiceURL, err)
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

	resp, err := replicaServiceClient.ReplicaExpand(ctx, &ptypes.ReplicaExpandRequest{
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

	if _, err := replicaServiceClient.ReplicaRevert(ctx, &ptypes.ReplicaRevertRequest{
		Name:    name,
		Created: created,
	}); err != nil {
		return fmt.Errorf("failed to revert replica %v: %v", c.replicaServiceURL, err)
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

	if _, err := replicaServiceClient.DiskRemove(ctx, &ptypes.DiskRemoveRequest{
		Name:  disk,
		Force: force,
	}); err != nil {
		return fmt.Errorf("failed to remove disk %v for replica %v: %v", disk, c.replicaServiceURL, err)
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

	if _, err := replicaServiceClient.DiskReplace(ctx, &ptypes.DiskReplaceRequest{
		Target: target,
		Source: source,
	}); err != nil {
		return fmt.Errorf("failed to replace disk %v with %v for replica %v: %v", target, source, c.replicaServiceURL, err)
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

	reply, err := replicaServiceClient.DiskPrepareRemove(ctx, &ptypes.DiskPrepareRemoveRequest{
		Name: disk,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to prepare removing disk %v for replica %v: %v", disk, c.replicaServiceURL, err)
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

	if _, err := replicaServiceClient.DiskMarkAsRemoved(ctx, &ptypes.DiskMarkAsRemovedRequest{
		Name: disk,
	}); err != nil {
		return fmt.Errorf("failed to mark disk %v as removed for replica %v: %v", disk, c.replicaServiceURL, err)
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

	if _, err := replicaServiceClient.RebuildingSet(ctx, &ptypes.RebuildingSetRequest{
		Rebuilding: rebuilding,
	}); err != nil {
		return fmt.Errorf("failed to set rebuilding to %v for replica %v: %v", rebuilding, c.replicaServiceURL, err)
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

	if _, err := syncAgentServiceClient.FileRemove(ctx, &ptypes.FileRemoveRequest{
		FileName: file,
	}); err != nil {
		return fmt.Errorf("failed to remove file %v: %v", file, err)
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

	if _, err := syncAgentServiceClient.FileRename(ctx, &ptypes.FileRenameRequest{
		OldFileName: oldFileName,
		NewFileName: newFileName,
	}); err != nil {
		return fmt.Errorf("failed to rename or replace old file %v with new file %v: %v", oldFileName, newFileName, err)
	}

	return nil
}

func (c *ReplicaClient) SendFile(from, host string, port int32) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceLongTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.FileSend(ctx, &ptypes.FileSendRequest{
		FromFileName: from,
		Host:         host,
		Port:         port,
	}); err != nil {
		return fmt.Errorf("failed to send file %v to %v:%v: %v", from, host, port, err)
	}

	return nil
}

func (c *ReplicaClient) ExportVolume(snapshotName, host string, port int32, exportBackingImageIfExist bool) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceLongTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.VolumeExport(ctx, &ptypes.VolumeExportRequest{
		SnapshotFileName:          snapshotName,
		Host:                      host,
		Port:                      port,
		ExportBackingImageIfExist: exportBackingImageIfExist,
	}); err != nil {
		return fmt.Errorf("failed to export snapshot %v to %v:%v: %v", snapshotName, host, port, err)
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

	reply, err := syncAgentServiceClient.ReceiverLaunch(ctx, &ptypes.ReceiverLaunchRequest{
		ToFileName: toFilePath,
	})
	if err != nil {
		return "", 0, fmt.Errorf("failed to launch receiver for %v: %v", toFilePath, err)
	}

	return c.host, reply.Port, nil
}

func (c *ReplicaClient) SyncFiles(fromAddress string, list []types.SyncFileInfo) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceLongTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.FilesSync(ctx, &ptypes.FilesSyncRequest{
		FromAddress:      fromAddress,
		ToHost:           c.host,
		SyncFileInfoList: syncFileInfoListToSyncAgentGRPCFormat(list),
	}); err != nil {
		return fmt.Errorf("failed to sync files %+v from %v: %v", list, fromAddress, err)
	}

	return nil
}

func (c *ReplicaClient) CreateBackup(backupName, snapshot, dest, volume, backingImageName, backingImageChecksum string, labels []string, credential map[string]string) (*ptypes.BackupCreateResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	resp, err := syncAgentServiceClient.BackupCreate(ctx, &ptypes.BackupCreateRequest{
		SnapshotFileName:     snapshot,
		BackupTarget:         dest,
		VolumeName:           volume,
		BackingImageName:     backingImageName,
		BackingImageChecksum: backingImageChecksum,
		Labels:               labels,
		Credential:           credential,
		BackupName:           backupName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create backup to %v for volume %v: %v", dest, volume, err)
	}

	return resp, nil
}

func (c *ReplicaClient) BackupStatus(backupName string) (*ptypes.BackupStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	resp, err := syncAgentServiceClient.BackupStatus(ctx, &ptypes.BackupStatusRequest{
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

	if _, err := syncAgentServiceClient.BackupRemove(ctx, &ptypes.BackupRemoveRequest{
		Backup: backup,
	}); err != nil {
		return fmt.Errorf("failed to remove backup %v: %v", backup, err)
	}

	return nil
}

func (c *ReplicaClient) RestoreBackup(backup, snapshotDiskName string, credential map[string]string) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.BackupRestore(ctx, &ptypes.BackupRestoreRequest{
		Backup:           backup,
		SnapshotDiskName: snapshotDiskName,
		Credential:       credential,
	}); err != nil {
		return fmt.Errorf("failed to restore backup data %v to snapshot file %v: %v", backup, snapshotDiskName, err)
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

	if _, err := syncAgentServiceClient.Reset(ctx, &empty.Empty{}); err != nil {
		return fmt.Errorf("failed to cleanup restore info in Sync Agent Server: %v", err)
	}

	return nil
}

func (c *ReplicaClient) RestoreStatus() (*ptypes.RestoreStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	resp, err := syncAgentServiceClient.RestoreStatus(ctx, &empty.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to get restore status: %v", err)
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

	if _, err := syncAgentServiceClient.SnapshotPurge(ctx, &empty.Empty{}); err != nil {
		return fmt.Errorf("failed to start snapshot purge: %v", err)
	}

	return nil
}

func (c *ReplicaClient) SnapshotPurgeStatus() (*ptypes.SnapshotPurgeStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	status, err := syncAgentServiceClient.SnapshotPurgeStatus(ctx, &empty.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot purge status: %v", err)
	}

	return status, nil
}

func (c *ReplicaClient) ReplicaRebuildStatus() (*ptypes.ReplicaRebuildStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	status, err := syncAgentServiceClient.ReplicaRebuildStatus(ctx, &empty.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to get replica rebuild status: %v", err)
	}

	return status, nil
}

func (c *ReplicaClient) CloneSnapshot(fromAddress, snapshotFileName string, exportBackingImageIfExist bool) error {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceLongTimeout)
	defer cancel()

	if _, err := syncAgentServiceClient.SnapshotClone(ctx, &ptypes.SnapshotCloneRequest{
		FromAddress:               fromAddress,
		ToHost:                    c.host,
		SnapshotFileName:          snapshotFileName,
		ExportBackingImageIfExist: exportBackingImageIfExist,
	}); err != nil {
		return fmt.Errorf("failed to clone snapshot %v from replica %v to host %v: %v", snapshotFileName, fromAddress, c.host, err)
	}

	return nil
}

func (c *ReplicaClient) SnapshotCloneStatus() (*ptypes.SnapshotCloneStatusResponse, error) {
	syncAgentServiceClient, err := c.getSyncServiceClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), GRPCServiceCommonTimeout)
	defer cancel()

	status, err := syncAgentServiceClient.SnapshotCloneStatus(ctx, &empty.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot clone status: %v", err)
	}
	return status, nil
}
