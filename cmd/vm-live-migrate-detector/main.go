package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type detectorOptions struct {
	kubeConfigPath string
	kubeContext    string
	shutdown       bool
	nodeName       string
}

var (
	AppVersion = "dev"
	GitCommit  = "commit"

	logDebug bool
	logTrace bool

	kubeConfigPath string
	kubeContext    string
	shutdown       bool
)

var rootCmd = &cobra.Command{
	Use:   "vm-live-migrate-detector NODENAME",
	Short: "VM Live Migrate Detector",
	Long: `A simple VM detector and executor for Harvester upgrades

The detector accepts a node name and inferences the possible places the VMs on top of it could be live migrated to.
If there is no place to go, it can optionally shut down the VMs.
	`,
	Version: AppVersion,
	Args:    cobra.ExactArgs(1),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logrus.SetOutput(os.Stdout)
		if logDebug {
			logrus.SetLevel(logrus.DebugLevel)
		}
		if logTrace {
			logrus.SetLevel(logrus.TraceLevel)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Context(context.Background())
		options := detectorOptions{
			kubeConfigPath: kubeConfigPath,
			kubeContext:    kubeContext,
			shutdown:       shutdown,
			nodeName:       args[0],
		}
		if err := run(ctx, options); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			os.Exit(1)
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
	rootCmd.Flags().BoolVar(&shutdown, "shutdown", false, "Do not shutdown non-migratable VMs")
}

func run(ctx context.Context, options detectorOptions) error {
	logrus.Info("Starting VM Live Migrate Detector")
	detector := newVMLiveMigrateDetector(options)
	detector.init()
	return detector.run(ctx)
}

func main() {
	cobra.CheckErr(rootCmd.Execute())
}
