package sharemanager

import (
	"context"
	"net"
	"testing"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	fakeclientset "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestOnShareManagerChangeAnnotatesStaticIP(t *testing.T) {
	shareManager := &longhornv1beta2.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-test",
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.ShareManagerStaticIPAnnotation: "true",
				util.ShareManagerIfaceAnnotation:    "lhnet1",
			},
		},
	}
	storageNetworkSetting := &harvesterv1beta1.Setting{
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
				"network.harvesterhci.io/type": "L2VlanNetwork",
			},
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28"]}}`,
		},
	}
	staticNAD := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "storagenetwork-frqx7-static",
			Namespace: util.HarvesterSystemNamespaceName,
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"static"}}`,
		},
	}
	excludePool := &harvesterv1beta1.IPPoolUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-system-storagenetwork-frqx7-exclude",
		},
		Spec: harvesterv1beta1.IPPoolUsageSpec{
			CIDR:            "172.16.0.1/28",
			ReservedIPCount: util.IPPoolUsageDefaultReservedIPCount,
		},
	}

	clientset := fakeclientset.NewSimpleClientset(shareManager, storageNetworkSetting, excludePool)
	_, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), sourceNAD, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), staticNAD, metav1.CreateOptions{})
	require.NoError(t, err)
	handler := &Handler{
		shareManagers:    fakeclients.LonghornShareManagerClient(clientset.LonghornV1beta2().ShareManagers),
		settingsCache:    fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		nadCache:         fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		ipPoolUsages:     fakeclients.IPPoolUsageClient(clientset.HarvesterhciV1beta1().IPPoolUsages),
		ipPoolUsageCache: fakeclients.IPPoolUsageCache(clientset.HarvesterhciV1beta1().IPPoolUsages),
	}

	updated, err := handler.OnShareManagerChange("", shareManager)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "172.16.0.9/24", actualShareManager.Annotations[util.ShareManagerIPAnnotation])
	assert.Equal(t, "harvester-system/storagenetwork-frqx7", actualShareManager.Annotations[util.ShareManagerNADAnnotation])
	assert.Equal(t, "harvester-system/storagenetwork-frqx7-static", actualShareManager.Annotations[util.ShareManagerStaticNADAnnotation])
	mac, err := net.ParseMAC(actualShareManager.Annotations[util.ShareManagerMACAnnotation])
	require.NoError(t, err)
	assert.Len(t, mac, 6)
	assert.Equal(t, byte(0x02), mac[0]&0x03)

	staticNAD, err = clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Get(context.Background(), "storagenetwork-frqx7-static", metav1.GetOptions{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"static"}}`, staticNAD.Spec.Config)

	excludePool, err = clientset.HarvesterhciV1beta1().IPPoolUsages().Get(context.Background(), "harvester-system-storagenetwork-frqx7-exclude", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "172.16.0.1/28", excludePool.Spec.CIDR)
	assert.Equal(t, harvesterv1beta1.IPPoolUsageResourceRef{
		APIVersion: longhornv1beta2.SchemeGroupVersion.String(),
		Kind:       shareManagerKind,
		Namespace:  util.LonghornSystemNamespaceName,
		Name:       "pvc-test",
	}, excludePool.Status.Allocations["172.16.0.9"].Resource)
}

func TestOnShareManagerChangeUsesDedicatedRWXNetwork(t *testing.T) {
	shareManager := &longhornv1beta2.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-test",
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.ShareManagerStaticIPAnnotation: "true",
				util.ShareManagerIfaceAnnotation:    "lhnet1",
			},
		},
	}
	rwxNetworkSetting := &harvesterv1beta1.Setting{
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
				"network.harvesterhci.io/type": "L2VlanNetwork",
			},
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.0/28"]}}`,
		},
	}
	staticNAD := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rwx-network-762g9-static",
			Namespace: util.HarvesterSystemNamespaceName,
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"static"}}`,
		},
	}
	excludePool := &harvesterv1beta1.IPPoolUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-system-rwx-network-762g9-exclude",
		},
		Spec: harvesterv1beta1.IPPoolUsageSpec{
			CIDR:            "172.16.0.0/28",
			ReservedIPCount: util.IPPoolUsageDefaultReservedIPCount,
		},
	}

	clientset := fakeclientset.NewSimpleClientset(shareManager, rwxNetworkSetting, excludePool)
	_, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), sourceNAD, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), staticNAD, metav1.CreateOptions{})
	require.NoError(t, err)
	handler := &Handler{
		shareManagers:    fakeclients.LonghornShareManagerClient(clientset.LonghornV1beta2().ShareManagers),
		settingsCache:    fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		nadCache:         fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		ipPoolUsages:     fakeclients.IPPoolUsageClient(clientset.HarvesterhciV1beta1().IPPoolUsages),
		ipPoolUsageCache: fakeclients.IPPoolUsageCache(clientset.HarvesterhciV1beta1().IPPoolUsages),
	}

	updated, err := handler.OnShareManagerChange("", shareManager)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "172.16.0.9/24", actualShareManager.Annotations[util.ShareManagerIPAnnotation])
	assert.Equal(t, "harvester-system/rwx-network-762g9", actualShareManager.Annotations[util.ShareManagerNADAnnotation])
	assert.Equal(t, "harvester-system/rwx-network-762g9-static", actualShareManager.Annotations[util.ShareManagerStaticNADAnnotation])
	mac, err := net.ParseMAC(actualShareManager.Annotations[util.ShareManagerMACAnnotation])
	require.NoError(t, err)
	assert.Len(t, mac, 6)
	assert.Equal(t, byte(0x02), mac[0]&0x03)
}

func TestOnShareManagerChangeReusesExistingExcludePoolAllocation(t *testing.T) {
	shareManager := &longhornv1beta2.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-test",
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.ShareManagerStaticIPAnnotation:  "true",
				util.ShareManagerIfaceAnnotation:     "lhnet1",
				util.ShareManagerMACAnnotation:       "02:00:00:00:00:01",
				util.ShareManagerNADAnnotation:       "harvester-system/storagenetwork-frqx7",
				util.ShareManagerStaticNADAnnotation: "harvester-system/storagenetwork-frqx7-static",
			},
		},
	}
	storageNetworkSetting := &harvesterv1beta1.Setting{
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
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28"]}}`,
		},
	}
	staticNAD := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "storagenetwork-frqx7-static",
			Namespace: util.HarvesterSystemNamespaceName,
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"static"}}`,
		},
	}
	excludePool := &harvesterv1beta1.IPPoolUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-system-storagenetwork-frqx7-exclude",
		},
		Spec: harvesterv1beta1.IPPoolUsageSpec{
			CIDR:            "172.16.0.1/28",
			ReservedIPCount: util.IPPoolUsageDefaultReservedIPCount,
		},
		Status: harvesterv1beta1.IPPoolUsageStatus{
			Allocations: map[string]harvesterv1beta1.IPPoolAllocation{
				"172.16.0.5": {
					Resource: harvesterv1beta1.IPPoolUsageResourceRef{
						APIVersion: longhornv1beta2.SchemeGroupVersion.String(),
						Kind:       shareManagerKind,
						Namespace:  util.LonghornSystemNamespaceName,
						Name:       "pvc-test",
					},
				},
			},
		},
	}

	clientset := fakeclientset.NewSimpleClientset(shareManager, storageNetworkSetting, excludePool)
	_, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), sourceNAD, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), staticNAD, metav1.CreateOptions{})
	require.NoError(t, err)
	handler := &Handler{
		shareManagers:    fakeclients.LonghornShareManagerClient(clientset.LonghornV1beta2().ShareManagers),
		settingsCache:    fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		nadCache:         fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		ipPoolUsages:     fakeclients.IPPoolUsageClient(clientset.HarvesterhciV1beta1().IPPoolUsages),
		ipPoolUsageCache: fakeclients.IPPoolUsageCache(clientset.HarvesterhciV1beta1().IPPoolUsages),
	}

	updated, err := handler.OnShareManagerChange("", shareManager)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "172.16.0.5/24", actualShareManager.Annotations[util.ShareManagerIPAnnotation])
	assert.Equal(t, "02:00:00:00:00:01", actualShareManager.Annotations[util.ShareManagerMACAnnotation])
}

func TestOnShareManagerChangeClearsAnnotationsWithoutExcludeRange(t *testing.T) {
	shareManager := &longhornv1beta2.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-test",
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.ShareManagerStaticIPAnnotation:  "true",
				util.ShareManagerIfaceAnnotation:     "lhnet1",
				util.ShareManagerIPAnnotation:        "172.16.0.9/24",
				util.ShareManagerMACAnnotation:       "02:00:00:00:00:01",
				util.ShareManagerNADAnnotation:       "harvester-system/storagenetwork-frqx7",
				util.ShareManagerStaticNADAnnotation: "harvester-system/storagenetwork-frqx7-static",
			},
		},
	}
	storageNetworkSetting := &harvesterv1beta1.Setting{
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
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24"}}`,
		},
	}

	clientset := fakeclientset.NewSimpleClientset(shareManager, storageNetworkSetting)
	_, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), sourceNAD, metav1.CreateOptions{})
	require.NoError(t, err)
	handler := &Handler{
		shareManagers: fakeclients.LonghornShareManagerClient(clientset.LonghornV1beta2().ShareManagers),
		settingsCache: fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		nadCache:      fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
	}

	updated, err := handler.OnShareManagerChange("", shareManager)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerIPAnnotation])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerMACAnnotation])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerNADAnnotation])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerStaticNADAnnotation])
	assert.Equal(t, shareManagerStaticIPStatusNoExcludeRange, actualShareManager.Annotations[util.ShareManagerStaticIPStatusAnnotation])
}

func TestOnShareManagerChangeClearsAnnotationsUntilDerivedResourcesExist(t *testing.T) {
	shareManager := &longhornv1beta2.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-test",
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.ShareManagerStaticIPAnnotation:  "true",
				util.ShareManagerIfaceAnnotation:     "lhnet1",
				util.ShareManagerIPAnnotation:        "172.16.0.9/24",
				util.ShareManagerMACAnnotation:       "02:00:00:00:00:01",
				util.ShareManagerNADAnnotation:       "harvester-system/storagenetwork-frqx7",
				util.ShareManagerStaticNADAnnotation: "harvester-system/storagenetwork-frqx7-static",
			},
		},
	}
	storageNetworkSetting := &harvesterv1beta1.Setting{
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
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28"]}}`,
		},
	}

	clientset := fakeclientset.NewSimpleClientset(shareManager, storageNetworkSetting)
	_, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), sourceNAD, metav1.CreateOptions{})
	require.NoError(t, err)
	handler := &Handler{
		shareManagers:    fakeclients.LonghornShareManagerClient(clientset.LonghornV1beta2().ShareManagers),
		settingsCache:    fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		nadCache:         fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		ipPoolUsageCache: fakeclients.IPPoolUsageCache(clientset.HarvesterhciV1beta1().IPPoolUsages),
	}

	updated, err := handler.OnShareManagerChange("", shareManager)
	require.NoError(t, err)
	assert.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerIPAnnotation])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerMACAnnotation])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerNADAnnotation])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerStaticNADAnnotation])
	assert.Equal(t, shareManagerStaticIPStatusStaticNADPending, actualShareManager.Annotations[util.ShareManagerStaticIPStatusAnnotation])
}

func TestOnShareManagerRemoveReleasesExcludePoolAllocation(t *testing.T) {
	shareManager := &longhornv1beta2.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-test",
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.ShareManagerNADAnnotation:   "harvester-system/storagenetwork-frqx7",
				util.ShareManagerIfaceAnnotation: "lhnet1",
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
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28"]}}`,
		},
	}
	excludePool := &harvesterv1beta1.IPPoolUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-system-storagenetwork-frqx7-exclude",
		},
		Spec: harvesterv1beta1.IPPoolUsageSpec{
			CIDR:            "172.16.0.1/28",
			ReservedIPCount: util.IPPoolUsageDefaultReservedIPCount,
		},
		Status: harvesterv1beta1.IPPoolUsageStatus{
			Allocations: map[string]harvesterv1beta1.IPPoolAllocation{
				"172.16.0.5": {
					Resource: harvesterv1beta1.IPPoolUsageResourceRef{
						APIVersion: longhornv1beta2.SchemeGroupVersion.String(),
						Kind:       shareManagerKind,
						Namespace:  util.LonghornSystemNamespaceName,
						Name:       "pvc-test",
					},
				},
				"172.16.0.6": {
					Resource: harvesterv1beta1.IPPoolUsageResourceRef{
						APIVersion: "v1",
						Kind:       "Pod",
						Namespace:  "default",
						Name:       "other",
					},
				},
			},
		},
	}

	clientset := fakeclientset.NewSimpleClientset(shareManager, excludePool)
	_, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), sourceNAD, metav1.CreateOptions{})
	require.NoError(t, err)
	handler := &Handler{
		ipPoolUsages:     fakeclients.IPPoolUsageClient(clientset.HarvesterhciV1beta1().IPPoolUsages),
		ipPoolUsageCache: fakeclients.IPPoolUsageCache(clientset.HarvesterhciV1beta1().IPPoolUsages),
		nadCache:         fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
	}

	_, err = handler.OnShareManagerRemove("", shareManager)
	require.NoError(t, err)

	updatedPool, err := clientset.HarvesterhciV1beta1().IPPoolUsages().Get(context.Background(), excludePool.Name, metav1.GetOptions{})
	require.NoError(t, err)
	_, exists := updatedPool.Status.Allocations["172.16.0.5"]
	assert.False(t, exists)
	_, exists = updatedPool.Status.Allocations["172.16.0.6"]
	assert.True(t, exists)
}
