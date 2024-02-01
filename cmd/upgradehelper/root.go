package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	logDebug bool
	logTrace bool

	kubeConfigPath string
	kubeContext    string
)

var rootCmd = &cobra.Command{
	Use:     "upgrade-helper",
	Short:   "Harvester Upgrade Helpers",
	Long:    "A collection of upgrade helpers for Harvester",
	Version: AppVersion,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
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

	rootCmd.PersistentFlags().BoolVar(&logDebug, "debug", debug, "set logging level to debug")
	rootCmd.PersistentFlags().BoolVar(&logTrace, "trace", trace, "set logging level to trace")

	rootCmd.Flags().StringVar(&kubeConfigPath, "kubeconfig", os.Getenv("KUBECONFIG"), "Path to the kubeconfig file")
	rootCmd.Flags().StringVar(&kubeContext, "kubecontext", os.Getenv("KUBECONTEXT"), "Context name")
}
