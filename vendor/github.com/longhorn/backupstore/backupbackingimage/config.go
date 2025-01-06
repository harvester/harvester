package backupbackingimage

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/backupstore"
	"github.com/longhorn/backupstore/common"
	"github.com/longhorn/backupstore/util"
)

const (
	BackingImageBlockSeparateLayer1 = 2
	BackingImageBlockSeparateLayer2 = 4

	BackingImageDirectory  = "backing-images"
	BackingImageConfigFile = "backing-image.cfg"

	BlocksDirectory = "blocks"
	BlkSuffix       = ".blk"
)

func addBackingImageConfigInBackupStore(driver backupstore.BackupStoreDriver, backupBackingImage *BackupBackingImage) (bool, error) {
	log := backupstore.GetLog().WithFields(logrus.Fields{"type": BackingImageLogType, "name": backupBackingImage.Name})

	if backingImageExists(driver, backupBackingImage.Name) {
		return true, nil
	}

	if !util.ValidateName(backupBackingImage.Name) {
		return false, fmt.Errorf("invalid backing image name %v", backupBackingImage.Name)
	}

	if err := saveBackingImageConfig(driver, backupBackingImage); err != nil {
		return false, errors.Wrap(err, "failed to add backing image config to backupstore")
	}

	log.Info("Added backing image config to backupstore")
	return false, nil
}

func removeBackupBackingImage(backupBackingImage *BackupBackingImage, driver backupstore.BackupStoreDriver) error {
	log := backupstore.GetLog().WithFields(logrus.Fields{"type": BackingImageLogType, "name": backupBackingImage.Name})

	filePath := getBackingImageFilePath(backupBackingImage.Name)
	if err := driver.Remove(filePath); err != nil {
		return err
	}
	log.Infof("Removed backing image on backupstore with filePath: %v", filePath)
	return nil
}

func backingImageExists(driver backupstore.BackupStoreDriver, backingImageName string) bool {
	return driver.FileExists(getBackingImageFilePath(backingImageName))
}

func getBackingImageFilePath(backingImageName string) string {
	backingImagePath := getBackingImagePath(backingImageName)
	backingImageCfg := BackingImageConfigFile
	return filepath.Join(backingImagePath, backingImageCfg)
}

func getBackingImagePath(backingImageName string) string {
	return filepath.Join(backupstore.GetBackupstoreBase(), BackingImageDirectory, BackingImageDirectory, backingImageName) + "/"
}

func saveBackingImageConfig(driver backupstore.BackupStoreDriver, backupBackingImage *BackupBackingImage) error {
	return backupstore.SaveConfigInBackupStore(driver, getBackingImageFilePath(backupBackingImage.Name), backupBackingImage)
}

func loadBackingImageConfigInBackupStore(driver backupstore.BackupStoreDriver, backingImageName string) (*BackupBackingImage, error) {
	log := backupstore.GetLog()
	backupBackingImage := &BackupBackingImage{}
	path := getBackingImageFilePath(backingImageName)
	if err := backupstore.LoadConfigInBackupStore(driver, path, backupBackingImage); err != nil {
		return nil, err
	}
	if backupBackingImage.CompressionMethod == "" {
		log.Infof("Fall back compression method to %v for backing image %v", backupstore.LEGACY_COMPRESSION_METHOD, backupBackingImage.Name)
		backupBackingImage.CompressionMethod = backupstore.LEGACY_COMPRESSION_METHOD
	}

	if backupBackingImage.Blocks == nil {
		backupBackingImage.Blocks = []common.BlockMapping{}
	}
	if backupBackingImage.ProcessingBlocks == nil {
		backupBackingImage.ProcessingBlocks = &common.ProcessingBlocks{
			Blocks: map[string][]*common.BlockMapping{},
		}
	}

	return backupBackingImage, nil
}

func getTotalBackupBlockCounts(mappings *common.Mappings) (int64, error) {
	totalBlockCounts := int64(len(mappings.Mappings))
	return totalBlockCounts, nil
}

func getBackingImageBlockFilePath(checksum string) string {
	blockSubDirLayer1 := checksum[0:BackingImageBlockSeparateLayer1]
	blockSubDirLayer2 := checksum[BackingImageBlockSeparateLayer1:BackingImageBlockSeparateLayer2]
	path := filepath.Join(getBackingImageBlockPath(), blockSubDirLayer1, blockSubDirLayer2)
	fileName := checksum + BlkSuffix

	return filepath.Join(path, fileName)
}

func getBackingImageBlockPath() string {
	return filepath.Join(backupstore.GetBackupstoreBase(), BackingImageDirectory, BlocksDirectory) + "/"
}

func EncodeBackupBackingImageURL(backingImageName, destURL string) string {
	if destURL == "" || backingImageName == "" {
		return ""
	}

	u, err := url.Parse(destURL)
	if err != nil {
		log := backupstore.GetLog()
		log.WithError(err).Errorf("Failed to parse destURL %v", destURL)
		return ""
	}
	if u.Scheme == "" {
		return ""
	}

	v := url.Values{}
	v.Add("backingImage", backingImageName)
	prefixChar := "?"
	if strings.Contains(destURL, "?") {
		prefixChar = "&"
	}
	return destURL + prefixChar + v.Encode()
}

func DecodeBackupBackingImageURL(backupURL string) (string, string, error) {
	u, err := url.Parse(backupURL)
	if err != nil {
		return "", "", err
	}
	v := u.Query()
	backingImageName := v.Get("backingImage")
	if !util.ValidateName(backingImageName) {
		return "", "", fmt.Errorf("invalid backing image name parsed: %v", backingImageName)
	}
	u.RawQuery = ""
	destURL := u.String()
	return backingImageName, destURL, nil
}

func GetAllBackupBackingImageNames(driver backupstore.BackupStoreDriver) ([]string, error) {
	result := []string{}
	backingImageConfigBase := filepath.Join(backupstore.GetBackupstoreBase(), BackingImageDirectory, BackingImageDirectory) + "/"
	nameList, err := driver.List(backingImageConfigBase)
	if err != nil {
		return result, nil
	}
	return nameList, nil
}

func getAllBlockNames(driver backupstore.BackupStoreDriver) ([]string, error) {
	names := []string{}
	blockPathBase := getBackingImageBlockPath()
	lv1Dirs, err := driver.List(blockPathBase)
	// Directory doesn't exist
	if err != nil {
		return names, nil
	}
	for _, lv1 := range lv1Dirs {
		lv1Path := filepath.Join(blockPathBase, lv1)
		lv2Dirs, err := driver.List(lv1Path)
		if err != nil {
			return nil, err
		}
		for _, lv2 := range lv2Dirs {
			lv2Path := filepath.Join(lv1Path, lv2)
			blockNames, err := driver.List(lv2Path)
			if err != nil {
				return nil, err
			}
			names = append(names, blockNames...)
		}
	}

	return util.ExtractNames(names, "", BlkSuffix), nil
}

func isBackupInProgress(backupBackingImage *BackupBackingImage) bool {
	return backupBackingImage != nil && backupBackingImage.CompleteTime == ""
}

type BackupInfo struct {
	Name              string
	URL               string
	CompleteAt        string
	Size              int64 `json:",string"`
	Checksum          string
	Labels            map[string]string
	CompressionMethod string `json:",omitempty"`
	Secret            string
	SecretNamespace   string
}

func InspectBackupBackingImage(backupURL string) (*BackupInfo, error) {
	backupBackingImageName, destURL, err := DecodeBackupBackingImageURL(backupURL)
	if err != nil {
		return nil, err
	}

	bsDriver, err := backupstore.GetBackupStoreDriver(destURL)
	if err != nil {
		return nil, err
	}

	backupBackingImage, err := loadBackingImageConfigInBackupStore(bsDriver, backupBackingImageName)
	if err != nil {
		return nil, err
	} else if isBackupInProgress(backupBackingImage) {
		// for now we don't return in progress backup backing image to the ui
		return nil, fmt.Errorf("backup backing image %v is still in progress", backupBackingImage.Name)
	}

	return fillFullBackupBackingImageInfo(backupBackingImage, bsDriver.GetURL()), nil
}

func fillFullBackupBackingImageInfo(backupBackingImage *BackupBackingImage, destURL string) *BackupInfo {
	return &BackupInfo{
		Name:              backupBackingImage.Name,
		URL:               EncodeBackupBackingImageURL(backupBackingImage.Name, destURL),
		CompleteAt:        backupBackingImage.CompleteTime,
		Size:              backupBackingImage.Size,
		Checksum:          backupBackingImage.Checksum,
		Labels:            backupBackingImage.Labels,
		CompressionMethod: backupBackingImage.CompressionMethod,
		Secret:            backupBackingImage.Secret,
		SecretNamespace:   backupBackingImage.SecretNamespace,
	}
}
