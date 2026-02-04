package systembackup

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/longhorn/backupstore"
	"github.com/longhorn/backupstore/util"
)

var (
	log = backupstore.GetLog()
)

const (
	SubDirectory = "system-backups"

	ConfigFile = "system-backup.cfg"
	ZipFile    = "system-backup.zip"
)

type Name string

type URI string

type SystemBackups map[Name]URI

type Config struct {
	Name              string
	LonghornVersion   string
	LonghornGitCommit string
	BackupTargetURL   string
	ManagerImage      string
	EngineImage       string
	CreatedAt         time.Time
	Checksum          string
}

func Upload(localFilePath string, cfg *Config) error {
	driver, err := backupstore.GetBackupStoreDriver(cfg.BackupTargetURL)
	if err != nil {
		return err
	}

	remoteBackupURI := getSystemBackupZipURI(cfg)
	err = backupstore.SaveLocalFileToBackupStore(localFilePath, remoteBackupURI, driver)
	if err != nil {
		return err
	}

	cfgURI := getSystemBackupConfigURI(cfg)
	if err := backupstore.SaveConfigInBackupStore(driver, cfgURI, cfg); err != nil {
		log.WithError(err).Errorf("Failed to upload system backup config %v", cfg.Name)
		return err
	}
	log.Infof("Uploaded system backup config %v", cfg.Name)

	return nil
}

func Download(localFilePath string, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("invalid empty system backup config")
	}

	driver, err := backupstore.GetBackupStoreDriver(cfg.BackupTargetURL)
	if err != nil {
		return err
	}

	remoteBackupURI := getSystemBackupZipURI(cfg)
	if !driver.FileExists(remoteBackupURI) {
		return fmt.Errorf("system backup %v doesn't exist", remoteBackupURI)
	}

	err = backupstore.SaveBackupStoreToLocalFile(driver, remoteBackupURI, localFilePath)
	if err != nil {
		return err
	}

	sha512sum, err := util.GetFileChecksum(localFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to get %v checksum", localFilePath)
	}

	if sha512sum != cfg.Checksum {
		if err := os.Remove(localFilePath); err != nil {
			log.WithError(err).Warnf("Failed to clean up local file %v", localFilePath)
		}
		return fmt.Errorf("downloaded %v checksum mismatched: got %v: expect %v", localFilePath, sha512sum, cfg.Checksum)
	}

	return nil
}

func List(destURL string) (SystemBackups, error) {
	driver, err := backupstore.GetBackupStoreDriver(destURL)
	if err != nil {
		return nil, err
	}

	systemBackups, err := getSystemBackups(driver)
	if err != nil {
		return nil, err
	}

	return systemBackups, nil
}

func Delete(cfg *Config) error {
	driver, err := backupstore.GetBackupStoreDriver(cfg.BackupTargetURL)
	if err != nil {
		return err
	}

	systemBackupURI := getSystemBackupURI(cfg.Name, cfg.LonghornVersion)
	if err := driver.Remove(systemBackupURI); err != nil {
		return err
	}

	log.Infof("Removed %v", systemBackupURI)
	return nil
}

func LoadConfig(name, longhornVersion, backupTargetURL string) (*Config, error) {
	driver, err := backupstore.GetBackupStoreDriver(backupTargetURL)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	cfgURI := getSystemBackupConfigURI(&Config{
		Name:            name,
		LonghornVersion: longhornVersion,
		BackupTargetURL: backupTargetURL,
	})
	if err := backupstore.LoadConfigInBackupStore(driver, cfgURI, config); err != nil {
		return nil, err
	}
	return config, nil
}

func getSystemBackupConfigURI(cfg *Config) string {
	systemBackupURI := getSystemBackupURI(cfg.Name, cfg.LonghornVersion)
	return filepath.Join(systemBackupURI, ConfigFile)
}

func getSystemBackupZipURI(cfg *Config) string {
	rootPath := getSystemBackupURI(cfg.Name, cfg.LonghornVersion)
	return filepath.Join(rootPath, ZipFile)
}

func GetSystemBackupURL(name, longhornVersion, backupTarget string) (string, error) {
	u, err := url.Parse(backupTarget)
	if err != nil {
		return "", err
	}

	if name == "" {
		return "", errors.New("getting system backup URL: missing system backup name")
	}

	if longhornVersion == "" {
		return "", errors.New("getting system backup URL: missing Longhorn version")
	}

	u.Path = filepath.Join(u.Path, getSystemBackupURI(name, longhornVersion))
	return u.String(), nil
}

func getSystemBackupURI(name, longhornVersion string) string {
	return filepath.Join(backupstore.GetBackupstoreBase(),
		SubDirectory, longhornVersion, name) + "/"
}

func getSystemBackups(driver backupstore.BackupStoreDriver) (SystemBackups, error) {
	systemBackupURI := filepath.Join(backupstore.GetBackupstoreBase(), SubDirectory)
	lv1Dirs, err := driver.List(systemBackupURI)
	if err != nil {
		log.WithError(err).Warnf("Failed to list 1st level directory %v", systemBackupURI)
		return nil, err
	}

	errs := []string{}
	systemBackups := SystemBackups{}
	for _, longhornVersion := range lv1Dirs {
		path := filepath.Join(systemBackupURI, longhornVersion)
		lv2Dirs, err := driver.List(path)
		if err != nil {
			log.WithError(err).Warnf("Failed to list 2nd level directory %v", path)
			return nil, err
		}

		for _, name := range lv2Dirs {
			systemBackups[Name(name)] = URI(filepath.Join(path, name))
		}
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "\n"))
	}
	return systemBackups, nil
}

// ParseSystemBackupURL parse the URL and return backup target, version and name.
// An example: s3://bucket@region/path/backupstore/system-backups/v1.4.0/sample-system-backup
// should return s3://bucket@region/path/, v1.4.0, sample-system-backup, nil
func ParseSystemBackupURL(systemBackupURL string) (backupTarget, version, name string, err error) {
	u, err := url.Parse(systemBackupURL)
	if err != nil {
		return "", "", "", err
	}

	split := strings.Split(u.Path, "/")
	if len(split) < 5 {
		return "", "", "", fmt.Errorf("invalid system backup URI: %v", u.Path)
	}

	subDir := split[len(split)-3]
	if subDir != SubDirectory {
		return "", "", "", fmt.Errorf("invalid system backup URL subdirectory %v: %v", subDir, systemBackupURL)
	}

	backupStoreDir := split[len(split)-4]
	if backupStoreDir != backupstore.GetBackupstoreBase() {
		return "", "", "", fmt.Errorf("invalid system backup URL backupstore directory %v: %v", backupStoreDir, systemBackupURL)
	}

	u.Path = strings.Join(split[:len(split)-4], "/")
	backupTarget = u.String()
	if strings.Contains(backupTarget, "@") {
		backupTarget = backupTarget + "/"
	}
	return backupTarget, split[len(split)-2], split[len(split)-1], nil
}
