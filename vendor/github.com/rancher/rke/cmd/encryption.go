package cmd

import (
	"context"
	"fmt"

	"github.com/rancher/rke/cluster"
	"github.com/rancher/rke/hosts"
	"github.com/rancher/rke/log"
	"github.com/rancher/rke/pki"
	"github.com/rancher/rke/pki/cert"
	v3 "github.com/rancher/rke/types"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func EncryptionCommand() cli.Command {
	encryptFlags := []cli.Flag{
		cli.StringFlag{
			Name:   "config",
			Usage:  "Specify an alternate cluster YAML file",
			Value:  pki.ClusterConfig,
			EnvVar: "RKE_CONFIG",
		},
	}
	encryptFlags = append(encryptFlags, commonFlags...)
	return cli.Command{
		Name:  "encrypt",
		Usage: "Manage cluster encryption provider keys",
		Subcommands: cli.Commands{
			cli.Command{
				Name:   "rotate-key",
				Usage:  "Rotate cluster encryption provider key",
				Action: rotateEncryptionKeyFromCli,
				Flags:  encryptFlags,
			},
		},
	}
}

func rotateEncryptionKeyFromCli(ctx *cli.Context) error {
	logrus.Infof("Running RKE version: %v", ctx.App.Version)
	clusterFile, filePath, err := resolveClusterFile(ctx)
	if err != nil {
		return fmt.Errorf("Failed to resolve cluster file: %v", err)
	}

	rkeConfig, err := cluster.ParseConfig(clusterFile)
	if err != nil {
		return fmt.Errorf("Failed to parse cluster file: %v", err)
	}

	rkeConfig, err = setOptionsFromCLI(ctx, rkeConfig)
	if err != nil {
		return err
	}

	// setting up the flags
	flags := cluster.GetExternalFlags(false, false, false, false, "", filePath)

	_, _, _, _, _, err = RotateEncryptionKey(context.Background(), rkeConfig, hosts.DialersOptions{}, flags)
	return err
}

func RotateEncryptionKey(
	ctx context.Context,
	rkeConfig *v3.RancherKubernetesEngineConfig,
	dialersOptions hosts.DialersOptions,
	flags cluster.ExternalFlags,
) (string, string, string, string, map[string]pki.CertificatePKI, error) {
	log.Infof(ctx, "Rotating cluster secrets encryption key")

	var APIURL, caCrt, clientCert, clientKey string

	stateFilePath := cluster.GetStateFilePath(flags.ClusterFilePath, flags.ConfigDir)
	rkeFullState, _ := cluster.ReadStateFile(ctx, stateFilePath)

	// We generate the first encryption config in ClusterInit, to store it ASAP. It's written to the DesiredState
	stateEncryptionConfig := rkeFullState.DesiredState.EncryptionConfig
	// if CurrentState has EncryptionConfig, it means this is NOT the first time we enable encryption, we should use the _latest_ applied value from the current cluster
	if rkeFullState.CurrentState.EncryptionConfig != "" {
		stateEncryptionConfig = rkeFullState.CurrentState.EncryptionConfig
	}

	kubeCluster, err := cluster.InitClusterObject(ctx, rkeConfig, flags, stateEncryptionConfig)
	if err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}

	if kubeCluster.IsEncryptionCustomConfig() {
		return APIURL, caCrt, clientCert, clientKey, nil, fmt.Errorf("can't rotate encryption keys: Key Rotation is not supported with custom configuration")
	}
	if !kubeCluster.IsEncryptionEnabled() {
		return APIURL, caCrt, clientCert, clientKey, nil, fmt.Errorf("can't rotate encryption keys: Encryption Configuration is disabled. Please disable rotate_encryption_key and run rke up again")
	}

	kubeCluster.Certificates = rkeFullState.DesiredState.CertificatesBundle
	if err := kubeCluster.SetupDialers(ctx, dialersOptions); err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}
	if err := kubeCluster.TunnelHosts(ctx, flags); err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}
	if len(kubeCluster.ControlPlaneHosts) > 0 {
		APIURL = fmt.Sprintf("https://%s:6443", kubeCluster.ControlPlaneHosts[0].Address)
	}
	clientCert = string(cert.EncodeCertPEM(kubeCluster.Certificates[pki.KubeAdminCertName].Certificate))
	clientKey = string(cert.EncodePrivateKeyPEM(kubeCluster.Certificates[pki.KubeAdminCertName].Key))
	caCrt = string(cert.EncodeCertPEM(kubeCluster.Certificates[pki.CACertName].Certificate))

	err = kubeCluster.RotateEncryptionKey(ctx, rkeFullState)
	if err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}

	// make sure we have the latest state
	rkeFullState, _ = cluster.ReadStateFile(ctx, stateFilePath)

	log.Infof(ctx, "Reconciling cluster state")
	if err := kubeCluster.ReconcileDesiredStateEncryptionConfig(ctx, rkeFullState); err != nil {
		return APIURL, caCrt, clientCert, clientKey, nil, err
	}

	log.Infof(ctx, "Cluster secrets encryption key rotated successfully")
	return APIURL, caCrt, clientCert, clientKey, kubeCluster.Certificates, nil
}
