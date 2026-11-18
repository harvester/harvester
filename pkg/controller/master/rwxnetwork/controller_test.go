package rwxnetwork

import (
	"context"
	"testing"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	networkutil "github.com/harvester/harvester/pkg/util/network"
)

func TestSyncHostNetworkConfig(t *testing.T) {
	network := networkutil.BridgeNAD{ClusterNetwork: "mgmt", Vlan: 2011, Range: "172.16.0.0/24"}
	existing := func() *networkv1.HostNetworkConfig {
		return newHostNetworkConfig(network, 24, map[string]string{"node-1": "172.16.0.16", "node-2": "172.16.0.17"})
	}
	node := func(name string) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}
	newHandler := func(objs ...runtime.Object) (*Handler, *fake.Clientset) {
		clientset := fake.NewSimpleClientset(objs...)
		return &Handler{
			hncs:      fakeclients.HostNetworkConfigClient(clientset.NetworkV1beta1().HostNetworkConfigs),
			hncCache:  fakeclients.HostNetworkConfigCache(clientset.NetworkV1beta1().HostNetworkConfigs),
			nodeCache: fakeclients.NodeCache(clientset.CoreV1().Nodes),
		}, clientset
	}
	get := func(t *testing.T, clientset *fake.Clientset) *networkv1.HostNetworkConfig {
		t.Helper()
		hnc, err := clientset.NetworkV1beta1().HostNetworkConfigs().Get(context.TODO(), HostNetworkConfigName, metav1.GetOptions{})
		require.NoError(t, err)
		return hnc
	}

	t.Run("keeps the HostNetworkConfig when no node is eligible", func(t *testing.T) {
		h, clientset := newHandler(existing())

		_, err := h.syncHostNetworkConfig(network, "172.16.0.16/28")
		require.NoError(t, err)
		assert.Equal(t, existing().Spec, get(t, clientset).Spec)
	})

	t.Run("keeps the HostNetworkConfig it cannot update in place", func(t *testing.T) {
		h, clientset := newHandler(existing(), node("node-1"), node("node-2"))
		moved := network
		moved.Vlan = 2012

		_, err := h.syncHostNetworkConfig(moved, "172.16.0.16/28")
		assert.ErrorIs(t, err, errHostNetworkConfigMismatch)
		assert.Equal(t, existing().Spec, get(t, clientset).Spec)
	})

	t.Run("adds a joining node without touching the others", func(t *testing.T) {
		h, clientset := newHandler(existing(), node("node-1"), node("node-2"), node("node-3"))

		_, err := h.syncHostNetworkConfig(network, "172.16.0.16/28")
		require.NoError(t, err)
		assert.Equal(t, map[string]networkv1.IPAddr{
			"node-1": "172.16.0.16/24",
			"node-2": "172.16.0.17/24",
			"node-3": "172.16.0.18/24",
		}, get(t, clientset).Spec.HostIPs)
		assert.Equal(t, "true", get(t, clientset).Labels[util.RWXNetworkManagedLabel])
	})
}

func TestTeardownReleasesExcludesAfterHostNetworkConfig(t *testing.T) {
	const excludes = `["172.16.0.240/29","172.16.0.248/30"]`
	network := networkutil.BridgeNAD{ClusterNetwork: "rwx", Vlan: 2017, Range: "172.16.0.0/24"}
	nad := &nadv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "rwx-network-abcde",
			Namespace:   util.HarvesterSystemNamespaceName,
			Annotations: map[string]string{util.RWXManagedExcludeAnnotation: excludes},
		},
		Spec: nadv1.NetworkAttachmentDefinitionSpec{
			Config: `{"type":"bridge","bridge":"rwx-br","vlan":2017,"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":` + excludes + `}}`,
		},
	}
	clientset := fake.NewSimpleClientset(newHostNetworkConfig(network, 24, map[string]string{"node-1": "172.16.0.240"}))
	nads := clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions
	_, err := nads(nad.Namespace).Create(context.TODO(), nad, metav1.CreateOptions{})
	require.NoError(t, err)
	h := &Handler{
		nads:     fakeclients.NetworkAttachmentDefinitionClient(nads),
		nadCache: fakeclients.NetworkAttachmentDefinitionCache(nads),
		hncs:     fakeclients.HostNetworkConfigClient(clientset.NetworkV1beta1().HostNetworkConfigs),
		hncCache: fakeclients.HostNetworkConfigCache(clientset.NetworkV1beta1().HostNetworkConfigs),
	}
	setting := &harvesterv1.Setting{ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName}}
	getNAD := func(t *testing.T) *nadv1.NetworkAttachmentDefinition {
		t.Helper()
		got, err := nads(nad.Namespace).Get(context.TODO(), nad.Name, metav1.GetOptions{})
		require.NoError(t, err)
		return got
	}

	_, err = h.teardown(setting)
	require.NoError(t, err)
	_, err = clientset.NetworkV1beta1().HostNetworkConfigs().Get(context.TODO(), HostNetworkConfigName, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	assert.Equal(t, excludes, getNAD(t).Annotations[util.RWXManagedExcludeAnnotation])
	assert.Contains(t, getNAD(t).Spec.Config, "exclude")

	_, err = h.teardown(setting)
	require.NoError(t, err)
	assert.NotContains(t, getNAD(t).Annotations, util.RWXManagedExcludeAnnotation)
	assert.NotContains(t, getNAD(t).Spec.Config, "exclude")
}
