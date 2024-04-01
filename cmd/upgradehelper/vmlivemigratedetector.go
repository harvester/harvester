package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/harvester/harvester/pkg/upgradehelper/vmlivemigratedetector"
)

var (
	shutdown bool
)

var vmLiveMigrateDetectorCmd = &cobra.Command{
	Use:   "vm-live-migrate-detector NODENAME",
	Short: "VM Live Migrate Detector",
	Long: `A simple VM detector and executor for Harvester upgrades

The detector accepts a node name and inferences the possible places the VMs on top of it could be live migrated to.
If there is no place to go, it can optionally shut down the VMs.
	`,
	Args: cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		ctx := context.Context(context.Background())
		options := vmlivemigratedetector.DetectorOptions{
			KubeConfigPath: kubeConfigPath,
			KubeContext:    kubeContext,
			Shutdown:       shutdown,
			NodeName:       args[0],
		}
		if err := run(ctx, options); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	vmLiveMigrateDetectorCmd.Flags().BoolVar(&shutdown, "shutdown", false, "Shutdown non-migratable VMs")

	rootCmd.AddCommand(vmLiveMigrateDetectorCmd)
}

func run(ctx context.Context, options vmlivemigratedetector.DetectorOptions) error {
	logrus.Info("Starting VM Live Migrate Detector")
	detector := vmlivemigratedetector.NewVMLiveMigrateDetector(options)
	if err := detector.Init(); err != nil {
		return err
	}
	return detector.Run(ctx)
}
