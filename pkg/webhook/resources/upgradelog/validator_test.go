package upgradelog

import (
	"context"
	"testing"

	"github.com/rancher/wrangler/v3/pkg/webhook"
	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/webhook/config"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const harvesterControllerUsername = "system:serviceaccount:harvester-system:harvester"

func TestCreate(t *testing.T) {
	type input struct {
		upgradeLog *v1beta1.UpgradeLog
		upgrades   []*v1beta1.Upgrade
		request    *types.Request
	}

	type output struct {
		wantErr bool
		errMsg  string
	}

	testCases := []struct {
		name   string
		input  input
		output output
	}{
		{
			name: "valid upgradelog with existing upgrade",
			input: input{
				upgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "test-upgrade",
					},
				},
				upgrades: []*v1beta1.Upgrade{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-upgrade",
							Namespace: util.HarvesterSystemNamespaceName,
						},
						Spec: v1beta1.UpgradeSpec{
							LogEnabled: true,
						},
					},
				},
				request: newControllerRequest(),
			},
			output: output{
				wantErr: false,
			},
		},
		{
			name: "upgradelog with LogEnabled disabled is rejected",
			input: input{
				upgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "test-upgrade",
					},
				},
				upgrades: []*v1beta1.Upgrade{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-upgrade",
							Namespace: util.HarvesterSystemNamespaceName,
						},
					},
				},
				request: newControllerRequest(),
			},
			output: output{
				wantErr: true,
				errMsg:  "LogEnabled is false",
			},
		},
		{
			name: "upgradelog with non-existent upgrade",
			input: input{
				upgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "non-existent-upgrade",
					},
				},
				upgrades: []*v1beta1.Upgrade{},
				request:  newControllerRequest(),
			},
			output: output{
				wantErr: true,
				errMsg:  "referenced upgrade non-existent-upgrade not found",
			},
		},
		{
			name: "upgradelog without upgradeName",
			input: input{
				upgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "",
					},
				},
				upgrades: []*v1beta1.Upgrade{},
				request:  newControllerRequest(),
			},
			output: output{
				wantErr: true,
				errMsg:  "upgradeName field is not specified",
			},
		},
		{
			name: "upgradelog outside harvester-system namespace is rejected",
			input: input{
				upgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: "default",
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "test-upgrade",
					},
				},
				upgrades: []*v1beta1.Upgrade{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-upgrade",
							Namespace: util.HarvesterSystemNamespaceName,
						},
					},
				},
				request: newControllerRequest(),
			},
			output: output{
				wantErr: true,
				errMsg:  "UpgradeLog must be created in namespace " + util.HarvesterSystemNamespaceName,
			},
		},
		{
			name: "upgradelog created by non-controller user is rejected",
			input: input{
				upgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "test-upgrade",
					},
				},
				upgrades: []*v1beta1.Upgrade{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-upgrade",
							Namespace: util.HarvesterSystemNamespaceName,
						},
						Spec: v1beta1.UpgradeSpec{
							LogEnabled: true,
						},
					},
				},
				request: newUserRequest("rancher"),
			},
			output: output{
				wantErr: true,
				errMsg:  "upgradelog can only be created by the harvester controller",
			},
		},
		{
			name: "upgradelog created without request is rejected",
			input: input{
				upgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "test-upgrade",
					},
				},
				upgrades: []*v1beta1.Upgrade{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-upgrade",
							Namespace: util.HarvesterSystemNamespaceName,
						},
					},
				},
				request: nil,
			},
			output: output{
				wantErr: true,
				errMsg:  "upgradelog can only be created by the harvester controller",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			for _, u := range tc.input.upgrades {
				_, err := clientset.HarvesterhciV1beta1().Upgrades(u.Namespace).Create(context.Background(), u, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			upgradeCache := fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades)
			upgradeLogCache := fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs)
			validator := NewValidator(upgradeCache, upgradeLogCache)

			err := validator.Create(tc.input.request, tc.input.upgradeLog)

			if tc.output.wantErr {
				assert.Error(t, err)
				if tc.output.errMsg != "" {
					assert.Contains(t, err.Error(), tc.output.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreate_DuplicateUpgradeLog(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	upgrade := &v1beta1.Upgrade{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-upgrade",
			Namespace: util.HarvesterSystemNamespaceName,
		},
		Spec: v1beta1.UpgradeSpec{
			LogEnabled: true,
		},
	}

	existingUpgradeLog := &v1beta1.UpgradeLog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-upgradelog",
			Namespace: util.HarvesterSystemNamespaceName,
		},
		Spec: v1beta1.UpgradeLogSpec{
			UpgradeName: "test-upgrade",
		},
	}

	newUpgradeLog := &v1beta1.UpgradeLog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-upgradelog",
			Namespace: util.HarvesterSystemNamespaceName,
		},
		Spec: v1beta1.UpgradeLogSpec{
			UpgradeName: "test-upgrade",
		},
	}

	_, err := clientset.HarvesterhciV1beta1().Upgrades(upgrade.Namespace).Create(context.Background(), upgrade, metav1.CreateOptions{})
	assert.NoError(t, err)

	_, err = clientset.HarvesterhciV1beta1().UpgradeLogs(existingUpgradeLog.Namespace).Create(context.Background(), existingUpgradeLog, metav1.CreateOptions{})
	assert.NoError(t, err)

	upgradeCache := fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades)
	upgradeLogCache := fakeclients.UpgradeLogCache(clientset.HarvesterhciV1beta1().UpgradeLogs)
	validator := NewValidator(upgradeCache, upgradeLogCache)

	err = validator.Create(newControllerRequest(), newUpgradeLog)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists for upgrade")
	assert.Contains(t, err.Error(), "test-upgrade")
}

func TestUpdate(t *testing.T) {
	type input struct {
		oldUpgradeLog *v1beta1.UpgradeLog
		newUpgradeLog *v1beta1.UpgradeLog
	}

	type output struct {
		wantErr bool
		errMsg  string
	}

	testCases := []struct {
		name   string
		input  input
		output output
	}{
		{
			name: "valid update when upgradeName does not change",
			input: input{
				oldUpgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "test-upgrade",
					},
				},
				newUpgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							"foo": "bar",
						},
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "test-upgrade",
					},
				},
			},
			output: output{
				wantErr: false,
			},
		},
		{
			name: "reject update when upgradeName changes",
			input: input{
				oldUpgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "test-upgrade",
					},
				},
				newUpgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "different-upgrade",
					},
				},
			},
			output: output{
				wantErr: true,
				errMsg:  "spec.upgradeName is immutable",
			},
		},
		{
			name: "allow update when namespace is unchanged outside harvester-system",
			input: input{
				oldUpgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: "default",
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "test-upgrade",
					},
				},
				newUpgradeLog: &v1beta1.UpgradeLog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgradelog",
						Namespace: "default",
					},
					Spec: v1beta1.UpgradeLogSpec{
						UpgradeName: "test-upgrade",
					},
				},
			},
			output: output{
				wantErr: false,
			},
		},
	}

	validator := NewValidator(nil, nil)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Update(nil, tc.input.oldUpgradeLog, tc.input.newUpgradeLog)
			if tc.output.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.output.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func newUserRequest(username string) *types.Request {
	return types.NewRequest(
		&webhook.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{Username: username},
			},
		},
		&config.Options{HarvesterControllerUsername: harvesterControllerUsername},
	)
}

func newControllerRequest() *types.Request {
	return newUserRequest(harvesterControllerUsername)
}
