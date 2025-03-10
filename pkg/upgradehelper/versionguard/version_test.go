package versionguard

import (
	"fmt"
	"testing"

	"github.com/harvester/go-common/version"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

const (
	testUpgradeNamespace = "harvester-system"
	testUpgradeName      = "test-upgrade"
)

var (
	repoInfoTemplate = `
release:
  harvester: %s
  minUpgradableVersion: %s
`
)

func TestCheck(t *testing.T) {
	testCases := []struct {
		name                 string
		upgrade              *v1beta1.Upgrade
		strictMode           bool
		minUpgradableVersion string
		expectedErr          error
	}{
		{
			name: "upgrading from v1.2.1 to v1.2.2 with minimum upgradable version v1.2.1",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2.1",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.2.2", "v1.2.1"),
				},
			},
			strictMode: true,
		},
		{
			name: "upgrading from v1.2.0 to v1.2.2 with minimum upgradable version v1.2.1",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2.0",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.2.2", "v1.2.1"),
				},
			},
			strictMode:  true,
			expectedErr: version.ErrMinUpgradeRequirement,
		},
		{
			name: "upgrading from v1.2.1 to v1.2.0 with minimum upgradable version v1.1.2 (effectively downgrade)",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2.1",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.2.0", "v1.1.2"),
				},
			},
			strictMode:  true,
			expectedErr: version.ErrDowngrade,
		},
		{
			name: "upgrading from v1.2.1 to v1.2.2-rc1 with minimum upgradable version v1.2.1 (upgrade to rc)",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2.1",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.2.2-rc1", "v1.2.1"),
				},
			},
			strictMode: true,
		},
		{
			name: "upgrading from v1.2.2-rc1 to v1.2.2-rc2 with minimum upgradable version v1.2.1 (upgrade from rc to rc)",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2.2-rc1",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.2.2-rc2", "v1.2.1"),
				},
			},
			strictMode: true,
		},
		{
			name: "upgrading from v1.2.2-rc2 to v1.2.2-rc1 with minimum upgradable version v1.2.1 (effectively downgrade from rc to rc)",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2.2-rc2",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.2.2-rc1", "v1.2.1"),
				},
			},
			strictMode:  true,
			expectedErr: version.ErrDowngrade,
		},
		{
			name: "upgrading from v1.2.1 to v1.2-head without minimum upgradable version",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2.1",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.2-head", ""),
				},
			},
			strictMode: true,
		},
		{
			name: "upgrading from v1.2-head to v1.3-head without minimum upgradable version (dev version upgrades)",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2-head",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.3-head", ""),
				},
			},
			strictMode: true,
		},
		{
			name: "upgrading from v1.2-head to v1.3.1 with minimum upgradable version v1.2.2",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2-head",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.3.1", "v1.2.2"),
				},
			},
			strictMode:  true,
			expectedErr: version.ErrDevUpgrade,
		},
		{
			name: "upgrading from v1.2-head to v1.3.1 with minimum upgradable version v1.2.2 (disable strict mode)",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2-head",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.3.1", "v1.2.2"),
				},
			},
			strictMode: false,
		},
		{
			name: "upgrading from v1.2.2-rc1 to v1.2.2 with minimum upgradable version v1.2.1",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testUpgradeNamespace,
					Name:      testUpgradeName,
				},
				Status: v1beta1.UpgradeStatus{
					PreviousVersion: "v1.2.2-rc1",
					RepoInfo:        fmt.Sprintf(repoInfoTemplate, "v1.2.2", "v1.2.1"),
				},
			},
			strictMode: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualErr := Check(tc.upgrade, tc.strictMode, tc.minUpgradableVersion)
			if tc.expectedErr != nil {
				assert.Equal(t, tc.expectedErr, actualErr, tc.name)
			} else {
				assert.Nil(t, actualErr, tc.name)
			}
		})
	}
}
