package image

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/longhorn/backupstore"
)

const (
	metadataFolderPath          = "harvester/vmimages/"
	imageMetadataControllerName = "harvester-vmimage-metadata-controller"
)

var (
	FileNotFoundErr = errors.New("file not found")
)

type VirtualMachineImageMetadata struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	Description      string `json:"description,omitempty"`
	DisplayName      string `json:"displayName"`
	SourceType       string `json:"sourceType"`
	PVCName          string `json:"pvcName,omitempty"`
	PVCNamespace     string `json:"pvcNamespace,omitempty"`
	URL              string `json:"url,omitempty"`
	Checksum         string `json:"checksum,omitempty"`
	Size             int64  `json:"size,omitempty"`
	StorageClassName string `json:"storageClassName,omitempty"`
}

func loadVMImageMetadataInBackupTarget(filePath string, bsDriver backupstore.BackupStoreDriver) (*VirtualMachineImageMetadata, error) {
	if !bsDriver.FileExists(filePath) {
		return nil, fmt.Errorf("cannot find %s in backupstore", filePath)
	}

	rc, err := bsDriver.Read(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s in backupstore, err: %w", filePath, err)
	}
	defer rc.Close()

	backupMetadata := &VirtualMachineImageMetadata{}
	if err := json.NewDecoder(rc).Decode(backupMetadata); err != nil {
		return nil, fmt.Errorf("cannot decode content in %s, err: %w", filePath, err)
	}
	return backupMetadata, nil
}

func getVMImageMetadataFilePath(image harvesterv1.VirtualMachineImage) string {
	return filepath.Join(metadataFolderPath, fmt.Sprintf("%s-%s-%s/vmimage.yaml", image.Namespace, image.Name, image.Spec.DisplayName))
}

func getVMImageISOFilePath(image harvesterv1.VirtualMachineImage) string {
	return filepath.Join(metadataFolderPath, fmt.Sprintf("%s-%s-%s/%s", image.Namespace, image.Name, image.Spec.DisplayName, image.Spec.DisplayName))
}
