package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"

	"github.com/rancher/rke/docker"
	"github.com/rancher/rke/hosts"
	v3 "github.com/rancher/rke/types"
	"github.com/rancher/rke/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	SELinuxCheckContainer = "rke-selinux-checker"
)

func (c *Cluster) RunSELinuxCheck(ctx context.Context) error {
	// We only need to check this on k8s 1.22 and higher
	matchedRange, err := util.SemVerMatchRange(c.Version, util.SemVerK8sVersion122OrHigher)
	if err != nil {
		return err
	}

	if matchedRange {
		var errgrp errgroup.Group
		allHosts := hosts.GetUniqueHostList(c.EtcdHosts, c.ControlPlaneHosts, c.WorkerHosts)
		hostsQueue := util.GetObjectQueue(allHosts)
		for w := 0; w < WorkerThreads; w++ {
			errgrp.Go(func() error {
				var errList []error
				for host := range hostsQueue {
					if hosts.IsDockerSELinuxEnabled(host.(*hosts.Host)) {
						err := checkSELinuxLabelOnHost(ctx, host.(*hosts.Host), c.SystemImages.Alpine, c.PrivateRegistriesMap)
						if err != nil {
							errList = append(errList, err)
						}
					}
				}
				return util.ErrList(errList)
			})
		}
		if err := errgrp.Wait(); err != nil {
			return err
		}
	}
	return nil
}

func checkSELinuxLabelOnHost(ctx context.Context, host *hosts.Host, image string, prsMap map[string]v3.PrivateRegistry) error {
	var err error
	imageCfg := &container.Config{
		Image: image,
	}
	hostCfg := &container.HostConfig{
		SecurityOpt: []string{SELinuxLabel},
	}
	for retries := 0; retries < 3; retries++ {
		logrus.Infof("[selinux] Checking if host [%s] recognizes SELinux label [%s], try #%d", host.Address, SELinuxLabel, retries+1)
		if err = docker.DoRemoveContainer(ctx, host.DClient, SELinuxCheckContainer, host.Address); err != nil {
			return err
		}
		if err = docker.DoRunOnetimeContainer(ctx, host.DClient, imageCfg, hostCfg, SELinuxCheckContainer, host.Address, "selinux", prsMap); err != nil {
			// If we hit the error that indicates that the rancher-selinux RPM package is not installed (SELinux label is not recognized), we immediately return
			// Else we keep trying as there might be an error with Docker (slow system for example)
			if strings.Contains(err.Error(), "invalid argument") {
				return fmt.Errorf("[selinux] Host [%s] does not recognize SELinux label [%s]. This is required for Kubernetes version [%s]. Please install rancher-selinux RPM package and try again", host.Address, SELinuxLabel, util.SemVerK8sVersion122OrHigher)
			}
			continue
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("[selinux] Host [%s] was not able to correctly perform SELinux label check: [%v]", host.Address, err)
	}
	return nil
}
