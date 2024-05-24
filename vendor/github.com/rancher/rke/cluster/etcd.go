package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/rancher/rke/docker"
	"github.com/rancher/rke/hosts"
	"github.com/rancher/rke/log"
	"github.com/rancher/rke/pki"
	"github.com/rancher/rke/services"
	"github.com/rancher/rke/util"
	"golang.org/x/sync/errgroup"
)

const MinEtcdVersionWithDistrolessImage = "v3.5.7"

func (c *Cluster) SnapshotEtcd(ctx context.Context, snapshotName string) error {
	backupImage := c.getBackupImage()
	containerTimeout := DefaultEtcdBackupConfigTimeout
	if c.Services.Etcd.BackupConfig != nil && c.Services.Etcd.BackupConfig.Timeout > 0 {
		containerTimeout = c.Services.Etcd.BackupConfig.Timeout
	}

	// store first error message
	var snapshotErr error
	snapshotFailures := 0
	s3UploadFailures := 0

	for _, host := range c.EtcdHosts {
		newCtx := context.WithValue(ctx, docker.WaitTimeoutContextKey, containerTimeout)
		if err := services.RunEtcdSnapshotSave(newCtx, host, c.PrivateRegistriesMap, backupImage, snapshotName, true, c.Services.Etcd, c.Version); err != nil {
			if strings.Contains(err.Error(), "failed to upload etcd snapshot file to s3 on host") {
				s3UploadFailures++
			} else {
				if snapshotErr == nil {
					snapshotErr = err
				}
				snapshotFailures++
			}
		}
	}

	if snapshotFailures == len(c.EtcdHosts) {
		log.Warnf(ctx, "[etcd] Failed to take snapshot on all etcd hosts: %s", snapshotErr)
		return fmt.Errorf("[etcd] Failed to take snapshot on all etcd hosts: %s", snapshotErr)
	} else if snapshotFailures > 0 {
		log.Warnf(ctx, "[etcd] Failed to take snapshot on %s etcd hosts", snapshotFailures)
	} else {
		log.Infof(ctx, "[etcd] Finished saving snapshot [%s] on all etcd hosts", snapshotName)
	}

	if c.Services.Etcd.BackupConfig != nil && c.Services.Etcd.BackupConfig.S3BackupConfig == nil {
		return nil
	}

	if s3UploadFailures >= len(c.EtcdHosts)-snapshotFailures {
		log.Warnf(ctx, "[etcd] Failed to upload etcd snapshot file to s3 on all etcd hosts")
		return fmt.Errorf("[etcd] Failed to upload etcd snapshot file to s3 on all etcd hosts")
	} else if s3UploadFailures > 0 {
		log.Warnf(ctx, "[etcd] Failed to upload etcd snapshot file to s3 on %s etcd hosts", s3UploadFailures)
	} else {
		log.Infof(ctx, "[etcd] Finished uploading etcd snapshot file to s3 on all etcd hosts")
	}

	return nil
}

func (c *Cluster) DeployRestoreCerts(ctx context.Context, clusterCerts map[string]pki.CertificatePKI) error {
	var errgrp errgroup.Group
	hostsQueue := util.GetObjectQueue(c.EtcdHosts)
	restoreCerts := map[string]pki.CertificatePKI{}
	for _, n := range []string{pki.CACertName, pki.KubeNodeCertName, pki.KubeNodeCertName} {
		restoreCerts[n] = clusterCerts[n]
	}
	for w := 0; w < WorkerThreads; w++ {
		errgrp.Go(func() error {
			var errList []error
			for host := range hostsQueue {
				h := host.(*hosts.Host)
				var env []string
				if h.IsWindows() {
					env = c.getWindowsEnv(h)
				}
				if err := pki.DeployCertificatesOnPlaneHost(
					ctx,
					h,
					c.RancherKubernetesEngineConfig,
					restoreCerts,
					c.SystemImages.CertDownloader,
					c.PrivateRegistriesMap,
					false,
					env,
					c.Version); err != nil {
					errList = append(errList, err)
				}
			}
			return util.ErrList(errList)
		})
	}
	return errgrp.Wait()
}

func (c *Cluster) DeployStateFile(ctx context.Context, stateFilePath, snapshotName string) error {
	stateFileExists, err := util.IsFileExists(stateFilePath)
	if err != nil {
		logrus.Warnf("Could not read cluster state file from [%s], error: [%v]. Snapshot will be created without cluster state file. You can retrieve the cluster state file using 'rke util get-state-file'", stateFilePath, err)
		return nil
	}
	if !stateFileExists {
		logrus.Warnf("Could not read cluster state file from [%s], file does not exist. Snapshot will be created without cluster state file. You can retrieve the cluster state file using 'rke util get-state-file'", stateFilePath)
		return nil
	}

	var errgrp errgroup.Group
	hostsQueue := util.GetObjectQueue(c.EtcdHosts)
	for w := 0; w < WorkerThreads; w++ {
		errgrp.Go(func() error {
			var errList []error
			for host := range hostsQueue {
				err := pki.DeployStateOnPlaneHost(ctx, host.(*hosts.Host), c.SystemImages.CertDownloader, c.PrivateRegistriesMap, stateFilePath, snapshotName, c.Version)
				if err != nil {
					errList = append(errList, err)
				}
			}
			return util.ErrList(errList)
		})
	}
	return errgrp.Wait()
}

func (c *Cluster) GetStateFileFromSnapshot(ctx context.Context, snapshotName string) (string, error) {
	backupImage := c.getBackupImage()
	for _, host := range c.EtcdHosts {
		stateFile, err := services.RunGetStateFileFromSnapshot(ctx, host, c.PrivateRegistriesMap, backupImage, snapshotName, c.Services.Etcd, c.Version)
		if err != nil || stateFile == "" {
			logrus.Infof("Could not extract state file from snapshot [%s] on host [%s]", snapshotName, host.Address)
			continue
		}
		return stateFile, nil
	}
	return "", fmt.Errorf("Unable to find statefile in snapshot [%s]", snapshotName)
}

func (c *Cluster) PrepareBackup(ctx context.Context, snapshotPath string) error {
	// local backup case
	var backupReady bool
	var backupServer *hosts.Host
	backupImage := c.getBackupImage()
	var errors []error
	// s3 backup case
	if c.Services.Etcd.BackupConfig != nil &&
		c.Services.Etcd.BackupConfig.S3BackupConfig != nil {
		log.Infof(ctx, "[etcd] etcd s3 backup configuration found, will use s3 as source")
		downloadFailed := false
		for _, host := range c.EtcdHosts {
			if err := services.DownloadEtcdSnapshotFromS3(ctx, host, c.PrivateRegistriesMap, backupImage, snapshotPath, c.Services.Etcd, c.Version); err != nil {
				log.Warnf(ctx, "failed to download snapshot [%s] from s3 on host [%s]: %v", snapshotPath, host.Address, err)
				downloadFailed = true
				break
			}
		}
		backupReady = !downloadFailed
	}
	// legacy rke local backup or rancher local backup
	if !backupReady {
		if c.Services.Etcd.BackupConfig == nil {
			log.Infof(ctx, "[etcd] No etcd snapshot configuration found, will use local as source")
		} else if c.Services.Etcd.BackupConfig.S3BackupConfig == nil {
			log.Infof(ctx, "[etcd] etcd snapshot configuration found and no s3 backup configuration found, will use local as source")
		} else {
			log.Warnf(ctx, "[etcd] etcd snapshot configuration found and s3 backup configuration failed, falling back to use local as source")
		}
		// stop etcd on all etcd nodes, we need this because we start the backup server on the same port
		for _, host := range c.EtcdHosts {
			if err := docker.StopContainer(ctx, host.DClient, host.Address, services.EtcdContainerName); err != nil {
				log.Warnf(ctx, "failed to stop etcd container on host [%s]: %v", host.Address, err)
			}
			// start the download server, only one node should have it!
			if err := services.StartBackupServer(ctx, host, c.PrivateRegistriesMap, backupImage, snapshotPath, c.Version); err != nil {
				log.Warnf(ctx, "failed to start backup server on host [%s]: %v", host.Address, err)
				errors = append(errors, err)
				continue
			}
			backupServer = host
			break
		}

		if backupServer == nil { //failed to start the backupServer, I will cleanup and exit
			for _, host := range c.EtcdHosts {
				if err := docker.StartContainer(ctx, host.DClient, host.Address, services.EtcdContainerName); err != nil {
					log.Warnf(ctx, "failed to start etcd container on host [%s]: %v", host.Address, err)
				}
			}
			return fmt.Errorf("failed to start backup server on all etcd nodes: %v", errors)
		}
		// start downloading the snapshot
		for _, host := range c.EtcdHosts {
			if host.Address == backupServer.Address { // we skip the backup server if it's there
				continue
			}
			if err := services.DownloadEtcdSnapshotFromBackupServer(ctx, host, c.PrivateRegistriesMap, backupImage, snapshotPath, backupServer, c.Version); err != nil {
				return err
			}
		}
		// all good, let's remove the backup server container
		if err := docker.DoRemoveContainer(ctx, backupServer.DClient, services.EtcdServeBackupContainerName, backupServer.Address); err != nil {
			return err
		}
		backupReady = true
	}

	if !backupReady {
		return fmt.Errorf("failed to prepare backup for restore")
	}
	// this applies to all cases!
	if isEqual := c.etcdSnapshotChecksum(ctx, snapshotPath); !isEqual {
		return fmt.Errorf("etcd snapshots are not consistent")
	}
	return nil
}

func (c *Cluster) RestoreEtcdSnapshot(ctx context.Context, snapshotPath string) error {
	// Start restore process on all etcd hosts
	initCluster := services.GetEtcdInitialCluster(c.EtcdHosts)
	backupImage := c.getBackupImage()
	restoreImage := c.getRestoreImage()
	for _, host := range c.EtcdHosts {
		containerTimeout := DefaultEtcdBackupConfigTimeout
		if c.Services.Etcd.BackupConfig != nil && c.Services.Etcd.BackupConfig.Timeout > 0 {
			containerTimeout = c.Services.Etcd.BackupConfig.Timeout
		}
		newCtx := context.WithValue(ctx, docker.WaitTimeoutContextKey, containerTimeout)
		if err := services.RestoreEtcdSnapshot(newCtx, host, c.PrivateRegistriesMap, restoreImage, backupImage,
			snapshotPath, initCluster, c.Services.Etcd, c.Version); err != nil {
			return fmt.Errorf("[etcd] Failed to restore etcd snapshot: %v", err)
		}
	}
	return nil
}

func (c *Cluster) RemoveEtcdSnapshot(ctx context.Context, snapshotName string) error {
	backupImage := c.getBackupImage()
	for _, host := range c.EtcdHosts {
		if err := services.RunEtcdSnapshotRemove(ctx, host, c.PrivateRegistriesMap, backupImage, snapshotName,
			false, c.Services.Etcd, c.Version); err != nil {
			return err
		}
	}
	return nil
}

func (c *Cluster) etcdSnapshotChecksum(ctx context.Context, snapshotPath string) bool {
	log.Infof(ctx, "[etcd] Checking if all snapshots are identical")
	etcdChecksums := []string{}
	backupImage := c.getBackupImage()

	for _, etcdHost := range c.EtcdHosts {
		checksum, err := services.GetEtcdSnapshotChecksum(ctx, etcdHost, c.PrivateRegistriesMap, backupImage, snapshotPath, c.Version)
		if err != nil {
			return false
		}
		etcdChecksums = append(etcdChecksums, checksum)
		log.Infof(ctx, "[etcd] Checksum of etcd snapshot on host [%s] is [%s]", etcdHost.Address, checksum)
	}
	hostChecksum := etcdChecksums[0]
	for _, checksum := range etcdChecksums {
		if checksum != hostChecksum {
			return false
		}
	}
	return true
}

func (c *Cluster) getBackupImage() string {
	rkeToolsImage, err := util.GetDefaultRKETools(c.SystemImages.Alpine)
	if err != nil {
		logrus.Errorf("[etcd] error getting backup image %v", err)
		return ""
	}
	logrus.Debugf("[etcd] Image used for etcd snapshot is: [%s]", rkeToolsImage)
	return rkeToolsImage
}

func (c *Cluster) getRestoreImage() string {

	// use etcd image for restore in case of custom system image
	if !strings.Contains(c.SystemImages.Etcd, "rancher/mirrored-coreos-etcd") {
		return c.SystemImages.Etcd
	}

	etcdImageTag, err := util.GetImageTagFromImage(c.SystemImages.Etcd)
	if err != nil {
		logrus.Errorf("[etcd] getRestoreImage: error extracting tag from etcd image: %v", err)
		return ""
	}

	etcdVersion, err := util.StrToSemVer(etcdImageTag)
	if err != nil {
		logrus.Errorf("[etcd] getRestoreImage: error converting etcd image tag to semver: %v", err)
		return ""
	}

	minEtcdVersionWithDistrolessImage, err := util.StrToSemVer(MinEtcdVersionWithDistrolessImage)
	if err != nil {
		logrus.Errorf("[etcd] getRestoreImage: error converting min distroless etcd image version to semver: %v", err)
		return ""
	}

	if etcdVersion.LessThan(*minEtcdVersionWithDistrolessImage) {
		return c.SystemImages.Etcd
	}

	return c.getBackupImage()
}
