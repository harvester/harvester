package settings

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerFlavorSetting(t *testing.T) {
	require.Equal(t, ServerFlavorSettingName, ServerFlavor.Name)
	require.Equal(t, ServerFlavorCommunity, ServerFlavor.Default)
	require.Equal(t, "HARVESTER_SERVER_FLAVOR", GetEnvKey(ServerFlavorSettingName))
}
