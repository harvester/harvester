package deployment

import (
	"testing"

	"github.com/rancher/wrangler/v3/pkg/webhook"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/webhook/config"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func TestDeleteRepoDeployment(t *testing.T) {
	upgrade := &harvesterv1.Upgrade{ObjectMeta: metav1.ObjectMeta{
		Name: "upgrade-a", Namespace: util.HarvesterSystemNamespaceName, UID: "uid-a",
	}}
	repo := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: "upgrade-repo-upgrade-a", Namespace: upgrade.Namespace,
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: harvesterv1.SchemeGroupVersion.String(), Kind: "Upgrade", Name: upgrade.Name, UID: upgrade.UID,
		}},
	}}
	for _, tc := range []struct {
		name     string
		username string
		mutate   func(*harvesterv1.Upgrade, *appsv1.Deployment)
		deny     bool
	}{
		{name: "missing labels", deny: true},
		{name: "incorrect labels", deny: true, mutate: func(_ *harvesterv1.Upgrade, d *appsv1.Deployment) {
			d.Labels = map[string]string{util.LabelHarvesterUpgrade: "other", util.LabelHarvesterUpgradeComponent: "other"}
		}},
		{name: "same upgrade name with new UID", mutate: func(u *harvesterv1.Upgrade, _ *appsv1.Deployment) { u.UID = "new" }},
		{name: "owner no longer exists", mutate: func(u *harvesterv1.Upgrade, _ *appsv1.Deployment) { u.Name = "other" }},
		{name: "other deployment of same upgrade", mutate: func(_ *harvesterv1.Upgrade, d *appsv1.Deployment) { d.Name = "other" }},
		{name: "completed", mutate: func(u *harvesterv1.Upgrade, _ *appsv1.Deployment) { harvesterv1.UpgradeCompleted.True(u) }},
		{name: "failed", mutate: func(u *harvesterv1.Upgrade, _ *appsv1.Deployment) { harvesterv1.UpgradeCompleted.False(u) }},
		{name: "controller", username: "controller"},
		{name: "garbage collector", username: "gc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, d := upgrade.DeepCopy(), repo.DeepCopy()
			if tc.mutate != nil {
				tc.mutate(u, d)
			}
			clientset := fake.NewSimpleClientset(u)
			v := NewValidator(fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades), nil)
			request := types.NewRequest(&webhook.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Delete, UserInfo: authenticationv1.UserInfo{Username: tc.username},
			}}, &config.Options{HarvesterControllerUsername: "controller", GarbageCollectionUsername: "gc"})
			err := v.Delete(request, d)
			if tc.deny {
				require.ErrorContains(t, err, "upgrade upgrade-a is still in progress")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
