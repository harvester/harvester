package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rancher/rke/cluster"
	"github.com/rancher/rke/hosts"
	"github.com/rancher/rke/k8s"
	"github.com/rancher/rke/pki"
	v3 "github.com/rancher/rke/types"
	"github.com/rancher/rke/util"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func UtilCommand() cli.Command {
	utilCfgFlags := []cli.Flag{
		cli.StringFlag{
			Name:   "config",
			Usage:  "Specify an alternate cluster YAML file",
			Value:  pki.ClusterConfig,
			EnvVar: "RKE_CONFIG",
		},
	}
	utilFlags := append(utilCfgFlags, commonFlags...)

	return cli.Command{
		Name:  "util",
		Usage: "Various utilities to retrieve cluster related files and troubleshoot",
		Subcommands: cli.Commands{
			cli.Command{
				Name:   "get-state-file",
				Usage:  "Retrieve state file from cluster",
				Action: getStateFile,
				Flags:  utilFlags,
			},
			cli.Command{
				Name:   "get-kubeconfig",
				Usage:  "Retrieve kubeconfig file from cluster state",
				Action: getKubeconfigFile,
				Flags:  utilFlags,
			},
		},
	}
}

func getKubeconfigFile(ctx *cli.Context) error {
	logrus.Infof("Creating new kubeconfig file")
	// Check if we can successfully connect to the cluster using the existing kubeconfig file
	clusterFile, clusterFilePath, err := resolveClusterFile(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve cluster file: %v", err)
	}

	// setting up the flags
	flags := cluster.GetExternalFlags(false, false, false, false, "", clusterFilePath)
	rkeConfig, err := cluster.ParseConfig(clusterFile)
	if err != nil {
		return fmt.Errorf("failed to parse cluster file: %v", err)
	}

	rkeConfig, err = setOptionsFromCLI(ctx, rkeConfig)
	if err != nil {
		return err
	}

	clusterState, err := cluster.ReadStateFile(context.Background(), cluster.GetStateFilePath(flags.ClusterFilePath, flags.ConfigDir))
	if err != nil {
		return err
	}

	// Creating temp cluster to check if snapshot archive contains state file and retrieve it
	tempCluster, err := cluster.InitClusterObject(context.Background(), rkeConfig, flags, "")
	if err != nil {
		return err
	}

	// Move current kubeconfig file
	err = util.CopyFileWithPrefix(tempCluster.LocalKubeConfigPath, "kube_config")
	if err != nil {
		return err
	}
	kubeCluster, _ := tempCluster.GetClusterState(context.Background(), clusterState)

	return cluster.RebuildKubeconfig(context.Background(), kubeCluster)
}

func getStateFile(ctx *cli.Context) error {
	logrus.Infof("Retrieving state file from cluster")
	// Check if we can successfully connect to the cluster using the existing kubeconfig file
	localKubeConfig := pki.GetLocalKubeConfig(ctx.String("config"), "")
	clusterFile, clusterFilePath, err := resolveClusterFile(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve cluster file: %v", err)
	}
	// setting up the flags
	flags := cluster.GetExternalFlags(false, false, false, false, "", clusterFilePath)

	// not going to use a k8s dialer here.. this is a CLI command
	serverVersion, err := cluster.GetK8sVersion(localKubeConfig, nil)
	if err != nil {
		logrus.Infof("Unable to connect to server using kubeconfig, trying to get state from Control Plane node(s), error: %v", err)
		// We need to retrieve the state file using Docker on the node(s)

		rkeConfig, err := cluster.ParseConfig(clusterFile)
		if err != nil {
			return fmt.Errorf("failed to parse cluster file: %v", err)
		}

		rkeConfig, err = setOptionsFromCLI(ctx, rkeConfig)
		if err != nil {
			return err
		}

		_, _, _, _, _, err = RetrieveClusterStateConfigMap(context.Background(), rkeConfig, hosts.DialersOptions{}, flags, map[string]interface{}{})
		if err != nil {
			return err
		}

		return nil
	}
	logrus.Infof("Successfully connected to server using kubeconfig, retrieved server version [%s]", serverVersion)
	// Retrieve full-cluster-state configmap
	k8sClient, err := k8s.NewClient(localKubeConfig, nil)
	if err != nil {
		return err
	}
	cfgMap, err := k8s.GetConfigMap(k8sClient, cluster.FullStateConfigMapName)
	if err != nil {
		return err
	}
	clusterData := cfgMap.Data[cluster.FullStateConfigMapName]
	rkeFullState := &cluster.FullState{}
	if err = json.Unmarshal([]byte(clusterData), rkeFullState); err != nil {
		return err
	}

	// Move current state file
	stateFilePath := cluster.GetStateFilePath(flags.ClusterFilePath, flags.ConfigDir)
	err = util.ReplaceFileWithBackup(stateFilePath, "rkestate")
	if err != nil {
		return err
	}

	// Write new state file
	err = rkeFullState.WriteStateFile(context.Background(), stateFilePath)
	if err != nil {
		return err
	}

	return nil
}

func RetrieveClusterStateConfigMap(
	ctx context.Context,
	rkeConfig *v3.RancherKubernetesEngineConfig,
	dialersOptions hosts.DialersOptions,
	flags cluster.ExternalFlags,
	data map[string]interface{}) (string, string, string, string, map[string]pki.CertificatePKI, error) {
	var APIURL, caCrt, clientCert, clientKey string

	rkeFullState := &cluster.FullState{}

	// Creating temp cluster to check if snapshot archive contains state file and retrieve it
	tempCluster, err := cluster.InitClusterObject(ctx, rkeConfig, flags, "")
	if err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}
	if err := tempCluster.SetupDialers(ctx, dialersOptions); err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}
	if err := tempCluster.TunnelHosts(ctx, flags); err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}
	// Get ConfigMap containing cluster state from Control Plane Hosts
	stateFile, err := tempCluster.GetStateFileFromConfigMap(ctx)

	if err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}
	rkeFullState, err = cluster.StringToFullState(ctx, stateFile)

	if err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}

	// Move current state file
	stateFilePath := cluster.GetStateFilePath(flags.ClusterFilePath, flags.ConfigDir)
	err = util.ReplaceFileWithBackup(stateFilePath, "rkestate")
	if err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}

	err = rkeFullState.WriteStateFile(context.Background(), stateFilePath)
	if err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}

	// Move current kubeconfig file
	err = util.CopyFileWithPrefix(tempCluster.LocalKubeConfigPath, "kube_config")
	if err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}
	kubeCluster, _ := tempCluster.GetClusterState(ctx, rkeFullState)

	if err := cluster.RebuildKubeconfig(ctx, kubeCluster); err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, nil
	}

	return APIURL, caCrt, clientCert, clientKey, nil, nil
}
