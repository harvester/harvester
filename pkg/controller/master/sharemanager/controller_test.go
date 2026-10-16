package sharemanager

import (
	"context"
	"testing"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	fakeclientset "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestOnShareManagerChangeLearnsPodNetworkIdentityAndUpdatesSourceNAD(t *testing.T) {
	shareManager := newStaticIPShareManager()
	rwxNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXNetworkSettingName,
		},
		Value: `{"share-storage-network":true}`,
	}
	storageNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.StorageNetworkName,
			Annotations: map[string]string{
				util.NadStorageNetworkAnnotation: "harvester-system/storagenetwork-frqx7",
			},
		},
	}
	sourceNAD := newSourceNAD(
		"storagenetwork-frqx7",
		map[string]string{util.StorageNetworkAnnotation: "true"},
		`{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28"]}}`,
	)
	staticNAD := newStaticNADObject("storagenetwork-frqx7-static")
	pod := newShareManagerPod(`[{"interface":"lhnet1","ips":["172.16.0.22"],"mac":"02:00:00:00:00:01"}]`)

	clientset := fakeclientset.NewSimpleClientset(shareManager, rwxNetworkSetting, storageNetworkSetting, pod)
	createNAD(t, clientset, sourceNAD)
	createNAD(t, clientset, staticNAD)

	handler := newTestHandler(clientset)
	updated, err := handler.OnShareManagerChange("", shareManager)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "harvester-system/storagenetwork-frqx7", actualShareManager.Annotations[util.ShareManagerNADAnno])
	assert.Equal(t, "harvester-system/storagenetwork-frqx7-static", actualShareManager.Annotations[util.ShareManagerStaticNADAnno])
	assert.Equal(t, "172.16.0.22/24", actualShareManager.Annotations[util.ShareManagerIPAnno])
	assert.Equal(t, "02:00:00:00:00:01", actualShareManager.Annotations[util.ShareManagerMACAnno])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerStaticIPStatusAnno])

	updatedSourceNAD, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Get(context.Background(), sourceNAD.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28","172.16.0.22/32"]}}`,
		updatedSourceNAD.Spec.Config,
	)
}

func TestOnShareManagerChangeUsesDedicatedRWXNetwork(t *testing.T) {
	shareManager := newStaticIPShareManager()
	rwxNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXNetworkSettingName,
			Annotations: map[string]string{
				util.RWXNadNetworkAnnotation: "harvester-system/rwx-network-762g9",
			},
		},
		Value: `{"share-storage-network":false,"network":{"vlan":2011,"clusterNetwork":"mgmt","range":"172.16.0.0/24","exclude":["172.16.0.0/28"]}}`,
	}
	sourceNAD := newSourceNAD(
		"rwx-network-762g9",
		map[string]string{util.RWXNetworkAnnotation: "true"},
		`{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.0/28"]}}`,
	)
	staticNAD := newStaticNADObject("rwx-network-762g9-static")
	pod := newShareManagerPod(`[{"interface":"lhnet1","ips":["172.16.0.22"],"mac":"02:00:00:00:00:01"}]`)

	clientset := fakeclientset.NewSimpleClientset(shareManager, rwxNetworkSetting, pod)
	createNAD(t, clientset, sourceNAD)
	createNAD(t, clientset, staticNAD)

	handler := newTestHandler(clientset)
	updated, err := handler.OnShareManagerChange("", shareManager)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "harvester-system/rwx-network-762g9", actualShareManager.Annotations[util.ShareManagerNADAnno])
	assert.Equal(t, "harvester-system/rwx-network-762g9-static", actualShareManager.Annotations[util.ShareManagerStaticNADAnno])
	assert.Equal(t, "172.16.0.22/24", actualShareManager.Annotations[util.ShareManagerIPAnno])
	assert.Equal(t, "02:00:00:00:00:01", actualShareManager.Annotations[util.ShareManagerMACAnno])
}

func TestOnShareManagerChangeWaitsForPodIdentity(t *testing.T) {
	shareManager := newStaticIPShareManager()
	rwxNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXNetworkSettingName,
		},
		Value: `{"share-storage-network":true}`,
	}
	storageNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.StorageNetworkName,
			Annotations: map[string]string{
				util.NadStorageNetworkAnnotation: "harvester-system/storagenetwork-frqx7",
			},
		},
	}
	sourceNAD := newSourceNAD(
		"storagenetwork-frqx7",
		map[string]string{util.StorageNetworkAnnotation: "true"},
		`{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28"]}}`,
	)
	staticNAD := newStaticNADObject("storagenetwork-frqx7-static")

	clientset := fakeclientset.NewSimpleClientset(shareManager, rwxNetworkSetting, storageNetworkSetting)
	createNAD(t, clientset, sourceNAD)
	createNAD(t, clientset, staticNAD)

	handler := newTestHandler(clientset)
	updated, err := handler.OnShareManagerChange("", shareManager)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "harvester-system/storagenetwork-frqx7", actualShareManager.Annotations[util.ShareManagerNADAnno])
	assert.Equal(t, "harvester-system/storagenetwork-frqx7-static", actualShareManager.Annotations[util.ShareManagerStaticNADAnno])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerIPAnno])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerMACAnno])
	assert.Equal(t, statusWaitingForPod, actualShareManager.Annotations[util.ShareManagerStaticIPStatusAnno])

	updatedSourceNAD, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Get(context.Background(), sourceNAD.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.JSONEq(t, sourceNAD.Spec.Config, updatedSourceNAD.Spec.Config)
}

func TestOnShareManagerChangePreservesLearnedIdentityWhilePodIsAbsent(t *testing.T) {
	shareManager := newStaticIPShareManager()
	shareManager.Annotations[util.ShareManagerNADAnno] = "harvester-system/storagenetwork-frqx7"
	shareManager.Annotations[util.ShareManagerStaticNADAnno] = "harvester-system/storagenetwork-frqx7-static"
	shareManager.Annotations[util.ShareManagerIPAnno] = "172.16.0.22/24"
	shareManager.Annotations[util.ShareManagerMACAnno] = "02:00:00:00:00:01"

	rwxNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXNetworkSettingName,
		},
		Value: `{"share-storage-network":true}`,
	}
	storageNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.StorageNetworkName,
			Annotations: map[string]string{
				util.NadStorageNetworkAnnotation: "harvester-system/storagenetwork-frqx7",
			},
		},
	}
	sourceNAD := newSourceNAD(
		"storagenetwork-frqx7",
		map[string]string{util.StorageNetworkAnnotation: "true"},
		`{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28","172.16.0.22/32"]}}`,
	)
	staticNAD := newStaticNADObject("storagenetwork-frqx7-static")

	clientset := fakeclientset.NewSimpleClientset(shareManager, rwxNetworkSetting, storageNetworkSetting)
	createNAD(t, clientset, sourceNAD)
	createNAD(t, clientset, staticNAD)

	handler := newTestHandler(clientset)
	updated, err := handler.OnShareManagerChange("", shareManager)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "harvester-system/storagenetwork-frqx7", actualShareManager.Annotations[util.ShareManagerNADAnno])
	assert.Equal(t, "harvester-system/storagenetwork-frqx7-static", actualShareManager.Annotations[util.ShareManagerStaticNADAnno])
	assert.Equal(t, "172.16.0.22/24", actualShareManager.Annotations[util.ShareManagerIPAnno])
	assert.Equal(t, "02:00:00:00:00:01", actualShareManager.Annotations[util.ShareManagerMACAnno])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerStaticIPStatusAnno])
}

func TestOnShareManagerDefaultOptInAddsAnnotations(t *testing.T) {
	shareManager := newShareManager()
	rwxNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXNetworkSettingName,
		},
		Value: `{"share-storage-network":true}`,
	}
	storageNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.StorageNetworkName,
			Annotations: map[string]string{
				util.NadStorageNetworkAnnotation: "harvester-system/storagenetwork-frqx7",
			},
		},
	}

	clientset := fakeclientset.NewSimpleClientset(shareManager, rwxNetworkSetting, storageNetworkSetting)
	handler := newTestHandler(clientset)

	updated, err := handler.OnShareManagerDefaultOptIn("", shareManager)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "true", actualShareManager.Annotations[util.ShareManagerStaticIPAnno])
	assert.Equal(t, defaultShareManagerIface, actualShareManager.Annotations[util.ShareManagerIfaceAnno])
}

func TestOnShareManagerDefaultOptInSkipsEmptyRWXSettingValue(t *testing.T) {
	shareManager := newShareManager()
	rwxNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXNetworkSettingName,
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

	clientset := fakeclientset.NewSimpleClientset(shareManager, rwxNetworkSetting, storageNetworkSetting)
	handler := newTestHandler(clientset)

	updated, err := handler.OnShareManagerDefaultOptIn("", shareManager)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerStaticIPAnno])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerIfaceAnno])
}

func TestOnShareManagerRemoveRemovesSourceNADExclude(t *testing.T) {
	shareManager := &longhornv1beta2.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-test",
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.ShareManagerNADAnno: "harvester-system/storagenetwork-frqx7",
				util.ShareManagerIPAnno:  "172.16.0.22/24",
			},
		},
	}
	sourceNAD := newSourceNAD(
		"storagenetwork-frqx7",
		map[string]string{util.StorageNetworkAnnotation: "true"},
		`{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28","172.16.0.22/32"]}}`,
	)

	clientset := fakeclientset.NewSimpleClientset(shareManager)
	createNAD(t, clientset, sourceNAD)

	handler := newTestHandler(clientset)
	_, err := handler.OnShareManagerRemove("", shareManager)
	require.NoError(t, err)

	updatedSourceNAD, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Get(context.Background(), sourceNAD.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28"]}}`,
		updatedSourceNAD.Spec.Config,
	)
}

func TestOnPodRemoveReturnsNilWhenRWXSettingIsNotConfigured(t *testing.T) {
	shareManager := newShareManager()
	pod := newShareManagerPod("")
	pod.DeletionTimestamp = &metav1.Time{}

	clientset := fakeclientset.NewSimpleClientset(shareManager, pod)
	handler := newTestHandler(clientset)

	removedPod, err := handler.OnPodRemove("", pod)
	require.NoError(t, err)
	assert.Nil(t, removedPod)
}

func TestOnPodRemoveRequeuesWhenShareManagerNeedsDefaultOptIn(t *testing.T) {
	shareManager := newShareManager()
	rwxNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXNetworkSettingName,
		},
		Value: `{"share-storage-network":true}`,
	}
	storageNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.StorageNetworkName,
			Annotations: map[string]string{
				util.NadStorageNetworkAnnotation: "harvester-system/storagenetwork-frqx7",
			},
		},
	}
	pod := newShareManagerPod("")
	pod.DeletionTimestamp = &metav1.Time{}

	clientset := fakeclientset.NewSimpleClientset(shareManager, rwxNetworkSetting, storageNetworkSetting, pod)
	handler := newTestHandler(clientset)

	removedPod, err := handler.OnPodRemove("", pod)
	require.Error(t, err)
	assert.Equal(t, pod, removedPod)
}

func TestOnPodRemoveBlocksWhileWaitingForShareManagerAnnotations(t *testing.T) {
	shareManager := newStaticIPShareManager()
	rwxNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.RWXNetworkSettingName,
		},
		Value: `{"share-storage-network":true}`,
	}
	storageNetworkSetting := &harvesterv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.StorageNetworkName,
			Annotations: map[string]string{
				util.NadStorageNetworkAnnotation: "harvester-system/storagenetwork-frqx7",
			},
		},
	}
	sourceNAD := newSourceNAD(
		"storagenetwork-frqx7",
		map[string]string{util.StorageNetworkAnnotation: "true"},
		`{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/28"]}}`,
	)
	staticNAD := newStaticNADObject("storagenetwork-frqx7-static")
	pod := newShareManagerPod("")
	pod.DeletionTimestamp = &metav1.Time{}
	pod.Annotations = map[string]string{
		networkv1.NetworkAttachmentAnnot: `[{"name":"storagenetwork-frqx7-static","namespace":"harvester-system","ips":["172.16.0.22/24"],"mac":"02:00:00:00:00:01","interface":"lhnet1"}]`,
	}

	clientset := fakeclientset.NewSimpleClientset(shareManager, rwxNetworkSetting, storageNetworkSetting, pod)
	createNAD(t, clientset, sourceNAD)
	createNAD(t, clientset, staticNAD)

	handler := newTestHandler(clientset)
	removedPod, err := handler.OnPodRemove("", pod)
	require.Error(t, err)
	assert.Equal(t, pod, removedPod)

	actualShareManager, err := clientset.LonghornV1beta2().ShareManagers(util.LonghornSystemNamespaceName).Get(context.Background(), shareManager.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerNADAnno])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerStaticNADAnno])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerIPAnno])
	assert.Empty(t, actualShareManager.Annotations[util.ShareManagerMACAnno])
}

func TestOnPodRemoveAllowsDeletionAfterShareManagerAnnotationsReady(t *testing.T) {
	shareManager := newStaticIPShareManager()
	shareManager.Annotations[util.ShareManagerNADAnno] = "harvester-system/storagenetwork-frqx7"
	shareManager.Annotations[util.ShareManagerStaticNADAnno] = "harvester-system/storagenetwork-frqx7-static"
	shareManager.Annotations[util.ShareManagerIPAnno] = "172.16.0.22/24"
	shareManager.Annotations[util.ShareManagerMACAnno] = "02:00:00:00:00:01"

	pod := newShareManagerPod("")
	pod.DeletionTimestamp = &metav1.Time{}
	pod.Annotations = map[string]string{
		networkv1.NetworkAttachmentAnnot: `[{"name":"storagenetwork-frqx7-static","namespace":"harvester-system","ips":["172.16.0.22/24"],"mac":"02:00:00:00:00:01","interface":"lhnet1"}]`,
	}

	clientset := fakeclientset.NewSimpleClientset(shareManager, pod)
	handler := newTestHandler(clientset)

	removedPod, err := handler.OnPodRemove("", pod)
	require.NoError(t, err)
	assert.Nil(t, removedPod)
}

func newTestHandler(clientset *fakeclientset.Clientset) *Handler {
	return &Handler{
		shareManagers:     fakeclients.LonghornShareManagerClient(clientset.LonghornV1beta2().ShareManagers),
		shareManagerCache: fakeclients.LonghornShareManagerCache(clientset.LonghornV1beta2().ShareManagers),
		settingsCache:     fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		podCache:          fakeclients.PodCache(clientset.CoreV1().Pods),
		nads:              fakeclients.NetworkAttachmentDefinitionClient(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
		nadCache:          fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
	}
}

func newShareManager() *longhornv1beta2.ShareManager {
	return &longhornv1beta2.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-test",
			Namespace: util.LonghornSystemNamespaceName,
		},
	}
}

func newStaticIPShareManager() *longhornv1beta2.ShareManager {
	shareManager := newShareManager()
	shareManager.Annotations = map[string]string{
		util.ShareManagerStaticIPAnno: "true",
		util.ShareManagerIfaceAnno:    "lhnet1",
	}
	return shareManager
}

func newShareManagerPod(networkStatus string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "share-manager-pvc-test",
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				networkv1.NetworkStatusAnnot: networkStatus,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "longhorn.io/v1beta2",
				Kind:       shareManagerKind,
				Name:       "pvc-test",
			}},
		},
	}
}

func newSourceNAD(name string, annotations map[string]string, config string) *networkv1.NetworkAttachmentDefinition {
	return &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   util.HarvesterSystemNamespaceName,
			Annotations: annotations,
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: config,
		},
	}
}

func newStaticNADObject(name string) *networkv1.NetworkAttachmentDefinition {
	return &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: util.HarvesterSystemNamespaceName,
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":2011,"ipam":{"type":"static"}}`,
		},
	}
}

func createNAD(t *testing.T, clientset *fakeclientset.Clientset, nad *networkv1.NetworkAttachmentDefinition) {
	t.Helper()
	_, err := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions(util.HarvesterSystemNamespaceName).Create(context.Background(), nad, metav1.CreateOptions{})
	require.NoError(t, err)
}
