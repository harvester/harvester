package csi

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"k8s.io/mount-utils"

	utilexec "k8s.io/utils/exec"

	"github.com/longhorn/longhorn-manager/csi/crypto"

	longhornclient "github.com/longhorn/longhorn-manager/client"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	// CryptoKeyProvider specifies how the CryptoKeyValue is retrieved
	// We currently only support passphrase retrieval via direct secret values
	CryptoKeyProvider = "CRYPTO_KEY_PROVIDER"
	CryptoKeyValue    = "CRYPTO_KEY_VALUE"
	CryptoKeyCipher   = "CRYPTO_KEY_CIPHER"
	CryptoKeyHash     = "CRYPTO_KEY_HASH"
	CryptoKeySize     = "CRYPTO_KEY_SIZE"
	CryptoPBKDF       = "CRYPTO_PBKDF"

	defaultFsType = "ext4"
)

type fsParameters struct {
	formatParameters string
}

var supportedFs = map[string]fsParameters{
	"ext4": {
		formatParameters: "-b4096",
	},
	"xfs": {
		formatParameters: "-ssize=4096 -bsize=4096",
	},
}

type NodeServer struct {
	apiClient *longhornclient.RancherClient
	nodeID    string
	caps      []*csi.NodeServiceCapability
}

func NewNodeServer(apiClient *longhornclient.RancherClient, nodeID string) *NodeServer {
	return &NodeServer{
		apiClient: apiClient,
		nodeID:    nodeID,
		caps: getNodeServiceCapabilities(
			[]csi.NodeServiceCapability_RPC_Type{
				csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
				csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
			}),
	}
}

func getLoggerForCSINodeServer() *logrus.Entry {
	return logrus.StandardLogger().WithField("component", "csi-node-server")
}

// NodePublishVolume will mount the volume /dev/longhorn/<volume_name> to target_path
func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log := getLoggerForCSINodeServer()
	log = log.WithFields(logrus.Fields{"function": "NodePublishVolume"})

	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path missing in request")
	}

	stagingPath := req.GetStagingTargetPath()
	if stagingPath == "" {
		return nil, status.Error(codes.InvalidArgument, "staging path missing in request")
	}

	volumeCapability := req.GetVolumeCapability()
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability missing in request")
	}

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	volume, err := ns.apiClient.Volume.ById(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if volume == nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found", volumeID)
	}

	mounter, err := ns.getMounter(volume, volumeCapability, req.VolumeContext)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// For mount volumes, we don't want multiple controllers for a volume, since the filesystem could get messed up
	if len(volume.Controllers) == 0 || (len(volume.Controllers) > 1 && volumeCapability.GetBlock() == nil) {
		return nil, status.Errorf(codes.InvalidArgument, "volume %s invalid controller count %v", volumeID, len(volume.Controllers))
	}

	if volume.DisableFrontend || volume.Frontend != string(longhorn.VolumeFrontendBlockDev) {
		return nil, status.Errorf(codes.InvalidArgument, "volume %s invalid frontend type %v is disabled %v", volumeID, volume.Frontend, volume.DisableFrontend)
	}

	// Check volume attachment status
	if volume.State != string(longhorn.VolumeStateAttached) || volume.Controllers[0].Endpoint == "" {
		log.Infof("Volume %v hasn't been attached yet, unmounting potential mount point %v", volumeID, targetPath)
		if err := unmount(targetPath, mounter); err != nil {
			log.WithError(err).Warnf("Failed to unmount: %v", targetPath)
		}
		return nil, status.Errorf(codes.InvalidArgument, "volume %s hasn't been attached yet", volumeID)
	}

	if !volume.Ready {
		return nil, status.Errorf(codes.Aborted, "volume %s is not ready for workloads", volumeID)
	}

	if volumeCapability.GetBlock() != nil {
		devicePath := volume.Controllers[0].Endpoint
		if err := ns.nodePublishBlockVolume(volumeID, devicePath, targetPath, mounter); err != nil {
			return nil, err
		}

		log.Infof("Published BlockVolume %s", volumeID)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// we validate the staging path to make sure the global mount is still valid
	if isMnt, err := ensureMountPoint(stagingPath, mounter); err != nil || !isMnt {
		msg := fmt.Sprintf("Staging path is no longer valid for volume %v", volumeID)
		log.Error(msg)

		// HACK: normally when we return FailedPrecondition below kubelet should call NodeStageVolume again
		//	but currently it does not, so we manually call NodeStageVolume to remount the block device globally
		//	we currently don't reuse the previously mapped block device (major:minor) so the initial mount even after
		//	reattachment of the longhorn block dev is no longer valid
		log.Warnf("Calling NodeUnstageVolume for volume %v", volumeID)
		_, _ = ns.NodeUnstageVolume(ctx, &csi.NodeUnstageVolumeRequest{
			VolumeId:          volumeID,
			StagingTargetPath: stagingPath,
		})

		log.Warnf("Calling NodeStageVolume for volume %v", volumeID)
		_, err := ns.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
			VolumeId:          volumeID,
			PublishContext:    req.PublishContext,
			StagingTargetPath: stagingPath,
			VolumeCapability:  volumeCapability,
			Secrets:           req.Secrets,
			VolumeContext:     req.VolumeContext,
		})
		if err != nil {
			log.Errorf("Failed NodeStageVolume staging path is still in a bad state for volume %v", volumeID)
			return nil, status.Error(codes.FailedPrecondition, msg)
		}
	}

	isMnt, err := ensureMountPoint(targetPath, mounter)
	if err != nil {
		msg := fmt.Sprintf("Failed to prepare mount point for volume %v error %v", volumeID, err)
		log.Error(msg)
		return nil, status.Error(codes.Internal, msg)
	}

	if isMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}
	mountOptions = append(mountOptions, volumeCapability.GetMount().GetMountFlags()...)

	if err := mounter.Mount(stagingPath, targetPath, "", mountOptions); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to bind mount volume %v", volumeID)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeServer) nodeStageSharedVolume(volumeID, shareEndpoint, targetPath string, mounter mount.Interface, customMountOptions []string) error {
	log := getLoggerForCSINodeServer()

	isMnt, err := ensureMountPoint(targetPath, mounter)

	if err != nil {
		return status.Errorf(codes.Internal, "Failed to prepare mount point for shared volume %v error %v", volumeID, err)
	}

	if isMnt {
		return nil
	}

	uri, err := url.Parse(shareEndpoint)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "Invalid share endpoint %v for volume %v", shareEndpoint, volumeID)
	}

	// share endpoint is of the form nfs://server/export
	fsType := uri.Scheme
	if fsType != "nfs" {
		return status.Errorf(codes.InvalidArgument, "Unsupported share type %v for volume %v share endpoint %v", fsType, volumeID, shareEndpoint)
	}

	server := uri.Host
	exportPath := uri.Path
	export := fmt.Sprintf("%s:%s", server, exportPath)

	defaultMountOptions := []string{
		"vers=4.1",
		"noresvport",
		//"sync",    // sync mode is prohibitively expensive on the client, so we allow for host defaults
		//"intr",
		//"hard",
		//"softerr", // for this release we use soft mode, so we can always cleanup mount points
		"timeo=600", // This is tenths of a second, so a 60 second timeout, each retrans the timeout will be linearly increased, 60s, 120s, 240s, 480s, 600s(max)
		"retrans=5", // We try the io operation for a total of 5 times, before failing
	}

	mountOptions := append(defaultMountOptions, []string{"softerr"}...)
	if len(customMountOptions) != 0 {
		mountOptions = customMountOptions
	}

	log.Infof("Mounting shared volume %v on node %v via share endpoint %v with mount options %v", volumeID, ns.nodeID, shareEndpoint, mountOptions)
	if err := mounter.Mount(export, targetPath, fsType, mountOptions); err != nil {
		if len(customMountOptions) == 0 && strings.Contains(err.Error(), "an incorrect mount option was specified") {
			log.WithError(err).Warnf("Failed to mount volume %v with default mount options, retrying with soft mount", volumeID)
			mountOptions = append(defaultMountOptions, []string{"soft"}...)
			err = mounter.Mount(export, targetPath, fsType, mountOptions)
			if err == nil {
				return nil
			}
		}
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

func (ns *NodeServer) nodeStageMountVolume(volumeID, devicePath, targetPath, fsType string, mountFlags []string, mounter *mount.SafeFormatAndMount) error {
	isMnt, err := ensureMountPoint(targetPath, mounter)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to prepare mount point for volume %v error %v", volumeID, err)
	}

	if isMnt {
		return nil
	}

	if err := mounter.FormatAndMount(devicePath, targetPath, fsType, mountFlags); err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	return nil
}

func (ns *NodeServer) nodePublishBlockVolume(volumeID, devicePath, targetPath string, mounter mount.Interface) error {
	// we ensure the parent directory exists and is valid
	if _, err := ensureMountPoint(filepath.Dir(targetPath), mounter); err != nil {
		return status.Errorf(codes.Internal, "Failed to prepare mount point for block device %v error %v", devicePath, err)
	}

	// create file where we can bind mount the device to
	if err := makeFile(targetPath); err != nil {
		return status.Errorf(codes.Internal, "Error in making file %v", err)
	}

	if err := mounter.Mount(devicePath, targetPath, "", []string{"bind"}); err != nil {
		if removeErr := os.Remove(targetPath); removeErr != nil {
			return status.Errorf(codes.Internal, "Could not remove mount target %q: %v", targetPath, err)
		}
		return status.Errorf(codes.Internal, "Could not mount %q at %q: %v", devicePath, targetPath, err)
	}
	return nil
}

func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	log := getLoggerForCSINodeServer()
	log = log.WithFields(logrus.Fields{"function": "NodeUnpublishVolume"})

	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path missing in request")
	}

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	if err := cleanupMountPoint(targetPath, mount.New("")); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to cleanup volume %s mount point %v error %v", volumeID, targetPath, err)
	}

	log.Infof("Volume %s unmounted from path %s", volumeID, targetPath)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	log := getLoggerForCSINodeServer()
	log = log.WithFields(logrus.Fields{"function": "NodeStageVolume"})

	targetPath := req.GetStagingTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "staging path missing in request")
	}

	volumeCapability := req.GetVolumeCapability()
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability missing in request")
	}

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	volume, err := ns.apiClient.Volume.ById(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if volume == nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found", volumeID)
	}

	mounter, err := ns.getMounter(volume, volumeCapability, req.VolumeContext)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// For mount volumes, we don't want multiple controllers for a volume, since the filesystem could get messed up
	if len(volume.Controllers) == 0 || (len(volume.Controllers) > 1 && volumeCapability.GetBlock() == nil) {
		return nil, status.Errorf(codes.InvalidArgument, "volume %s invalid controller count %v", volumeID, len(volume.Controllers))
	}

	if volume.DisableFrontend || volume.Frontend != string(longhorn.VolumeFrontendBlockDev) {
		return nil, status.Errorf(codes.InvalidArgument, "volume %s invalid frontend type %v is disabled %v", volumeID, volume.Frontend, volume.DisableFrontend)
	}

	// Check volume attachment status
	if volume.State != string(longhorn.VolumeStateAttached) || volume.Controllers[0].Endpoint == "" {
		log.Infof("Volume %v hasn't been attached yet, try unmounting potential mount point %v", volumeID, targetPath)
		if err := unmount(targetPath, mounter); err != nil {
			log.Warnf("Failed to unmount error: %v", err)
		}
		return nil, status.Errorf(codes.InvalidArgument, "volume %s hasn't been attached yet", volumeID)
	}

	if !volume.Ready {
		return nil, status.Errorf(codes.Aborted, "volume %s is not ready for workloads", volumeID)
	}

	devicePath := volume.Controllers[0].Endpoint

	// do nothing for block devices, since they are handled by publish
	if volumeCapability.GetBlock() != nil {
		return &csi.NodeStageVolumeResponse{}, nil
	}

	if requiresSharedAccess(volume, volumeCapability) && !volume.Migratable {
		if volume.AccessMode != string(longhorn.AccessModeReadWriteMany) {
			return nil, status.Errorf(codes.FailedPrecondition, "volume %s requires shared access but is not marked for shared use", volumeID)
		}

		if !isVolumeShareAvailable(volume) {
			return nil, status.Errorf(codes.Aborted, "volume %s share not yet available", volumeID)
		}

		// undocumented field to allow testing different nfs mount options
		// this can be used to enable the default host (ubuntu) client async mode
		var mountOptions []string
		if len(req.VolumeContext["nfsOptions"]) > 0 {
			mountOptions = strings.Split(req.VolumeContext["nfsOptions"], ",")
		}

		if err := ns.nodeStageSharedVolume(volumeID, volume.ShareEndpoint, targetPath, mounter, mountOptions); err != nil {
			return nil, err
		}

		log.Infof("Mounted shared volume %v on node %v via share endpoint %v", volumeID, ns.nodeID, volume.ShareEndpoint)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	options := volumeCapability.GetMount().GetMountFlags()
	fsType := volumeCapability.GetMount().GetFsType()
	if fsType == "" {
		fsType = defaultFsType
	}

	formatMounter, ok := mounter.(*mount.SafeFormatAndMount)
	if !ok {
		return nil, status.Errorf(codes.Internal, "volume %v cannot get format mounter that support filesystem %v creation", volumeID, fsType)
	}

	diskFormat, err := formatMounter.GetDiskFormat(devicePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to evaluate device filesystem format")
	}

	log.Infof("Volume %v device %v contains filesystem of format %v", volumeID, devicePath, diskFormat)

	if volume.Encrypted {
		secrets := req.GetSecrets()
		keyProvider := secrets[CryptoKeyProvider]
		passphrase := secrets[CryptoKeyValue]
		if keyProvider != "" && keyProvider != "secret" {
			return nil, status.Errorf(codes.InvalidArgument, "unsupported key provider %v for encrypted volume %v", keyProvider, volumeID)
		}

		if len(passphrase) == 0 {
			return nil, status.Errorf(codes.InvalidArgument, "missing passphrase for encrypted volume %v", volumeID)
		}

		if diskFormat != "" && diskFormat != "crypto_LUKS" {
			return nil, status.Errorf(codes.InvalidArgument, "unsupported disk encryption format %v", diskFormat)
		}

		cryptoParams := crypto.NewEncryptParams(keyProvider, secrets[CryptoKeyCipher], secrets[CryptoKeyHash], secrets[CryptoKeySize], secrets[CryptoPBKDF])

		// initial setup of longhorn device for crypto
		if diskFormat == "" {
			if err := crypto.EncryptVolume(devicePath, passphrase, cryptoParams); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}

		cryptoDevice := crypto.VolumeMapper(volumeID)
		log.Infof("Volume %s requires crypto device %s", volumeID, cryptoDevice)

		if err := crypto.OpenVolume(volumeID, devicePath, passphrase); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// update the device path to point to the new crypto device
		devicePath = cryptoDevice
	}

	if err := ns.nodeStageMountVolume(volumeID, devicePath, targetPath, fsType, options, formatMounter); err != nil {
		return nil, err
	}

	// check if we need to resize the fs
	// this is important since cloned volumes of bigger size don't trigger NodeExpandVolume
	// therefore NodeExpandVolume is kind of redundant since we have to do this anyway
	// some refs below for more details
	// https://github.com/kubernetes/kubernetes/issues/94929
	// https://github.com/kubernetes-sigs/aws-ebs-csi-driver/pull/753
	resizer := mount.NewResizeFs(utilexec.New())
	if needsResize, err := resizer.NeedResize(devicePath, targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else if needsResize {
		if resized, err := resizer.Resize(devicePath, targetPath); err != nil {
			log.WithError(err).Errorf("Mounted volume %v on node %v failed required filesystem resize", volumeID, ns.nodeID)
			return nil, status.Error(codes.Internal, err.Error())
		} else if resized {
			log.Infof("Mounted volume %v on node %v successfully resized filesystem after mount", volumeID, ns.nodeID)
		} else {
			log.Infof("Mounted volume %v on node %v already has correct filesystem size", volumeID, ns.nodeID)
		}
	} else {
		log.Infof("Mounted volume %v on node %v does not require filesystem resize", volumeID, ns.nodeID)
	}

	log.Infof("Mounted volume %v on node %v via device %v", volumeID, ns.nodeID, devicePath)
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	log := getLoggerForCSINodeServer()
	log = log.WithFields(logrus.Fields{"function": "NodeUnstageVolume"})

	targetPath := req.GetStagingTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path missing in request")
	}

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	// CO owns the staging_path so we only unmount but not remove the path
	if err := unmount(targetPath, mount.New("")); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to unmount volume %s mount point %v error %v", volumeID, targetPath, err))
	}

	// optionally try to retrieve the volume and check if it's an RWX volume
	// if it is we let the share-manager clean up the crypto device
	volume, _ := ns.apiClient.Volume.ById(volumeID)
	cleanupCryptoDevice := !requiresSharedAccess(volume, nil)

	if cleanupCryptoDevice {
		cryptoDevice := crypto.VolumeMapper(volumeID)
		if isOpen, err := crypto.IsDeviceOpen(cryptoDevice); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		} else if isOpen {
			log.Infof("Volume %s closing active crypto device %s", volumeID, cryptoDevice)
			if err := crypto.CloseVolume(volumeID); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}

	log.Infof("Volume %s unmounted from node path %s", volumeID, targetPath)
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *NodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	volumePath := req.GetVolumePath()
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume path missing in request")
	}

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	existVol, err := ns.apiClient.Volume.ById(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if existVol == nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found", volumeID)
	}

	isBlockVolume, err := isBlockDevice(volumePath)
	if err != nil {
		// ENOENT means the volumePath does not exist
		// See https://man7.org/linux/man-pages/man2/stat.2.html for details.
		if errors.Is(err, unix.ENOENT) {
			return nil, status.Errorf(codes.NotFound, "volume %v is not mounted on path %v", volumeID, volumePath)
		}
		return nil, status.Errorf(codes.Internal, "failed to check volume mode for volume path %v: %v", volumePath, err)
	}

	if isBlockVolume {
		volCapacity, err := strconv.ParseInt(existVol.Size, 10, 64)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to convert volume size %v: %v", existVol.Size, err)
		}
		return &csi.NodeGetVolumeStatsResponse{
			Usage: []*csi.VolumeUsage{
				&csi.VolumeUsage{
					Total: volCapacity,
					Unit:  csi.VolumeUsage_BYTES,
				},
			},
		}, nil
	}

	stats, err := getFilesystemStatistics(volumePath)
	if err != nil {
		// ENOENT means the volumePath does not exist
		// See http://man7.org/linux/man-pages/man2/statfs.2.html for details.
		if errors.Is(err, unix.ENOENT) {
			return nil, status.Errorf(codes.NotFound, "volume %v is not mounted on path %v", volumeID, volumePath)
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve capacity statistics for volume path %v: %v", volumePath, err)
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			&csi.VolumeUsage{
				Available: stats.availableBytes,
				Total:     stats.totalBytes,
				Used:      stats.usedBytes,
				Unit:      csi.VolumeUsage_BYTES,
			},
			&csi.VolumeUsage{
				Available: stats.availableInodes,
				Total:     stats.totalInodes,
				Used:      stats.usedInodes,
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}, nil
}

// NodeExpandVolume is designed to expand the file system for ONLINE expansion,
func (ns *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	log := getLoggerForCSINodeServer()
	log = log.WithFields(logrus.Fields{"function": "NodeExpandVolume"})

	if req.CapacityRange == nil {
		return nil, status.Error(codes.InvalidArgument, "capacity range missing in request")
	}
	requestedSize := req.CapacityRange.GetRequiredBytes()

	volumeCapability := req.GetVolumeCapability()
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability missing in request")
	}

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	if req.VolumeCapability.GetBlock() != nil {
		log.Infof("Volume %v on node %v does not require filesystem resize/node expansion since it is access mode Block", volumeID, ns.nodeID)
		return &csi.NodeExpandVolumeResponse{}, nil
	}

	volume, err := ns.apiClient.Volume.ById(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	if volume == nil {
		return nil, status.Errorf(codes.NotFound, "volume %s missing", volumeID)
	}
	if len(volume.Controllers) != 1 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid controller count %v for volume %v node expansion", len(volume.Controllers), volumeID)
	}
	if volume.State != string(longhorn.VolumeStateAttached) {
		return nil, status.Errorf(codes.FailedPrecondition, "invalid state %v for volume %v node expansion", volume.State, volumeID)
	}
	devicePath := volume.Controllers[0].Endpoint

	mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: utilexec.New()}
	diskFormat, err := mounter.GetDiskFormat(devicePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to evaluate device filesystem format for volume %v node expansion", volumeID)
	}
	if diskFormat == "" {
		return nil, fmt.Errorf("unknown filesystem type for volume %v node expansion", volumeID)
	}

	devicePath, err = func() (string, error) {
		if !volume.Encrypted {
			return devicePath, nil
		}
		if diskFormat != "crypto_LUKS" {
			return "", status.Errorf(codes.InvalidArgument, "unsupported disk encryption format %v", diskFormat)
		}
		devicePath = crypto.VolumeMapper(volumeID)

		// Need to enable feature gate in v1.25:
		// https://github.com/kubernetes/enhancements/issues/3107
		// https://kubernetes.io/blog/2022/09/21/kubernetes-1-25-use-secrets-while-expanding-csi-volumes-on-node-alpha/
		secrets := req.GetSecrets()
		if len(secrets) == 0 {
			log.Infof("Skip encrypto device resizing for volume %v node expansion since the secret empty, maybe the related feature gate is not enabled", volumeID)
			return devicePath, nil
		}
		keyProvider := secrets[CryptoKeyProvider]
		passphrase := secrets[CryptoKeyValue]
		if keyProvider != "" && keyProvider != "secret" {
			return "", status.Errorf(codes.InvalidArgument, "unsupported key provider %v for encrypted volume %v", keyProvider, volumeID)
		}
		if len(passphrase) == 0 {
			return "", status.Errorf(codes.InvalidArgument, "missing passphrase for encrypted volume %v", volumeID)
		}

		// blindly resize the encrypto device
		if err := crypto.ResizeEncryptoDevice(volumeID, passphrase); err != nil {
			return "", status.Errorf(codes.InvalidArgument, "failed to resize crypto device %v for volume %v node expansion", devicePath, volumeID)
		}

		return devicePath, nil
	}()

	resizer := mount.NewResizeFs(utilexec.New())
	if needsResize, err := resizer.NeedResize(devicePath, req.StagingTargetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else if needsResize {
		if resized, err := resizer.Resize(devicePath, req.StagingTargetPath); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		} else if resized {
			log.Infof("Volume %v on node %v successfully resized filesystem after mount", volumeID, ns.nodeID)
		} else {
			log.Infof("Volume %v on node %v already has correct filesystem size", volumeID, ns.nodeID)
		}
	} else {
		log.Infof("Volume %v on node %v does not require filesystem resize", volumeID, ns.nodeID)
	}

	return &csi.NodeExpandVolumeResponse{CapacityBytes: requestedSize}, nil
}

func (ns *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId:            ns.nodeID,
		MaxVolumesPerNode: 0, // technically the scsi kernel limit is the max limit of volumes
	}, nil
}

func (ns *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: ns.caps,
	}, nil
}

func getNodeServiceCapabilities(cs []csi.NodeServiceCapability_RPC_Type) []*csi.NodeServiceCapability {
	var nscs []*csi.NodeServiceCapability

	for _, cap := range cs {
		logrus.Infof("Enabling node service capability: %v", cap.String())
		nscs = append(nscs, &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: cap,
				},
			},
		})
	}

	return nscs
}

func (ns *NodeServer) getMounter(volume *longhornclient.Volume, volumeCapability *csi.VolumeCapability, volumeContext map[string]string) (mount.Interface, error) {
	if volumeCapability.GetBlock() != nil {
		return mount.New(""), nil
	}

	// HACK: to nsenter host namespaces for the nfs mounts to stay available after csi plugin dies
	if requiresSharedAccess(volume, volumeCapability) && !volume.Migratable {
		return mount.New("/usr/local/sbin/nsmounter"), nil
	}

	// mounter that can format and use hard coded filesystem params
	if volumeCapability.GetMount() != nil {
		fsType := volumeCapability.GetMount().GetFsType()
		if fsType == "" {
			fsType = defaultFsType
		}

		// To allow users to override the default block size,
		// put the default block size in front of other user-defined parameters.
		params := ""
		if fsParams, ok := supportedFs[fsType]; ok {
			params += fsParams.formatParameters
		}

		//If the user specifies parameters in the storage class, the parameters are appended after the default value.
		if mkfsParams, ok := volumeContext["mkfsParams"]; ok && mkfsParams != "" {
			params += " " + mkfsParams
		}

		mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: utilexec.New()}
		if _, ok := supportedFs[fsType]; ok {
			logrus.Infof("Volume %v using user and longhorn provided %v fs creation params: %s", volume.Name, fsType, params)
			cmdParamMapping := map[string]string{"mkfs." + fsType: params}
			mounter = &mount.SafeFormatAndMount{
				Interface: mount.New(""),
				Exec:      NewForcedParamsExec(cmdParamMapping),
			}
		} else {
			logrus.Warnf("Volume %v with unsupported filesystem %v, use default fs creation params", volume.Name, fsType)
		}
		return mounter, nil
	}

	return nil, fmt.Errorf("failed to get mounter for volume %v unsupported volume capability %v", volume.Name, volumeCapability.GetAccessType())
}
