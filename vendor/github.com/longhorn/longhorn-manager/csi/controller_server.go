package csi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhornclient "github.com/longhorn/longhorn-manager/client"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	// we wait 1m30s for the volume state polling, this leaves 20s for the rest of the function call
	timeoutAttachDetach         = 90 * time.Second
	tickAttachDetach            = 2 * time.Second
	timeoutBackupControllerSync = 30 * time.Second
	tickBackupControllerSync    = 2 * time.Second
	backupStateCompleted        = "Completed"
	timeoutSnapshotCreation     = 90 * time.Second
	tickSnapshotCreation        = 2 * time.Second
	timeoutSnapshotDeletion     = 90 * time.Second
	tickSnapshotDeletion        = 2 * time.Second

	csiSnapshotTypeLonghornSnapshot         = "snap"
	csiSnapshotTypeLonghornBackingImage     = "bi"
	csiSnapshotTypeLonghornBackup           = "bak"
	deprecatedCSISnapshotTypeLonghornBackup = "bs"
)

type ControllerServer struct {
	apiClient   *longhornclient.RancherClient
	nodeID      string
	caps        []*csi.ControllerServiceCapability
	accessModes []*csi.VolumeCapability_AccessMode
}

func NewControllerServer(apiClient *longhornclient.RancherClient, nodeID string) *ControllerServer {
	return &ControllerServer{
		apiClient: apiClient,
		nodeID:    nodeID,
		caps: getControllerServiceCapabilities(
			[]csi.ControllerServiceCapability_RPC_Type{
				csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
				csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
				csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
				csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
				csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
			}),
		accessModes: getVolumeCapabilityAccessModes(
			[]csi.VolumeCapability_AccessMode_Mode{
				csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			}),
	}
}

func getLoggerForCSIControllerServer() *logrus.Entry {
	return logrus.StandardLogger().WithField("component", "csi-controller-server")
}

func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "CreateVolume"})

	volumeID := util.AutoCorrectName(req.GetName(), datastore.NameMaximumLength)
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}
	volumeCaps := req.GetVolumeCapabilities()
	if err := cs.validateVolumeCapabilities(volumeCaps); err != nil {
		return nil, err
	}
	volumeParameters := req.GetParameters()
	if volumeParameters == nil {
		volumeParameters = map[string]string{}
	}
	var reqVolSizeBytes int64
	if req.GetCapacityRange() != nil {
		reqVolSizeBytes = req.GetCapacityRange().GetRequiredBytes()
	}
	if reqVolSizeBytes < util.MinimalVolumeSize {
		log.Infof("Volume %s requested capacity %v is smaller than minimal capacity %v, enforcing minimal capacity.", volumeID, reqVolSizeBytes, util.MinimalVolumeSize)
		reqVolSizeBytes = util.MinimalVolumeSize
	}
	// Round up to multiple of 2 * 1024 * 1024
	reqVolSizeBytes = util.RoundUpSize(reqVolSizeBytes)

	volumeSource := req.GetVolumeContentSource()
	if volumeSource != nil {
		switch volumeSource.Type.(type) {
		case *csi.VolumeContentSource_Snapshot:
			if snapshot := volumeSource.GetSnapshot(); snapshot != nil {
				csiSnapshotType, sourceVolumeName, id := decodeSnapshotID(snapshot.SnapshotId)
				switch csiSnapshotType {
				case csiSnapshotTypeLonghornBackingImage:
					backingImageParameters := decodeSnapshoBackingImageID(snapshot.SnapshotId)
					if backingImageParameters[longhorn.BackingImageParameterName] == "" || backingImageParameters[longhorn.BackingImageParameterDataSourceType] == "" {
						return nil, status.Errorf(codes.InvalidArgument, "invalid CSI snapshotHandle %v for backing image", snapshot.SnapshotId)
					}
					updateVolumeParamsForBackingImage(volumeParameters, backingImageParameters)
				case csiSnapshotTypeLonghornSnapshot:
					if id == "" {
						return nil, status.Errorf(codes.NotFound, "volume source snapshot %v is not found", snapshot.SnapshotId)
					}
					dataSource, _ := types.NewVolumeDataSource(longhorn.VolumeDataSourceTypeSnapshot, map[string]string{types.VolumeNameKey: sourceVolumeName, types.SnapshotNameKey: id})
					volumeParameters["dataSource"] = string(dataSource)
				case csiSnapshotTypeLonghornBackup:
					if id == "" {
						return nil, status.Errorf(codes.NotFound, "volume source snapshot %v is not found", snapshot.SnapshotId)
					}
					backupVolume, backupName := sourceVolumeName, id
					bv, err := cs.apiClient.BackupVolume.ById(backupVolume)
					if err != nil {
						return nil, status.Errorf(codes.NotFound, "failed to restore CSI snapshot %s backup volume %s unavailable", snapshot.SnapshotId, backupVolume)
					}

					backup, err := cs.apiClient.BackupVolume.ActionBackupGet(bv, &longhornclient.BackupInput{Name: backupName})
					if err != nil {
						return nil, status.Errorf(codes.NotFound, "failed to restore CSI snapshot %v backup %s unavailable", snapshot.SnapshotId, backupName)
					}

					// use the fromBackup method for the csi snapshot restores as well
					// the same parameter was previously only used for restores based on the storage class
					volumeParameters["fromBackup"] = backup.Url
				default:
					return nil, status.Errorf(codes.InvalidArgument, "invalid CSI snapshot type: %v. Must be %v, %v or %v",
						csiSnapshotType, csiSnapshotTypeLonghornSnapshot, csiSnapshotTypeLonghornBackup, csiSnapshotTypeLonghornBackingImage)
				}
			}
		case *csi.VolumeContentSource_Volume:
			if srcVolume := volumeSource.GetVolume(); srcVolume != nil {
				longhornSrcVol, err := cs.apiClient.Volume.ById(srcVolume.VolumeId)
				if err != nil {
					return nil, status.Errorf(codes.NotFound, "failed to clone volume: source volume %s is unavailable", srcVolume.VolumeId)
				}
				if longhornSrcVol == nil {
					return nil, status.Errorf(codes.NotFound, "failed to clone volume: source volume %s is not found", srcVolume.VolumeId)
				}

				// check size of source and requested
				srcVolSizeBytes, err := strconv.ParseInt(longhornSrcVol.Size, 10, 64)
				if err != nil {
					return nil, status.Errorf(codes.Internal, err.Error())
				}
				if reqVolSizeBytes != srcVolSizeBytes {
					return nil, status.Errorf(codes.OutOfRange, "failed to clone volume: the requested size (%v bytes) is different than the source volume size (%v bytes)", reqVolSizeBytes, srcVolSizeBytes)
				}

				dataSource, _ := types.NewVolumeDataSource(longhorn.VolumeDataSourceTypeVolume, map[string]string{types.VolumeNameKey: srcVolume.VolumeId})
				volumeParameters["dataSource"] = string(dataSource)
			}
		default:
			return nil, status.Errorf(codes.InvalidArgument, "%v not a proper volume source", volumeSource)
		}
	}

	existVol, err := cs.apiClient.Volume.ById(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if existVol != nil {
		log.Infof("Volume %v already exists with name %v", volumeID, existVol.Name)

		exVolSize, err := util.ConvertSize(existVol.Size)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if exVolSize != reqVolSizeBytes {
			return nil, status.Errorf(codes.AlreadyExists, "volume %s size %v differs from requested size %v", existVol.Name, exVolSize, reqVolSizeBytes)
		}

		// pass through the volume content source in case this volume is in the process of being created.
		// We won't wait for clone/restore to complete but return OK immediately here so that
		// if Kubernetes wants to abort/delete the cloning/restoring volume, it has the volume ID and is able to do so.
		// We will wait for clone/restore to complete inside ControllerPublishVolume.
		rsp := &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      existVol.Id,
				CapacityBytes: exVolSize,
				VolumeContext: volumeParameters,
				ContentSource: volumeSource,
			},
		}

		return rsp, nil
	}

	// regardless of the used storage class, if this is requested in rwx mode
	// we need to mark the volume as a shared volume
	for _, cap := range volumeCaps {
		if requiresSharedAccess(nil, cap) {
			volumeParameters["share"] = "true"
			break
		}
	}

	vol, err := getVolumeOptions(volumeParameters)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err = cs.checkAndPrepareBackingImage(volumeID, vol.BackingImage, volumeParameters); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	vol.Name = volumeID
	vol.Size = fmt.Sprintf("%d", reqVolSizeBytes)

	log.Infof("Creating a volume by API client, name: %s, size: %s, accessMode: %v, backendStoreDriver: %v",
		vol.Name, vol.Size, vol.AccessMode, vol.BackendStoreDriver)
	resVol, err := cs.apiClient.Volume.Create(vol)
	// TODO: implement error response code for Longhorn API to differentiate different error type.
	// For example, creating a volume from a non-existing snapshot should return codes.NotFound instead of codes.Internal
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Do we have a better condition than this?
	checkVolumeCreated := func(vol *longhornclient.Volume) bool {
		return vol.State == string(longhorn.VolumeStateDetached)
	}

	if !cs.waitForVolumeState(resVol.Id, "volume created", checkVolumeCreated, true, false) {
		return nil, status.Error(codes.DeadlineExceeded, "failed to wait for volume creation to complete")
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      resVol.Id,
			CapacityBytes: reqVolSizeBytes,
			VolumeContext: volumeParameters,
			ContentSource: volumeSource,
		},
	}, nil
}

func (cs *ControllerServer) checkAndPrepareBackingImage(volumeName, backingImageName string, volumeParameters map[string]string) error {
	if backingImageName == "" {
		return nil
	}

	bidsType := volumeParameters[longhorn.BackingImageParameterDataSourceType]
	biChecksum := volumeParameters[longhorn.BackingImageParameterChecksum]
	bidsParameters := map[string]string{}
	if bidsParametersStr := volumeParameters[longhorn.BackingImageParameterDataSourceParameters]; bidsParametersStr != "" {
		if err := json.Unmarshal([]byte(bidsParametersStr), &bidsParameters); err != nil {
			return fmt.Errorf("volume %s is unable to create missing backing image with parameters %s: %v", volumeName, bidsParametersStr, err)
		}
	}

	// There will be an empty BackingImage object rather than nil returned even if there is an error
	existingBackingImage, err := cs.apiClient.BackingImage.ById(backingImageName)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("volume %s is unable to retrieve backing image %s: %v", volumeName, backingImageName, err)
	}
	// A new backing image will be created automatically
	// if there is no existing backing image with the name and the type is `download` or `export-from-volume`.
	if existingBackingImage == nil || existingBackingImage.Name == "" {
		switch longhorn.BackingImageDataSourceType(bidsType) {
		case longhorn.BackingImageDataSourceTypeUpload:
			return fmt.Errorf("volume %s backing image type %v is not supported via CSI", volumeName, bidsType)
		case longhorn.BackingImageDataSourceTypeDownload:
			if bidsParameters[longhorn.DataSourceTypeDownloadParameterURL] == "" {
				return fmt.Errorf("volume %s missing parameters %v for preparing backing image",
					volumeName, longhorn.DataSourceTypeDownloadParameterURL)
			}
		case longhorn.BackingImageDataSourceTypeExportFromVolume:
			if bidsParameters[longhorn.DataSourceTypeExportParameterExportType] == "" || bidsParameters[longhorn.DataSourceTypeExportParameterVolumeName] == "" {
				return fmt.Errorf("volume %s missing parameters %v or %v for preparing backing image",
					volumeName, longhorn.DataSourceTypeExportParameterExportType, longhorn.DataSourceTypeExportParameterVolumeName)
			}
		default:
			return fmt.Errorf("volume %s backing image type %v is not supported via CSI", volumeName, bidsType)
		}

		_, err = cs.apiClient.BackingImage.Create(&longhornclient.BackingImage{
			Name:             backingImageName,
			ExpectedChecksum: biChecksum,
			SourceType:       bidsType,
			Parameters:       bidsParameters,
		})
		return err
	}

	if (bidsType != "" && bidsType != existingBackingImage.SourceType) || (len(bidsParameters) != 0 && !reflect.DeepEqual(existingBackingImage.Parameters, bidsParameters)) {
		return fmt.Errorf("existing backing image %v data source is different from the parameters in the creation request or StorageClass", backingImageName)
	}
	if biChecksum != "" {
		if (existingBackingImage.CurrentChecksum != "" && existingBackingImage.CurrentChecksum != biChecksum) ||
			(existingBackingImage.ExpectedChecksum != "" && existingBackingImage.ExpectedChecksum != biChecksum) {
			return fmt.Errorf("existing backing image %v expected checksum or current checksum doesn't match the specified checksum %v in the request", backingImageName, biChecksum)
		}
	}

	return nil
}

func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "DeleteVolume"})

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	existVol, err := cs.apiClient.Volume.ById(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if existVol == nil {
		return &csi.DeleteVolumeResponse{}, nil
	}

	log.Infof("Deleting volume %v", volumeID)
	if err = cs.apiClient.Volume.Delete(existVol); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	checkVolumeDeleted := func(vol *longhornclient.Volume) bool {
		return vol == nil
	}
	if !cs.waitForVolumeState(req.GetVolumeId(), "volume deleted", checkVolumeDeleted, false, true) {
		return nil, status.Errorf(codes.DeadlineExceeded, "failed to delete volume %s", volumeID)
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.caps,
	}, nil
}

func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	existVol, err := cs.apiClient.Volume.ById(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if existVol == nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found", volumeID)
	}

	if err := cs.validateVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return nil, err
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeContext:      req.GetVolumeContext(),
			VolumeCapabilities: req.GetVolumeCapabilities(),
			Parameters:         req.GetParameters(),
		},
	}, nil
}

// ControllerPublishVolume will attach the volume to the specified node
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	nodeID := req.GetNodeId()
	if nodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "node id missing in request")
	}

	volumeCapability := req.GetVolumeCapability()
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability missing in request")
	}

	// TODO: #1875 API returns error instead of not found, so we cannot differentiate between a retrieval failure and non existing resource
	if _, err := cs.apiClient.Node.ById(nodeID); err != nil {
		return nil, status.Errorf(codes.NotFound, "node %s not found", nodeID)
	}

	volume, err := cs.apiClient.Volume.ById(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if volume == nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found", volumeID)
	}

	if volume.Frontend != string(longhorn.VolumeFrontendBlockDev) {
		return nil, status.Errorf(codes.InvalidArgument, "volume %s invalid frontend type %s", volumeID, volume.Frontend)
	}

	if requiresSharedAccess(volume, volumeCapability) {
		volume, err = cs.updateVolumeAccessMode(volume, longhorn.AccessModeReadWriteMany)
		if err != nil {
			return nil, err
		}
	}

	// TODO: JM Restore should be handled by the volume attach call, consider returning `codes.Aborted`
	// TODO: JM should readiness be handled by the caller?
	//  Most of the readiness conditions are covered by the attach, except auto attachment which requires changes to the design
	//  should be handled by the processing of the api return codes
	if !volume.Ready {
		return nil, status.Errorf(codes.Aborted, "volume %s is not ready for workloads", volumeID)
	}

	attachmentID := generateAttachmentID(volumeID, nodeID)

	return cs.publishVolume(volume, nodeID, attachmentID, func() error {
		checkVolumePublished := func(vol *longhornclient.Volume) bool {
			isRegularRWXVolume := vol.AccessMode == string(longhorn.AccessModeReadWriteMany) && !vol.Migratable
			attachment, ok := vol.VolumeAttachment.Attachments[attachmentID]
			if isRegularRWXVolume {
				return ok && attachment.Satisfied
			}
			return ok && attachment.Satisfied && isVolumeAvailableOn(vol, nodeID)
		}
		if !cs.waitForVolumeState(volumeID, "volume published", checkVolumePublished, false, false) {
			// check if there is error while attaching
			if existVol, err := cs.apiClient.Volume.ById(volumeID); err == nil && existVol != nil {
				if attachment, ok := existVol.VolumeAttachment.Attachments[attachmentID]; ok {
					for _, condition := range attachment.Conditions {
						if condition.Type == longhorn.AttachmentStatusConditionTypeSatisfied && condition.Status == string(longhorn.ConditionStatusFalse) && condition.Message != "" {
							return status.Errorf(codes.Internal, "volume %v failed to attach to node %v with attachmentID %v: %v", volumeID, nodeID, attachmentID, condition.Message)
						}
					}
				}
			}
			return status.Errorf(codes.DeadlineExceeded, "volume %v failed to attach to node %v with attachmentID %v", volumeID, nodeID, attachmentID)
		}
		return nil
	})
}

// We pick the same name as the volume attachment object at
// https://github.com/kubernetes/kubernetes/blob/f1e74f77ff88abb7acf0fb0e86ba21bc0f2395c9/pkg/volume/csi/csi_attacher.go#L653-L656
func generateAttachmentID(volName, nodeID string) string {
	result := sha256.Sum256([]byte(fmt.Sprintf("%s%s%s", volName, types.LonghornDriverName, nodeID)))
	return fmt.Sprintf("csi-%x", result)
}

// publishVolume sends the actual attach request to the longhorn api and executes the passed waitForResult func
func (cs *ControllerServer) publishVolume(volume *longhornclient.Volume, nodeID, attachmentID string, waitForResult func() error) (*csi.ControllerPublishVolumeResponse, error) {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "publishVolume"})

	input := &longhornclient.AttachInput{
		HostId:          nodeID,
		DisableFrontend: false,
		AttacherType:    string(longhorn.AttacherTypeCSIAttacher),
		AttachmentID:    attachmentID,
	}

	log.Infof("Volume %v with accessMode %v requesting publishing with attachInput %+v", volume.Name, volume.AccessMode, input)
	if _, err := cs.apiClient.Volume.ActionAttach(volume, input); err != nil {
		// TODO: JM process the returned error and return the correct error responses for kubernetes
		//  i.e. FailedPrecondition if the RWO volume is already attached to a different node
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := waitForResult(); err != nil {
		return nil, err
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (cs *ControllerServer) updateVolumeAccessMode(volume *longhornclient.Volume, accessMode longhorn.AccessMode) (*longhornclient.Volume, error) {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "updateVolumeAccessMode"})

	mode := string(accessMode)
	if volume.AccessMode == mode {
		return volume, nil
	}

	volumeName := volume.Name
	input := &longhornclient.UpdateAccessModeInput{AccessMode: mode}

	log.Infof("Changing volume %s access mode to %s", volumeName, mode)
	volume, err := cs.apiClient.Volume.ActionUpdateAccessMode(volume, input)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to change Volume %s access mode to %s", volumeName, mode)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return volume, nil
}

// ControllerUnpublishVolume will detach the volume
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	nodeID := req.GetNodeId()

	volume, err := cs.apiClient.Volume.ById(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// VOLUME_NOT_FOUND is no longer the ControllerUnpublishVolume error
	// See https://github.com/container-storage-interface/spec/issues/382 for details
	if volume == nil {
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	attachmentID := generateAttachmentID(volumeID, nodeID)

	// TODO: hadndle cases in which NodeID is empty. Should we detach the volume from all nodes???

	return cs.unpublishVolume(volume, nodeID, attachmentID, func() error {
		checkVolumeUnpublished := func(vol *longhornclient.Volume) bool {
			_, ok := vol.VolumeAttachment.Attachments[attachmentID]
			return !ok
		}

		if !cs.waitForVolumeState(volumeID, "volume unpublished", checkVolumeUnpublished, false, true) {
			return status.Errorf(codes.DeadlineExceeded, "Failed to detach volume %s from node %s", volumeID, volumeID)
		}
		return nil
	})
}

// unpublishVolume sends the actual detach request to the longhorn api and executes the passed waitForResult func
func (cs *ControllerServer) unpublishVolume(volume *longhornclient.Volume, nodeID, attachmentID string, waitForResult func() error) (*csi.ControllerUnpublishVolumeResponse, error) {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "unpublishVolume"})

	logrus.Infof("Requesting volume %v detachment for %v with attachmentID %v ", volume.Name, nodeID, attachmentID)
	detachInput := &longhornclient.DetachInput{
		AttachmentID: attachmentID,
		HostId:       nodeID,
		// if nodeID == "" means to detach from all nodes
		ForceDetach: nodeID == "",
	}
	_, err := cs.apiClient.Volume.ActionDetach(volume, detachInput)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = waitForResult(); err != nil {
		return nil, err
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (cs *ControllerServer) ListVolumes(context.Context, *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) GetCapacity(context.Context, *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "CreateSnapshot"})

	var rsp *csi.CreateSnapshotResponse
	var err error
	defer func() {
		if err != nil {
			log.WithError(err).Errorf("Failed to create snapshot")
		}
	}()

	csiSnapshotType := normalizeCSISnapshotType(req.Parameters["type"])
	switch csiSnapshotType {
	case csiSnapshotTypeLonghornSnapshot:
		rsp, err = cs.createCSISnapshotTypeLonghornSnapshot(req)
	case csiSnapshotTypeLonghornBackingImage:
		rsp, err = cs.createCSISnapshotTypeLonghornBackingImage(req)
	case "", csiSnapshotTypeLonghornBackup:
		// For backward compatibility, empty type is considered as csiSnapshotTypeLonghornBackup
		rsp, err = cs.createCSISnapshotTypeLonghornBackup(req)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid CSI snapshot type: %v. Must be %v or %v or \"\"", csiSnapshotType, csiSnapshotTypeLonghornSnapshot, csiSnapshotTypeLonghornBackup)
	}

	return rsp, err
}

func (cs *ControllerServer) createCSISnapshotTypeLonghornBackingImage(req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "createCSISnapshotTypeLonghornBackingImage"})

	csiSnapshotName := req.GetName()
	csiVolumeName := req.GetSourceVolumeId()
	if len(csiVolumeName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume name must be provided")
	} else if len(csiSnapshotName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Snapshot name must be provided")
	}

	vol, err := cs.apiClient.Volume.ById(csiVolumeName)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if vol == nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found", csiVolumeName)
	}

	var backingImage *longhornclient.BackingImage
	backingImageListOutput, err := cs.apiClient.BackingImage.List(&longhornclient.ListOpts{})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	for _, bi := range backingImageListOutput.Data {
		if bi.Name == csiSnapshotName {
			backingImage = &bi
			break
		}
	}

	exportType, exist := req.Parameters["export-type"]
	if !exist {
		exportType = "raw"
	}
	biParameters := map[string]string{
		longhorn.DataSourceTypeExportParameterVolumeName: csiVolumeName,
		longhorn.DataSourceTypeExportParameterExportType: exportType,
	}
	if backingImage == nil {
		log.Infof("Creating backing image type snapshot %v exported from volume %s", csiSnapshotName, vol.Name)
		backingImage, err = cs.apiClient.BackingImage.Create(&longhornclient.BackingImage{
			Name:       csiSnapshotName,
			SourceType: string(longhorn.BackingImageDataSourceTypeExportFromVolume),
			Parameters: biParameters,
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	snapshotID := encodeSnapshotBackingImageID(backingImage.Name, exportType, vol.Name)
	return createSnapshotResponseForSnapshotTypeLonghornBackingImage(vol.Name, snapshotID, time.Now().UTC().Format(time.RFC3339), vol.Size, true), nil
}

func (cs *ControllerServer) createCSISnapshotTypeLonghornSnapshot(req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "createCSISnapshotTypeLonghornSnapshot"})

	csiLabels := req.Parameters
	csiSnapshotName := req.GetName()
	csiVolumeName := req.GetSourceVolumeId()
	if len(csiVolumeName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume name must be provided")
	} else if len(csiSnapshotName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Snapshot name must be provided")
	}

	vol, err := cs.apiClient.Volume.ById(csiVolumeName)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if vol == nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found", csiVolumeName)
	}

	var snapshotCR *longhornclient.SnapshotCR
	snapshotCRs, err := cs.apiClient.Volume.ActionSnapshotCRList(vol)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	for _, snapCR := range snapshotCRs.Data {
		if snapCR.Name == csiSnapshotName {
			snapshotCR = &snapCR
			break
		}
	}

	if snapshotCR == nil {
		log.Infof("Creating volume %s snapshot %s", vol.Name, csiSnapshotName)
		snapshotCR, err = cs.apiClient.Volume.ActionSnapshotCRCreate(vol, &longhornclient.SnapshotCRInput{
			Labels: csiLabels,
			Name:   csiSnapshotName,
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// wait for the snapshot creation to be fully finished
	snapshotCR, err = cs.waitForSnapshotToBeReady(snapshotCR.Name, vol.Name)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	snapshotID := encodeSnapshotID(csiSnapshotTypeLonghornSnapshot, vol.Name, snapshotCR.Name)
	return createSnapshotResponseForSnapshotTypeLonghornSnapshot(vol.Name, snapshotID, snapshotCR), nil
}

func (cs *ControllerServer) createCSISnapshotTypeLonghornBackup(req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "createCSISnapshotTypeLonghornBackup"})

	csiLabels := req.Parameters
	csiSnapshotName := req.GetName()
	csiVolumeName := req.GetSourceVolumeId()
	if len(csiVolumeName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume name must be provided")
	} else if len(csiSnapshotName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Snapshot name must be provided")
	}

	// We check for backup existence first, since it's possible that the actual volume is no longer available but the
	// backup still is.
	backup, err := cs.getBackup(csiVolumeName, csiSnapshotName)
	if err != nil {
		// Status code set in waitForBackupControllerSync.
		return nil, err
	}

	if backup != nil {
		// Per the CSI spec, if we are unable to complete the CreateSnapshot call successfully, we must return a non-ok
		// gRPC code. In practice, doing so ensures we get requeued (and quickly deleted) when we hit
		// https://github.com/kubernetes-csi/external-snapshotter/issues/880.
		if backup.Error != "" {
			return nil, status.Error(codes.Internal, backup.Error)
		}

		snapshotID := encodeSnapshotID(csiSnapshotTypeLonghornBackup, backup.VolumeName, backup.Name)
		rsp := createSnapshotResponseForSnapshotTypeLonghornBackup(backup.VolumeName, snapshotID,
			backup.SnapshotCreated, backup.VolumeSize, backup.State == string(longhorn.BackupStateCompleted))
		return rsp, nil
	}

	existVol, err := cs.apiClient.Volume.ById(csiVolumeName)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if existVol == nil {
		return nil, status.Errorf(codes.NotFound, "volume %s not found", csiVolumeName)
	}

	var snapshotCR *longhornclient.SnapshotCR
	snapshotCRs, err := cs.apiClient.Volume.ActionSnapshotCRList(existVol)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	for _, snap := range snapshotCRs.Data {
		if snap.Name == csiSnapshotName {
			snapshotCR = &snap
			break
		}
	}

	// no existing backup and no local snapshot, create a new one
	if snapshotCR == nil {
		log.Infof("Creating Volume %s snapshot %s", existVol.Name, csiSnapshotName)
		snapshotCR, err = cs.apiClient.Volume.ActionSnapshotCRCreate(existVol, &longhornclient.SnapshotCRInput{
			Labels: csiLabels,
			Name:   csiSnapshotName,
		})
		// failed to create snapshot, so there is no way to backup
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// wait for the snapshot creation to be fully finished
	snapshotCR, err = cs.waitForSnapshotToBeReady(snapshotCR.Name, existVol.Name)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// create backup based on local volume snapshot
	log.Infof("Creating volume %s backup for snapshot %s", existVol.Name, csiSnapshotName)
	existVol, err = cs.apiClient.Volume.ActionSnapshotBackup(existVol, &longhornclient.SnapshotInput{
		Labels: csiLabels,
		Name:   csiSnapshotName,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Get the newly created backup. Don't wait for the backup controller to actually start the backup and update the
	// status. It's possible that the backup operation can't actually be completed, but we need to return quickly so the
	// CO can unfreeze I/O (if freezing is supported) and without error (if possible) so the CO knows our ID and can use
	// it in future calls.
	backup, err = cs.waitForBackupControllerSync(existVol.Name, csiSnapshotName)
	if err != nil {
		// Status code set in waitForBackupControllerSync.
		return nil, err
	}

	logrus.Infof("Volume %s backup %s of snapshot %s created", existVol.Name, backup.Id, csiSnapshotName)
	snapshotID := encodeSnapshotID(csiSnapshotTypeLonghornBackup, existVol.Name, backup.Id)
	rsp := createSnapshotResponseForSnapshotTypeLonghornBackup(existVol.Name, snapshotID, snapshotCR.CreationTime,
		existVol.Size, backup.State == string(longhorn.BackupStateCompleted))
	return rsp, nil
}

func createSnapshotResponseForSnapshotTypeLonghornSnapshot(sourceVolumeName, snapshotID string, snapshotCR *longhornclient.SnapshotCR) *csi.CreateSnapshotResponse {
	creationTime, err := toProtoTimestamp(snapshotCR.CreationTime)
	if err != nil {
		logrus.Errorf("Failed to parse creation time %v for csi snapshot %v", snapshotCR.CreationTime, snapshotID)
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      snapshotCR.RestoreSize,
			SnapshotId:     snapshotID,
			SourceVolumeId: sourceVolumeName,
			CreationTime:   creationTime,
			ReadyToUse:     snapshotCR.ReadyToUse,
		},
	}
}

func createSnapshotResponseForSnapshotTypeLonghornBackup(sourceVolumeName, snapshotID, snapshotTime, sourceVolumeSize string, readyToUse bool) *csi.CreateSnapshotResponse {
	creationTime, err := toProtoTimestamp(snapshotTime)
	if err != nil {
		logrus.Errorf("Failed to parse creation time %v for CSI snapshot %v", snapshotTime, snapshotID)
	}

	size, _ := util.ConvertSize(sourceVolumeSize)
	size = util.RoundUpSize(size)
	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      size,
			SnapshotId:     snapshotID,
			SourceVolumeId: sourceVolumeName,
			CreationTime:   creationTime,
			ReadyToUse:     readyToUse,
		},
	}
}

func createSnapshotResponseForSnapshotTypeLonghornBackingImage(sourceVolumeName, snapshotID, snapshotTime, sourceVolumeSize string, readyToUse bool) *csi.CreateSnapshotResponse {
	creationTime, err := toProtoTimestamp(snapshotTime)
	if err != nil {
		logrus.Errorf("Failed to parse creation time %v for CSI snapshot %v", snapshotTime, snapshotID)
	}

	size, _ := util.ConvertSize(sourceVolumeSize)
	size = util.RoundUpSize(size)
	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      size,
			SnapshotId:     snapshotID,
			SourceVolumeId: sourceVolumeName,
			CreationTime:   creationTime,
			ReadyToUse:     readyToUse,
		},
	}
}

func encodeSnapshotID(csiSnapshotType, sourceVolumeName, id string) string {
	csiSnapshotType = normalizeCSISnapshotType(csiSnapshotType)
	if csiSnapshotType == csiSnapshotTypeLonghornSnapshot || csiSnapshotType == csiSnapshotTypeLonghornBackup {
		return fmt.Sprintf("%s://%s/%s", csiSnapshotType, sourceVolumeName, id)
	}
	// If the csiSnapshotType is invalid, pass through the id
	return id
}

func decodeSnapshotID(snapshotID string) (csiSnapshotType, sourceVolumeName, id string) {
	split := strings.Split(snapshotID, "://")
	if len(split) < 2 {
		return "", "", ""
	}
	csiSnapshotType = split[0]
	if normalizeCSISnapshotType(csiSnapshotType) == csiSnapshotTypeLonghornBackingImage {
		return csiSnapshotTypeLonghornBackingImage, "", ""
	}

	split = strings.Split(split[1], "/")
	if len(split) < 2 {
		return "", "", ""
	}
	sourceVolumeName = split[0]
	id = split[1]
	return normalizeCSISnapshotType(csiSnapshotType), sourceVolumeName, id
}

func encodeSnapshotBackingImageID(id, exportType, volumeName string) string {
	return fmt.Sprintf("bi://backing?%v=%v&%v=%v&%v=%v&%v=%v",
		longhorn.BackingImageParameterName, id,
		longhorn.BackingImageParameterDataSourceType, longhorn.BackingImageDataSourceTypeExportFromVolume,
		longhorn.DataSourceTypeExportParameterExportType, exportType,
		longhorn.DataSourceTypeExportParameterVolumeName, volumeName,
	)
}

func decodeSnapshoBackingImageID(snapshotID string) (parameters map[string]string) {
	parameters = make(map[string]string)
	u, err := url.Parse(snapshotID)
	if err != nil {
		return
	}
	queries := u.Query()
	for key, value := range queries {
		parameters[key] = value[0]
	}
	return
}

// normalizeCSISnapshotType converts the deprecated CSISnapshotType to the its new value
func normalizeCSISnapshotType(cSISnapshotType string) string {
	if cSISnapshotType == deprecatedCSISnapshotTypeLonghornBackup {
		return csiSnapshotTypeLonghornBackup
	}
	return cSISnapshotType
}

func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	snapshotID := req.GetSnapshotId()
	if len(snapshotID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing snapshot id in request")
	}

	csiSnapshotType, sourceVolumeName, id := decodeSnapshotID(snapshotID)
	switch csiSnapshotType {
	case csiSnapshotTypeLonghornBackingImage:
		if err := cs.cleanupBackingImage(snapshotID); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	case csiSnapshotTypeLonghornSnapshot:
		if id == "" {
			return nil, status.Errorf(codes.NotFound, "volume source snapshot %v is not found", snapshotID)
		}
		if err := cs.cleanupSnapshot(sourceVolumeName, id); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	case csiSnapshotTypeLonghornBackup:
		if id == "" {
			return nil, status.Errorf(codes.NotFound, "volume source snapshot %v is not found", snapshotID)
		}
		if err := cs.cleanupBackupVolume(sourceVolumeName, id); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (cs *ControllerServer) cleanupBackingImage(snapshotID string) error {
	backingImageParameters := decodeSnapshoBackingImageID(snapshotID)
	backingImage, err := cs.apiClient.BackingImage.ById(backingImageParameters[longhorn.BackingImageParameterName])
	if err != nil {
		return err
	}
	if backingImage != nil {
		if err := cs.apiClient.BackingImage.Delete(backingImage); err != nil {
			return err
		}
	}
	return nil
}

func (cs *ControllerServer) cleanupSnapshot(sourceVolumeName, id string) error {
	volume, err := cs.apiClient.Volume.ById(sourceVolumeName)
	if err != nil {
		return err
	}
	if volume != nil {
		if _, err := cs.apiClient.Volume.ActionSnapshotCRDelete(volume, &longhornclient.SnapshotCRInput{
			Name: id,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (cs *ControllerServer) cleanupBackupVolume(sourceVolumeName, id string) error {
	backupVolumeName, backupName := sourceVolumeName, id
	backupVolume, err := cs.apiClient.BackupVolume.ById(backupVolumeName)
	if err != nil {
		return err
	}
	if backupVolume != nil && backupVolume.Name != "" {
		if _, err = cs.apiClient.BackupVolume.ActionBackupDelete(backupVolume, &longhornclient.BackupInput{
			Name: backupName,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (cs *ControllerServer) ListSnapshots(context.Context, *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "ControllerExpandVolume"})

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id missing in request")
	}

	if req.CapacityRange == nil {
		return nil, status.Error(codes.InvalidArgument, "capacity range missing in request")
	}
	requestedSize := req.CapacityRange.GetRequiredBytes()

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capacity missing in request")
	}
	isAccessModeMount := req.VolumeCapability.GetMount() != nil

	existVol, err := cs.apiClient.Volume.ById(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	if existVol == nil {
		return nil, status.Errorf(codes.NotFound, "volume %s missing", volumeID)
	}
	if len(existVol.Controllers) != 1 {
		return nil, status.Errorf(codes.InvalidArgument, "volume %s invalid controller count %v", volumeID, len(existVol.Controllers))
	}
	if existVol.State != string(longhorn.VolumeStateDetached) && existVol.State != string(longhorn.VolumeStateAttached) {
		return nil, status.Errorf(codes.FailedPrecondition, "volume %s invalid state %v for controller volume expansion", volumeID, existVol.State)
	}
	existingSize, err := strconv.ParseInt(existVol.Size, 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	isOnlineExpansion := existVol.State == string(longhorn.VolumeStateAttached)

	if existVol, err = cs.apiClient.Volume.ActionExpand(existVol, &longhornclient.ExpandInput{
		Size: strconv.FormatInt(requestedSize, 10),
	}); err != nil {
		// TODO: This manual error code parsing should be refactored once Longhorn API implements error code response
		// https://github.com/longhorn/longhorn/issues/1875
		if matched, _ := regexp.MatchString("failed to schedule .* more bytes to disk", err.Error()); matched {
			return nil, status.Errorf(codes.OutOfRange, err.Error())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	// kubernetes doesn't support volume shrinking and the csi spec specifies to return true
	// in the case where the current capacity is bigger or equal to the requested capacity
	// that's why we return the volumeSize below instead of the requested capacity
	volumeExpansionComplete := func(vol *longhornclient.Volume) bool {
		engineReady := false
		if len(vol.Controllers) > 0 {
			engine := vol.Controllers[0]
			engineSize, _ := strconv.ParseInt(engine.Size, 10, 64)
			engineReady = engineSize >= requestedSize && !engine.IsExpanding
		}
		size, _ := strconv.ParseInt(vol.Size, 10, 64)
		return size >= requestedSize && engineReady
	}

	// we wait for completion of the expansion, to ensure that longhorn and kubernetes state are in sync
	// should this time out kubernetes will retry the expansion call since the call is idempotent
	// we will exit early if the volume already has the requested size
	if !cs.waitForVolumeState(volumeID, "volume expansion", volumeExpansionComplete, false, false) {
		return nil, status.Errorf(codes.DeadlineExceeded, "volume %s expansion from existing capacity %v to requested capacity %v failed",
			volumeID, existingSize, requestedSize)
	}

	volumeSize, err := strconv.ParseInt(existVol.Size, 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	if !isOnlineExpansion {
		log.Info("Skip NodeExpandVolume since this is offline expansion, the filesystem resize will be handled by NodeStageVolume when there is a workload using the volume.")
	}
	if !isAccessModeMount {
		log.Info("Skip NodeExpandVolume since the current volume is access mode block")
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         volumeSize,
		NodeExpansionRequired: isAccessModeMount && isOnlineExpansion,
	}, nil
}

func (cs *ControllerServer) ControllerGetVolume(context.Context, *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// isVolumeAvailableOn checks that the volume is attached and that an engine is running on the requested node
func isVolumeAvailableOn(vol *longhornclient.Volume, node string) bool {
	return vol.State == string(longhorn.VolumeStateAttached) && isEngineOnNodeAvailable(vol, node)
}

// isVolumeUnavailableOn checks that the volume is not attached to the requested node
func isVolumeUnavailableOn(vol *longhornclient.Volume, node string) bool {
	isValidState := vol.State == string(longhorn.VolumeStateAttached) || vol.State == string(longhorn.VolumeStateDetached)
	return isValidState && !isEngineOnNodeAvailable(vol, node)
}

func isEngineOnNodeAvailable(vol *longhornclient.Volume, node string) bool {
	for _, controller := range vol.Controllers {
		if controller.HostId == node && controller.Endpoint != "" {
			return true
		}
	}

	return false
}

func isVolumeShareAvailable(vol *longhornclient.Volume) bool {
	return vol.AccessMode == string(longhorn.AccessModeReadWriteMany) &&
		vol.ShareState == string(longhorn.ShareManagerStateRunning) && vol.ShareEndpoint != ""
}

func (cs *ControllerServer) waitForVolumeState(volumeID string, stateDescription string,
	predicate func(vol *longhornclient.Volume) bool, notFoundRetry, notFoundReturn bool) bool {
	log := getLoggerForCSIControllerServer()
	log = log.WithFields(logrus.Fields{"function": "waitForVolumeState"})
	timer := time.NewTimer(timeoutAttachDetach)
	defer timer.Stop()
	timeout := timer.C

	ticker := time.NewTicker(tickAttachDetach)
	defer ticker.Stop()
	tick := ticker.C

	for {
		select {
		case <-timeout:
			log.Warnf("Timeout wTile waiting for volume %s state %s", volumeID, stateDescription)
			return false
		case <-tick:
			existVol, err := cs.apiClient.Volume.ById(volumeID)
			if err != nil {
				log.Warnf("Failed to get volume while waiting for volume %s state %s error %s", volumeID, stateDescription, err)
				continue
			}
			if existVol == nil {
				log.Warnf("Volume %s does not exist", volumeID)
				if notFoundRetry {
					continue
				}
				return notFoundReturn
			}
			if predicate(existVol) {
				return true
			}
		}
	}
}

// waitForBackupControllerSync returns the backup of the given snapshot of the given volume. It does not return until
// the backup controller has synced at least once (so the backup contains information we need). This function does not
// wait for the existence of a backup. If one doesn't exist, it returns without error immediately.
func (cs *ControllerServer) waitForBackupControllerSync(volumeName, snapshotName string) (*longhornclient.Backup, error) {
	// Don't wait if we don't need to.
	backup, err := cs.getBackup(volumeName, snapshotName)
	if err != nil {
		return nil, err
	}
	if backup.SnapshotCreated != "" {
		// The backup controller sets the snapshot creation time at first sync. If we do not wait to return until
		// this is done, we may see timestamp related errors in csi-snapshotter logs.
		return backup, nil
	}

	timer := time.NewTimer(timeoutBackupControllerSync)
	defer timer.Stop()
	timeout := timer.C

	ticker := time.NewTicker(tickBackupControllerSync)
	defer ticker.Stop()
	tick := ticker.C

	for {
		select {
		case <-timeout:
			msg := fmt.Sprintf("waitForBackupControllerSync: timeout while waiting for backup controller to sync for volume %s and snapshot %s", volumeName, snapshotName)
			logrus.Warn(msg)
			return nil, status.Error(codes.DeadlineExceeded, msg)
		case <-tick:
			backup, err := cs.getBackup(volumeName, snapshotName)
			if err != nil {
				return nil, err
			}
			if backup.SnapshotCreated != "" {
				return backup, nil
			}
		}
	}
}

// getBackup looks for all backups associated with the given backupVolume or volume and returns the one for the given
// snapshot. It does not rely on volume.BackupStatus because volume.BackupStatus.Snapshot is only set if the backup
// successfully initializes and after the backup monitor has had a chance to sync. We want to retrieve the backup
// (and in particular, its name) as quickly as possible and in any state.
func (cs *ControllerServer) getBackup(volumeName, snapshotName string) (*longhornclient.Backup, error) {
	// Successfully returns an empty BackupVolume with volumeName even if one doesn't exist.
	backupVolume, err := cs.apiClient.BackupVolume.ById(volumeName)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	backupListOutput, err := cs.apiClient.BackupVolume.ActionBackupList(backupVolume)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	var backup *longhornclient.Backup
	for _, b := range backupListOutput.Data {
		if b.SnapshotName == snapshotName {
			backup = &b
			break
		}
	}

	return backup, nil
}

func (cs *ControllerServer) validateVolumeCapabilities(volumeCaps []*csi.VolumeCapability) error {
	if volumeCaps == nil {
		return status.Error(codes.InvalidArgument, "Volume Capabilities cannot be empty")
	}

	for _, cap := range volumeCaps {
		if cap.GetMount() == nil && cap.GetBlock() == nil {
			return status.Error(codes.InvalidArgument, "failed to have both mount and block access type be undefined")
		}
		if cap.GetMount() != nil && cap.GetBlock() != nil {
			return status.Error(codes.InvalidArgument, "failed to have both block and mount access type")
		}

		supportedMode := false
		for _, m := range cs.accessModes {
			if cap.GetAccessMode().GetMode() == m.GetMode() {
				supportedMode = true
				break
			}
		}
		if !supportedMode {
			return status.Errorf(codes.InvalidArgument, "access mode %v is not supported", cap.GetAccessMode().Mode.String())
		}
	}

	return nil
}

func getControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) []*csi.ControllerServiceCapability {
	var cscs []*csi.ControllerServiceCapability

	for _, cap := range cl {
		logrus.Infof("Enabling controller service capability: %v", cap.String())
		cscs = append(cscs, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		})
	}

	return cscs
}

func getVolumeCapabilityAccessModes(vc []csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability_AccessMode {
	var vca []*csi.VolumeCapability_AccessMode
	for _, c := range vc {
		logrus.Infof("Enabling volume access mode: %v", c.String())
		vca = append(vca, &csi.VolumeCapability_AccessMode{Mode: c})
	}
	return vca
}

func toProtoTimestamp(s string) (*timestamp.Timestamp, error) {
	t, err := util.ParseTimeZ(s)
	if err != nil {
		return nil, err
	}

	return ptypes.TimestampProto(t)
}

func (cs *ControllerServer) waitForSnapshotToBeReady(snapshotName, volumeName string) (*longhornclient.SnapshotCR, error) {
	timer := time.NewTimer(timeoutSnapshotCreation)
	defer timer.Stop()
	timeout := timer.C

	ticker := time.NewTicker(tickSnapshotCreation)
	defer ticker.Stop()
	tick := ticker.C

	var existSnapshotCR *longhornclient.SnapshotCR
	for {
		select {
		case <-timeout:
			if existSnapshotCR != nil && existSnapshotCR.Error != "" {
				return nil, fmt.Errorf("waitForSnapshotToBeReady: timeout while waiting for snapshot %v to be ready: %v", snapshotName, existSnapshotCR.Error)
			}
			return nil, fmt.Errorf("waitForSnapshotToBeReady: timeout while waiting for snapshot %v to be ready", snapshotName)
		case <-tick:
			vol, err := cs.apiClient.Volume.ById(volumeName)
			if err != nil {
				return nil, fmt.Errorf("waitForSnapshotToBeReady: fail to get source volume %v: %v", volumeName, err)
			}
			if vol == nil {
				return nil, fmt.Errorf("waitForSnapshotToBeReady: volume %s not found", volumeName)
			}
			existSnapshotCR, err = cs.apiClient.Volume.ActionSnapshotCRGet(vol, &longhornclient.SnapshotCRInput{
				Name: snapshotName,
			})
			if err != nil {
				return nil, fmt.Errorf("waitForSnapshotToBeReady: error while waiting for snapshot %v to be ready: %v", snapshotName, err)
			}
			if existSnapshotCR.ReadyToUse {
				return existSnapshotCR, nil
			}
		}
	}
}
