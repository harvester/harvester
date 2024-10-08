package cmd

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/harvester/harvester/pkg/version"
)

var (
	logDebug bool
	logTrace bool

	KubeConfigPath string
	KubeContext    string
)

var RootCmd = &cobra.Command{
	Use:     "upgrade-helper",
	Short:   "Harvester Upgrade Helpers",
	Long:    "A collection of upgrade helpers for Harvester",
	Version: fmt.Sprintf("%s (%s)", version.Version, version.GitCommit),
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		logrus.SetOutput(os.Stdout)
		if logDebug {
			logrus.SetLevel(logrus.DebugLevel)
		}
		if logTrace {
			logrus.SetLevel(logrus.TraceLevel)
		}
	},
}

func init() {
	debug := envGetBool("DEBUG", false)
	trace := envGetBool("TRACE", false)

	RootCmd.PersistentFlags().BoolVar(&logDebug, "debug", debug, "set logging level to debug")
	RootCmd.PersistentFlags().BoolVar(&logTrace, "trace", trace, "set logging level to trace")
	RootCmd.PersistentFlags().StringVar(&KubeConfigPath, "kubeconfig", os.Getenv("KUBECONFIG"), "Path to the kubeconfig file")
	RootCmd.PersistentFlags().StringVar(&KubeContext, "kubecontext", os.Getenv("KUBECONTEXT"), "Context name")
}
