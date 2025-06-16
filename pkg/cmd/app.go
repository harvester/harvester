package cmd

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ehazlett/simplelog"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/version"
)

type App struct {
	app     *cli.App
	Options *config.CommonOptions
}

type Action func(*config.CommonOptions) error

func NewApp(name string, usage string, flags []cli.Flag, action Action) *App {
	cliApp := cli.NewApp()

	cliApp.Name = name
	cliApp.Version = version.FriendlyVersion()
	cliApp.Usage = usage

	// common flags
	options := config.CommonOptions{}
	cliApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "kubeconfig",
			EnvVar:      "KUBECONFIG",
			Usage:       "Kube config for accessing k8s cluster",
			Destination: &options.KubeConfig,
		},
		cli.StringFlag{
			Name:        "profile-listen-address",
			Value:       "0.0.0.0:6060",
			Usage:       "Address to listen on for profiling",
			Destination: &options.ProfilerAddress,
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
		cli.StringFlag{
			Name:        "log-format",
			EnvVar:      "HARVESTER_LOG_FORMAT",
			Usage:       "Log format",
			Value:       "text",
			Destination: &options.LogFormat,
		},
	}

	cliApp.Flags = append(cliApp.Flags, flags...)
	cliApp.Action = func(_ *cli.Context) error {
		initProfiling(&options)
		initLogs(&options)
		return action(&options)
	}

	app := &App{
		app:     cliApp,
		Options: &options,
	}
	// log app basic info
	app.info()
	return app
}

func initProfiling(options *config.CommonOptions) {
	// enable profiler
	if options.ProfilerAddress != "" {
		go func() {
			server := http.Server{
				Addr: options.ProfilerAddress,
				// fix G114: Use of net/http serve function that has no support for setting timeouts (gosec)
				// refer to https://app.deepsource.com/directory/analyzers/go/issues/GO-S2114
				ReadHeaderTimeout: 10 * time.Second,
			}
			log.Println(server.ListenAndServe())
		}()
	}
}

func initLogs(options *config.CommonOptions) {
	switch options.LogFormat {
	case "simple":
		logrus.SetFormatter(&simplelog.StandardFormatter{})
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{})
	default:
		logrus.SetFormatter(&logrus.TextFormatter{})
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

func (a *App) Run() {
	if err := a.app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func (a *App) info() {
	logrus.Infof("Starting %v version %v", a.app.Name, a.app.Version)
	logrus.Debugf("Options: %+v", a.Options)
}
