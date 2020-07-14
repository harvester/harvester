//go:generate go run pkg/codegen/cleanup/main.go
//go:generate /bin/rm -rf pkg/generated
//go:generate go run pkg/codegen/main.go

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/rancher/wrangler/pkg/kubeconfig"
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

	kubeConfig, err := kubeconfig.GetNonInteractiveClientConfig(KubeConfig).ClientConfig()
	if err != nil {
		logrus.Fatalf("failed to find kubeconfig: %v", err)
	}

	// this is vendor holder, need to be remove in the future.
	println(kubeConfig.Host)

	// foos, err := some.NewFactoryFromConfig(kubeConfig)
	// if err != nil {
	// 	logrus.Fatalf("Error building sample controllers: %s", err.Error())
	// }

	// foo.Register(ctx, foos.Some().V1().Foo())

	// if err := start.All(ctx, 2, foos); err != nil {
	// 	logrus.Fatalf("Error starting: %s", err.Error())
	// }

	<-ctx.Done()
}
