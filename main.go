//go:generate go run pkg/codegen/cleanup/main.go
//go:generate /bin/rm -rf pkg/generated
//go:generate go run pkg/codegen/main.go
//go:generate /bin/bash scripts/generate-manifest

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/ehazlett/simplelog"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/server"
	"github.com/harvester/harvester/pkg/version"
)

var (
	profileAddress = "localhost:6060"
	KubeConfig     string
)

func main() {
	var options config.Options

	app := cli.NewApp()
	app.Name = "rancher-harvester"
	app.Version = version.FriendlyVersion()
	app.Usage = ""
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "kubeconfig",
			EnvVar:      "KUBECONFIG",
			Usage:       "Kube config for accessing k8s cluster",
			Destination: &KubeConfig,
		},
		cli.BoolFlag{
			Name:        "debug",
			EnvVar:      "HARVESTER_DEBUG",
			Usage:       "Enable debug logs",
			Destination: &options.Debug,
		},
		cli.BoolFlag{
			Name:        "trace",
			EnvVar:      "HARVESTER_TRACE",
			Usage:       "Enable trace logs",
			Destination: &options.Trace,
		},
		cli.IntFlag{
			Name:        "threadiness",
			EnvVar:      "THREADINESS",
			Usage:       "Specify controller threads",
			Value:       10,
			Destination: &options.Threadiness,
		},
		cli.IntFlag{
			Name:        "http-port",
			EnvVar:      "HARVESTER_SERVER_HTTP_PORT",
			Usage:       "HTTP listen port",
			Value:       8080,
			Destination: &options.HTTPListenPort,
		},
		cli.IntFlag{
			Name:        "https-port",
			EnvVar:      "HARVESTER_SERVER_HTTPS_PORT",
			Usage:       "HTTPS listen port",
			Value:       8443,
			Destination: &options.HTTPSListenPort,
		},
		cli.StringFlag{
			Name:        "namespace",
			EnvVar:      "NAMESPACE",
			Destination: &options.Namespace,
			Usage:       "The default namespace to store management resources",
			Required:    true,
		},
		cli.BoolFlag{
			Name:        "skip-authentication",
			EnvVar:      "SKIP_AUTHENTICATION",
			Usage:       "Define whether to skip auth login or not, default to false",
			Destination: &options.SkipAuthentication,
		},
		cli.StringFlag{
			Name:   "authentication-mode",
			EnvVar: "HARVESTER_AUTHENTICATION_MODE",
			Usage:  "Define authentication mode, kubernetesCredentials, localUser and rancher are supported, could config more than one mode, separated by comma",
		},
		cli.BoolFlag{
			Name:        "hci-mode",
			EnvVar:      "HCI_MODE",
			Usage:       "Enable HCI mode. Additional controllers are registered in HCI mode",
			Destination: &options.HCIMode,
		},
		cli.StringFlag{
			Name:        "profile-listen-address",
			Value:       "0.0.0.0:6060",
			Usage:       "Address to listen on for profiling",
			Destination: &profileAddress,
		},
		cli.BoolFlag{
			Name:        "rancher-embedded",
			EnvVar:      "RANCHER_EMBEDDED",
			Usage:       "Specify whether the Harvester is running with embedded Rancher mode, default to false",
			Destination: &options.RancherEmbedded,
		},
		cli.StringFlag{
			Name:        "rancher-server-url",
			EnvVar:      "RANCHER_SERVER_URL",
			Usage:       "Specify the URL to connect to the Rancher server",
			Destination: &options.RancherURL,
		},
	}
	app.Action = func(c *cli.Context) error {
		// enable profiler
		if profileAddress != "" {
			go func() {
				log.Println(http.ListenAndServe(profileAddress, nil))
			}()
		}
		initLogs(c, options)
		return run(c, options)
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func initLogs(c *cli.Context, options config.Options) {
	switch c.String("log-format") {
	case "simple":
		logrus.SetFormatter(&simplelog.StandardFormatter{})
	case "text":
		logrus.SetFormatter(&logrus.TextFormatter{})
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{})
	}
	logrus.SetOutput(os.Stdout)
	if options.Debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debugf("Loglevel set to [%v]", logrus.DebugLevel)
	}
	if options.Trace {
		logrus.SetLevel(logrus.TraceLevel)
		logrus.Tracef("Loglevel set to [%v]", logrus.TraceLevel)
	}
}

func run(c *cli.Context, options config.Options) error {
	logrus.Info("Starting controller")
	ctx := signals.SetupSignalHandler(context.Background())

	kubeConfig, err := server.GetConfig(KubeConfig)
	if err != nil {
		return fmt.Errorf("failed to find kubeconfig: %v", err)
	}

	harv, err := server.New(ctx, kubeConfig, options)
	if err != nil {
		return fmt.Errorf("failed to create harvester server: %v", err)
	}
	return harv.ListenAndServe(nil, options)
}
