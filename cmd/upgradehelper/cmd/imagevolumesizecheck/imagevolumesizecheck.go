package imagevolumesizecheck

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/harvester/harvester/cmd/upgradehelper/cmd"
	"github.com/harvester/harvester/pkg/upgradehelper/imagevolumesizecheck"
)

var upgrade string

var imageVolumeSizeCheckCmd = &cobra.Command{
	Use:   "image-volume-size-check",
	Short: "Report volumes smaller than their source VM image",
	Long: `Scan all PersistentVolumeClaims cluster-wide for volumes created from a
VirtualMachineImage (via the harvesterhci.io/imageId annotation, or as the
image's own golden-image PVC) whose size is smaller than the image's
virtual size.
`,
	Run: func(_ *cobra.Command, _ []string) {
		ctx := context.Background()
		checker := imagevolumesizecheck.NewChecker(imagevolumesizecheck.Options{
			KubeConfigPath: cmd.KubeConfigPath,
			KubeContext:    cmd.KubeContext,
			Upgrade:        upgrade,
		})
		if err := checker.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to initialize image volume size checker: %v\n", err)
			os.Exit(1)
		}
		assessment, err := checker.Run(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "image volume size check failed: %v\n", err)
			os.Exit(1)
		}
		logrus.Infof("image volume size check completed: scanned %d, violations %d", assessment.Scanned, len(assessment.Violations))
	},
}

func init() {
	imageVolumeSizeCheckCmd.Flags().StringVar(&upgrade, "upgrade", "", "Upgrade CR name, used to record a summary event (optional)")

	cmd.RootCmd.AddCommand(imageVolumeSizeCheckCmd)
}
