package repoinfo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestHarvesterReleaseServerFlavor(t *testing.T) {
	release := HarvesterRelease{}
	require.NoError(t, yaml.Unmarshal([]byte("serverFlavor: prime\n"), &release))
	require.Equal(t, "prime", release.ServerFlavor)

	data, err := (&RepoInfo{Release: release}).Marshall()
	require.NoError(t, err)

	loaded := RepoInfo{}
	require.NoError(t, loaded.Load(data))
	require.Equal(t, release.ServerFlavor, loaded.Release.ServerFlavor)
}
