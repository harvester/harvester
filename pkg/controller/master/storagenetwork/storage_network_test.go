package storagenetwork

import (
	"testing"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	v1alpha1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestCleanUpOldSNIPPool(t *testing.T) {
	tests := []struct {
		title        string
		pools        []runtime.Object
		settingValue string
		expectErr    bool
	}{
		{
			title:        "first configuration of storage network no IP pool detected",
			pools:        nil,
			settingValue: `{"clusterNetwork":"mgmt","vlan":50,"range":"192.168.50.0/24","exclude":[]}`,
			expectErr:    false,
		},
		{
			title: "ip pool from previous setting was detected deletion error with reconciliation expected",
			pools: []runtime.Object{
				&v1alpha1.IPPool{
					ObjectMeta: v1.ObjectMeta{
						Name: "192.168.50.0-24",
					},
				},
			},
			settingValue: `{"clusterNetwork":"mgmt","vlan":50,"range":"192.168.50.0/24","exclude":[]}`,
			expectErr:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			fakeclientset := new(fake.Clientset)
			if test.pools == nil {
				fakeclientset = fake.NewSimpleClientset()
			} else {
				fakeclientset = fake.NewSimpleClientset(test.pools...)
			}

			h := Handler{
				whereaboutsCNIIPPoolClient: fakeclients.IPPoolClient(fakeclientset.WhereaboutsV1alpha1().IPPools),
			}
			err := h.cleanUpOldSNIPPool(&v1beta1.Setting{Value: test.settingValue}, "")
			if test.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

}
