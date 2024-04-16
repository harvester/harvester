package versionguard

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/harvester/harvester/cmd/upgradehelper/cmd"
	harv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/upgradehelper/versionguard"
)

const (
	harvesterSystemNamespace = "harvester-system"
)

var (
	strictMode           bool
	minUpgradableVersion string

	versionGuardCmd = &cobra.Command{
		Use:   "version-guard UPGRADENAME",
		Short: "Version Guard",
		Long: `A simple guard that checks whether the version can be upgraded to. The validating criteria includes:

	- Disallow any downgrade
	- Disallow upgrades from any formal release version lower than the minimal upgrade requirement of the targeting formal release version
	- Disallow upgrades from any dev version to any formal release version (if the strict mode is enabled)
	- Disallow upgrades from any dev version to any prerelease version (if the strict mode is enabled)
	- Allow upgrades from any lower prerelease version to any higher one within the same release version but not across different release versions

Any other upgrade paths not explicitly mentioned above are allowed.
If the validation fails due to a violation of the rules here in, the command exited with code 1.
If the validation passes, the command exits normally with code 0.
	`,
		Args: cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			ctx := context.Context(context.Background())
			if err := run(ctx, args[0]); err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				os.Exit(1)
			}
		},
	}
)

type versionGuard struct {
	kubeConfig  string
	kubeContext string

	strictMode           bool
	minUpgradableVersion string
	upgradeName          string

	harvClient *harv1type.HarvesterhciV1beta1Client
}

func init() {
	versionGuardCmd.Flags().BoolVar(&strictMode, "strict", true, "The strict mode controls whether dev versions can be upgraded. If it is `true`, upgrading from dev versions is prohibited. Note: upgrading to dev versions is always allowed. Default to `true`.")
	versionGuardCmd.Flags().StringVar(&minUpgradableVersion, "min-upgradable-version", "", "Manually specify a minimum upgradable version. The value has precedence over the one in the upgrade object.")

	cmd.RootCmd.AddCommand(versionGuardCmd)
}

func run(ctx context.Context, upgradeName string) error {
	logrus.Info("Starting Version Guard")

	versionguard := &versionGuard{
		kubeConfig:  cmd.KubeConfigPath,
		kubeContext: cmd.KubeContext,

		strictMode:           strictMode,
		minUpgradableVersion: minUpgradableVersion,

		upgradeName: upgradeName,
	}

	if err := versionguard.init(); err != nil {
		return err
	}
	return versionguard.run(ctx)
}

func (g *versionGuard) init() error {
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{
			ExplicitPath: g.kubeConfig,
		},
		&clientcmd.ConfigOverrides{
			ClusterInfo:    clientcmdapi.Cluster{},
			CurrentContext: g.kubeContext,
		},
	)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return err
	}

	g.harvClient, err = harv1type.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	return nil
}

func (g *versionGuard) run(ctx context.Context) error {
	if g.upgradeName == "" {
		return fmt.Errorf("please specify the upgrade name")
	}

	upgrade, err := g.harvClient.Upgrades(harvesterSystemNamespace).Get(ctx, g.upgradeName, v1.GetOptions{})
	if err != nil {
		return err
	}

	return versionguard.Check(upgrade, g.strictMode, g.minUpgradableVersion)
}
