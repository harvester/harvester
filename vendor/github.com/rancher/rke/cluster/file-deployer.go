package cluster

import (
	"context"
	"fmt"
	"path"

	"github.com/docker/docker/api/types/container"
	"github.com/rancher/rke/docker"
	"github.com/rancher/rke/hosts"
	"github.com/rancher/rke/log"
	v3 "github.com/rancher/rke/types"
	"github.com/rancher/rke/util"
	"github.com/sirupsen/logrus"
)

const (
	ContainerName = "file-deployer"
	ServiceName   = "file-deploy"
	ConfigEnv     = "FILE_DEPLOY"
)

func deployFile(ctx context.Context, uniqueHosts []*hosts.Host, alpineImage string, prsMap map[string]v3.PrivateRegistry, fileName, fileContents, k8sVersion string) error {
	for _, host := range uniqueHosts {
		log.Infof(ctx, "[%s] Deploying file [%s] to node [%s]", ServiceName, fileName, host.Address)
		if err := doDeployFile(ctx, host, fileName, fileContents, alpineImage, prsMap, k8sVersion); err != nil {
			return fmt.Errorf("[%s] Failed to deploy file [%s] on node [%s]: %v", ServiceName, fileName, host.Address, err)
		}
	}
	return nil
}

func doDeployFile(ctx context.Context, host *hosts.Host, fileName, fileContents, alpineImage string, prsMap map[string]v3.PrivateRegistry, k8sVersion string) error {
	// remove existing container. Only way it's still here is if previous deployment failed
	if err := docker.DoRemoveContainer(ctx, host.DClient, ContainerName, host.Address); err != nil {
		return err
	}
	var cmd, containerEnv []string

	// fileContents determines if a file is placed or removed
	// exception to this is the cloud-config file, as it is valid being empty (for example, when only specifying the aws cloudprovider and no additional config)
	if fileContents != "" || fileName == cloudConfigFileName {
		containerEnv = []string{ConfigEnv + "=" + fileContents}
		cmd = []string{
			"sh",
			"-c",
			fmt.Sprintf("t=$(mktemp); echo -e \"$%s\" > $t && mv $t %s && chmod 600 %s", ConfigEnv, fileName, fileName),
		}
	} else {
		cmd = []string{
			"sh",
			"-c",
			fmt.Sprintf("rm -f %s", fileName),
		}
	}

	imageCfg := &container.Config{
		Image: alpineImage,
		Cmd:   cmd,
		Env:   containerEnv,
	}

	matchedRange, err := util.SemVerMatchRange(k8sVersion, util.SemVerK8sVersion122OrHigher)
	if err != nil {
		return err
	}
	hostCfg := &container.HostConfig{}
	// Rewrite SELinux labels (:z) is the default
	binds := []string{
		fmt.Sprintf("%s:/etc/kubernetes:z", path.Join(host.PrefixPath, "/etc/kubernetes")),
	}
	// Do not rewrite SELinux labels if k8s version is 1.22
	if matchedRange {
		binds = []string{
			fmt.Sprintf("%s:/etc/kubernetes", path.Join(host.PrefixPath, "/etc/kubernetes")),
		}
		// If SELinux is enabled, configure SELinux label
		if hosts.IsDockerSELinuxEnabled(host) {
			// We configure the label because we do not rewrite SELinux labels anymore on volume mounts (no :z)
			logrus.Debugf("Configuring security opt label [%s] for [%s] container on host [%s]", SELinuxLabel, ContainerName, host.Address)
			hostCfg.SecurityOpt = append(hostCfg.SecurityOpt, SELinuxLabel)
		}
	}
	hostCfg.Binds = binds

	if err := docker.DoRunOnetimeContainer(ctx, host.DClient, imageCfg, hostCfg, ContainerName, host.Address, ServiceName, prsMap); err != nil {
		return err
	}
	if err := docker.DoRemoveContainer(ctx, host.DClient, ContainerName, host.Address); err != nil {
		return err
	}
	logrus.Debugf("[%s] Successfully deployed file [%s] on node [%s]", ServiceName, fileName, host.Address)
	return nil
}
