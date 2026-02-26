package networkattachmentdefinition

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	apiv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	fakegenerated "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	ctlv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestDeleteAllowsWhenNotUsed(t *testing.T) {
	clientset := fakegenerated.NewSimpleClientset()
	validator := NewValidator(ctlv1beta1.SettingCache(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings)))

	testCases := []struct {
		name      string
		nad       *cniv1.NetworkAttachmentDefinition
		expectErr bool
	}{
		{
			name: "incorrect namespace and name",
			nad: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "nad1",
				},
			},
			expectErr: false,
		},
		{
			name: "storage-network NAD: correct namespace and name, setting not referencing it",
			nad: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: util.HarvesterSystemNamespaceName,
					Name:      util.StorageNetworkNetAttachDefPrefix + "aaaaa",
				},
			},
			expectErr: false,
		},
		{
			name: "rwx-storage-network NAD: correct namespace and name, setting not referencing it",
			nad: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: util.RWXStorageNetworkNetAttachDefNamespace,
					Name:      util.RWXStorageNetworkNetAttachDefPrefix + "aaaaa",
				},
			},
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		err := validator.Delete(nil, tc.nad)
		if tc.expectErr {
			assert.Error(t, err, tc.name)
		} else {
			assert.NoError(t, err, tc.name)
		}
	}
}

func TestDeleteBlocksWhenUsed(t *testing.T) {
	nadName := util.StorageNetworkNetAttachDefPrefix + "bbbbb"
	nadNamespacedName := fmt.Sprintf("%s/%s", util.HarvesterSystemNamespaceName, nadName)

	clientset := fakegenerated.NewSimpleClientset(&apiv1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.StorageNetworkName,
			Annotations: map[string]string{
				util.NadStorageNetworkAnnotation: nadNamespacedName,
			},
		},
	})

	validator := NewValidator(ctlv1beta1.SettingCache(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings)))

	nad := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.HarvesterSystemNamespaceName,
			Name:      nadName,
			Annotations: map[string]string{
				util.StorageNetworkAnnotation: "true",
			},
		},
	}

	err := validator.Delete(nil, nad)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete NetworkAttachmentDefinition")
}

func TestDeleteBlocksRWXNadWhenUsed(t *testing.T) {
	nadName := util.RWXStorageNetworkNetAttachDefPrefix + "bbbbb"
	nadNamespacedName := fmt.Sprintf("%s/%s", util.RWXStorageNetworkNetAttachDefNamespace, nadName)

	clientset := fakegenerated.NewSimpleClientset(&apiv1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXStorageNetworkSettingName,
			Annotations: map[string]string{
				util.RWXNadStorageNetworkAnnotation: nadNamespacedName,
			},
		},
	})

	validator := NewValidator(ctlv1beta1.SettingCache(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings)))

	nad := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.RWXStorageNetworkNetAttachDefNamespace,
			Name:      nadName,
			Annotations: map[string]string{
				util.RWXStorageNetworkAnnotation: "true",
			},
		},
	}

	err := validator.Delete(nil, nad)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete NetworkAttachmentDefinition")
}

func TestDeleteFailsWhenSettingCacheFails(t *testing.T) {
	clientset := fakegenerated.NewSimpleClientset()
	clientset.PrependReactor("get", "settings", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("simulated cache error")
	})

	validator := NewValidator(ctlv1beta1.SettingCache(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings)))

	nad := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.HarvesterSystemNamespaceName,
			Name:      util.StorageNetworkNetAttachDefPrefix + "ccccc",
		},
	}

	err := validator.Delete(nil, nad)
	assert.Error(t, err)
}
