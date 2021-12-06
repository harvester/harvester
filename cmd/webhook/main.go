package main

import (
	"fmt"
	_ "net/http/pprof"

	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/harvester/harvester/pkg/cmd"
	harvesterconfig "github.com/harvester/harvester/pkg/config"
	apiserver "github.com/harvester/harvester/pkg/server"
	"github.com/harvester/harvester/pkg/webhook/config"
	"github.com/harvester/harvester/pkg/webhook/server"
)

func main() {
	var options config.Options

	flags := []cli.Flag{
		cli.IntFlag{
			Name:        "threadiness",
			EnvVar:      "THREADINESS",
			Usage:       "Specify controller threads",
			Value:       5,
			Destination: &options.Threadiness,
		},
		cli.IntFlag{
			Name:        "https-port",
			EnvVar:      "HARVESTER_WEBHOOK_SERVER_HTTPS_PORT",
			Usage:       "HTTPS listen port",
			Value:       9443,
			Destination: &options.HTTPSListenPort,
		},
		cli.StringFlag{
			Name:        "namespace",
			EnvVar:      "NAMESPACE",
			Destination: &options.Namespace,
			Usage:       "The harvester namespace",
			Required:    true,
		},
		cli.StringFlag{
			Name:        "controller-user",
			EnvVar:      "HARVESTER_CONTROLLER_USER_NAME",
			Destination: &options.HarvesterControllerUsername,
			Usage:       "The harvester controller username",
		},
		cli.StringFlag{
			Name:        "gc-user",
			EnvVar:      "GARBAGE_COLLECTION_USER_NAME",
			Destination: &options.GarbageCollectionUsername,
			Usage:       "The system username that performs garbage collection",
			Value:       "system:serviceaccount:kube-system:generic-garbage-collector",
		},
	}

	app := cmd.NewApp("Harvester Admission Webhook Server", "", flags, func(commonOptions *harvesterconfig.CommonOptions) error {
		return run(commonOptions, &options)
	})
	app.Run()
}

func run(commonOptions *harvesterconfig.CommonOptions, options *config.Options) error {
	logrus.Info("Starting webhook server")

	ctx := signals.SetupSignalContext()

	kubeConfig, err := apiserver.GetConfig(commonOptions.KubeConfig)
	if err != nil {
		return fmt.Errorf("failed to find kubeconfig: %v", err)
	}

	restCfg, err := kubeConfig.ClientConfig()
	if err != nil {
		return err
	}

	logrus.Debugf("Harvester controller username: %s", options.HarvesterControllerUsername)

	s := server.New(ctx, restCfg, options)
	if err := s.ListenAndServe(); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}
