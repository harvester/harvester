package restorevm

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/harvester/harvester/cmd/upgradehelper/cmd"
	"github.com/harvester/harvester/pkg/upgradehelper/restorevm"
)

var (
	upgrade  string
	nodeName string
)

var restoreVMCmd = &cobra.Command{
	Use:   "restore-vm --node NODENAME --upgrade UPGRADENAME",
	Short: "Restore VMs after upgrade",
	Long: `Restore VMs for a node after upgrade.

This command will:
1. Get VM names from the upgrade ConfigMap.
2. Wait until KubeVirt is ready.
3. Start all VMs listed in the ConfigMap for the node.
`,
	Run: func(_ *cobra.Command, _ []string) {
		ctx := context.Background()
		handler, err := restorevm.NewRestoreVMHandler(cmd.KubeConfigPath, cmd.KubeContext, nodeName, upgrade)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create restore handler: %v\n", err)
			os.Exit(1)
		}
		if err := handler.Run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "restore failed: %v\n", err)
			os.Exit(1)
		}
		logrus.Info("Restore VM completed successfully")
	},
}

func init() {
	restoreVMCmd.Flags().StringVar(&nodeName, "node", "", "Node name (required)")
	restoreVMCmd.Flags().StringVar(&upgrade, "upgrade", "", "Upgrade CR name (required)")
	err := restoreVMCmd.MarkFlagRequired("node")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to mark node flag as required: %v\n", err)
		os.Exit(1)
	}
	err = restoreVMCmd.MarkFlagRequired("upgrade")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to mark upgrade flag as required: %v\n", err)
		os.Exit(1)
	}

	cmd.RootCmd.AddCommand(restoreVMCmd)
}
