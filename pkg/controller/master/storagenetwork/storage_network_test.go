package storagenetwork

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harvester/harvester/pkg/settings"
)

func TestRWXReservedExcludes(t *testing.T) {
	const (
		dedicated = `{"share-storage-network":false,"network":{"vlan":2017,"clusterNetwork":"mgmt","range":"172.16.0.0/24"},"hostIPRange":"172.16.0.16/28","vipRange":"172.16.0.32/28"}`
		shared    = `{"share-storage-network":true,"hostIPRange":"172.16.0.16/28","vipRange":"172.16.0.32/28"}`
		noRanges  = `{"share-storage-network":true}`
	)
	ranges := []string{"172.16.0.16/28", "172.16.0.32/28"}

	tests := []struct {
		name        string
		settingName string
		rwxValue    string
		want        []string
	}{
		{name: "dedicated RWX NAD", settingName: settings.RWXNetworkSettingName, rwxValue: dedicated, want: ranges},
		{name: "storage network NAD in share mode", settingName: settings.StorageNetworkName, rwxValue: shared, want: ranges},
		{name: "storage network NAD without share mode", settingName: settings.StorageNetworkName, rwxValue: dedicated},
		{name: "RWX NAD in share mode", settingName: settings.RWXNetworkSettingName, rwxValue: shared},
		{name: "no ranges", settingName: settings.StorageNetworkName, rwxValue: noRanges},
		{name: "rwx-network not set", settingName: settings.StorageNetworkName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rwxReservedExcludes(tt.settingName, tt.rwxValue)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
