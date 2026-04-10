package sharemanager

import (
	"context"
	"testing"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	fakeclientset "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestOnRWXNetworkSettingChangeEnsuresSharedStorageDerivedResources(t *testing.T) {
	rwxSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name:        settings.RWXNetworkSettingName,
			Annotations: map[string]string{},
		},
		Value: `{"share-storage-network":true}`,
	}
	storageSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.StorageNetworkName,
			Annotations: map[string]string{
				util.NadStorageNetworkAnnotation: "harvester-system/storagenetwork-frqx7",
			},
		},
	}
	sourceNAD := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "storagenetwork-frqx7",
			Namespace: util.HarvesterSystemNamespaceName,
			Annotations: map[string]string{
				util.StorageNetworkAnnotation: "true",
			},
			Labels: map[string]string{
				util.HashStorageNetworkLabel: "test-hash",
			},
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28"]}}`,
		},
	}

	clientset := fakeclientset.NewSimpleClientset(rwxSetting, storageSetting)
	_, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), sourceNAD, metav1.CreateOptions{})
	require.NoError(t, err)

	handler := &Handler{
		settingsCache:    fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		nads:             fakeclients.NetworkAttachmentDefinitionClient(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		nadCache:         fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		ipPoolUsages:     fakeclients.IPPoolUsageClient(clientset.HarvesterhciV1beta1().IPPoolUsages),
		ipPoolUsageCache: fakeclients.IPPoolUsageCache(clientset.HarvesterhciV1beta1().IPPoolUsages),
	}

	updated, err := handler.OnRWXNetworkChange("", rwxSetting)
	require.NoError(t, err)
	require.NotNil(t, updated)

	staticNAD, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Get(context.Background(), "storagenetwork-frqx7-static", metav1.GetOptions{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"static"}}`, staticNAD.Spec.Config)

	excludePool, err := clientset.HarvesterhciV1beta1().IPPoolUsages().Get(context.Background(), "harvester-system-storagenetwork-frqx7-exclude", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "172.16.0.1/28", excludePool.Spec.CIDR)
	assert.Equal(t, util.IPPoolUsageDefaultReservedIPCount, excludePool.Spec.ReservedIPCount)
}

func TestOnRWXNetworkSettingChangeRemovesOldDerivedResourcesInSharedMode(t *testing.T) {
	rwxSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXNetworkSettingName,
			Annotations: map[string]string{
				util.RWXOldNadNetworkAnnotation: "harvester-system/storagenetwork-old",
			},
		},
		Value: `{"share-storage-network":true}`,
	}
	storageSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.StorageNetworkName,
			Annotations: map[string]string{
				util.NadStorageNetworkAnnotation: "harvester-system/storagenetwork-new",
			},
		},
	}
	sourceNAD := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "storagenetwork-new",
			Namespace: util.HarvesterSystemNamespaceName,
			Annotations: map[string]string{
				util.StorageNetworkAnnotation: "true",
			},
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28"]}}`,
		},
	}
	oldStaticNAD := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "storagenetwork-old-static",
			Namespace: util.HarvesterSystemNamespaceName,
		},
	}
	oldExcludePool := &harvesterv1beta1.IPPoolUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-system-storagenetwork-old-exclude",
		},
		Spec: harvesterv1beta1.IPPoolUsageSpec{
			CIDR: "172.16.0.1/28",
		},
	}

	clientset := fakeclientset.NewSimpleClientset(rwxSetting, storageSetting, oldExcludePool)
	_, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), sourceNAD, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), oldStaticNAD, metav1.CreateOptions{})
	require.NoError(t, err)

	handler := &Handler{
		settingsCache:    fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		nads:             fakeclients.NetworkAttachmentDefinitionClient(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		nadCache:         fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		ipPoolUsages:     fakeclients.IPPoolUsageClient(clientset.HarvesterhciV1beta1().IPPoolUsages),
		ipPoolUsageCache: fakeclients.IPPoolUsageCache(clientset.HarvesterhciV1beta1().IPPoolUsages),
	}

	_, err = handler.OnRWXNetworkChange("", rwxSetting)
	require.NoError(t, err)

	_, err = clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Get(context.Background(), oldStaticNAD.Name, metav1.GetOptions{})
	require.Error(t, err)
	_, err = clientset.HarvesterhciV1beta1().IPPoolUsages().Get(context.Background(), oldExcludePool.Name, metav1.GetOptions{})
	require.Error(t, err)
}

func TestOnRWXNetworkSettingChangeEnsuresDerivedResources(t *testing.T) {
	setting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXNetworkSettingName,
			Annotations: map[string]string{
				util.RWXNadNetworkAnnotation: "harvester-system/rwx-network-762g9",
			},
		},
		Value: `{"share-storage-network":false,"network":{"vlan":2011,"clusterNetwork":"mgmt","range":"172.16.0.0/24","exclude":["172.16.0.0/28"]}}`,
	}
	sourceNAD := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rwx-network-762g9",
			Namespace: util.HarvesterSystemNamespaceName,
			Annotations: map[string]string{
				util.RWXNetworkAnnotation: "true",
			},
			Labels: map[string]string{
				util.RWXHashNetworkAnnotation: "test-hash",
			},
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.0/28"]}}`,
		},
	}

	clientset := fakeclientset.NewSimpleClientset(setting)
	_, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), sourceNAD, metav1.CreateOptions{})
	require.NoError(t, err)

	handler := &Handler{
		settingsCache:    fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		nads:             fakeclients.NetworkAttachmentDefinitionClient(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		nadCache:         fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		ipPoolUsages:     fakeclients.IPPoolUsageClient(clientset.HarvesterhciV1beta1().IPPoolUsages),
		ipPoolUsageCache: fakeclients.IPPoolUsageCache(clientset.HarvesterhciV1beta1().IPPoolUsages),
	}

	updated, err := handler.OnRWXNetworkChange("", setting)
	require.NoError(t, err)
	require.NotNil(t, updated)

	staticNAD, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Get(context.Background(), "rwx-network-762g9-static", metav1.GetOptions{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"static"}}`, staticNAD.Spec.Config)

	excludePool, err := clientset.HarvesterhciV1beta1().IPPoolUsages().Get(context.Background(), "harvester-system-rwx-network-762g9-exclude", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "172.16.0.0/28", excludePool.Spec.CIDR)
	assert.Equal(t, util.IPPoolUsageDefaultReservedIPCount, excludePool.Spec.ReservedIPCount)
}
