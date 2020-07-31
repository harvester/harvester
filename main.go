//go:generate go run pkg/codegen/cleanup/main.go
//go:generate /bin/rm -rf pkg/generated
//go:generate go run pkg/codegen/main.go

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/rancher/vm/pkg/config"
	"github.com/rancher/vm/pkg/server"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var (
	Version    = "v0.0.0-dev"
	GitCommit  = "HEAD"
	KubeConfig string
)

func main() {
	app := cli.NewApp()
	app.Name = "rancher-vm-server"
	app.Version = fmt.Sprintf("%s (%s)", Version, GitCommit)
	app.Usage = ""
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "kubeconfig",
			EnvVar:      "KUBECONFIG",
			Destination: &KubeConfig,
		},
		cli.IntFlag{
			Name:        "threadiness",
			EnvVar:      "THREADINESS",
			Value:       10,
			Destination: &config.Threadiness,
		},
		cli.IntFlag{
			Name:        "http-port",
			EnvVar:      "VM_SERVER_HTTP_PORT",
			Value:       8080,
			Destination: &config.HTTPListenPort,
		},
		cli.IntFlag{
			Name:        "https-port",
			EnvVar:      "VM_SERVER_HTTPS_PORT",
			Value:       8443,
			Destination: &config.HTTPSListenPort,
		},
		cli.StringFlag{
			Name:        "namespace",
			EnvVar:      "NAMESPACE",
			Destination: &config.Namespace,
			Usage:       "The default namespace to store management resources",
		},
		cli.StringFlag{
			Name:        "minio-url",
			EnvVar:      "MINIO_URL",
			Destination: &config.MinioURL,
			Required:    true,
		},
		cli.StringFlag{
			Name:        "minio-access-key",
			EnvVar:      "MINIO_ACCESS_KEY",
			Destination: &config.MinioAccessKey,
			Required:    true,
		},
		cli.StringFlag{
			Name:        "minio-secret-key",
			EnvVar:      "MINIO_SECRET_KEY",
			Destination: &config.MinioSecretKey,
			Required:    true,
		},
	}
	app.Action = run

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) {
	flag.Parse()

	logrus.Info("Starting controller")
	ctx := signals.SetupSignalHandler(context.Background())

	cfg, err := server.GetConfig(KubeConfig)
	if err != nil {
		logrus.Fatalf("failed to find kubeconfig: %v", err)
	}

	vm, err := server.New(ctx, cfg)
	if err != nil {
		logrus.Fatalf("failed to create vm server: %v", err)
	}
	if err := vm.Start(); err != nil {
		logrus.Fatalf("vm server stop, %v", err)
	}
}
