package ippoolusage

import (
	"context"
	"testing"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	fakeclientset "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestOnChangeUpdatesReservedIPsWithDefaultCount(t *testing.T) {
	pool := &harvesterv1beta1.IPPoolUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pool1",
		},
		Spec: harvesterv1beta1.IPPoolUsageSpec{
			CIDR: "172.16.0.1/28",
		},
	}

	clientset := fakeclientset.NewSimpleClientset(pool)
	handler := &Handler{
		ipPoolUsages: fakeclients.IPPoolUsageClient(clientset.HarvesterhciV1beta1().IPPoolUsages),
	}

	updated, err := handler.OnChange("", pool)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actual, err := clientset.HarvesterhciV1beta1().IPPoolUsages().Get(context.Background(), pool.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"172.16.0.1",
		"172.16.0.2",
		"172.16.0.3",
		"172.16.0.4",
		"172.16.0.5",
		"172.16.0.6",
		"172.16.0.7",
		"172.16.0.8",
	}, actual.Status.ReservedIPs)
}

func TestOnChangeUpdatesReservedIPsWithExplicitCount(t *testing.T) {
	pool := &harvesterv1beta1.IPPoolUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pool1",
		},
		Spec: harvesterv1beta1.IPPoolUsageSpec{
			CIDR:            "172.16.0.1/28",
			ReservedIPCount: 2,
		},
	}

	clientset := fakeclientset.NewSimpleClientset(pool)
	handler := &Handler{
		ipPoolUsages: fakeclients.IPPoolUsageClient(clientset.HarvesterhciV1beta1().IPPoolUsages),
	}

	updated, err := handler.OnChange("", pool)
	require.NoError(t, err)
	require.NotNil(t, updated)

	actual, err := clientset.HarvesterhciV1beta1().IPPoolUsages().Get(context.Background(), pool.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"172.16.0.1",
		"172.16.0.2",
	}, actual.Status.ReservedIPs)
}
