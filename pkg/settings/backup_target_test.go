package settings

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackupTargetPassword(t *testing.T) {
	require.NotContains(t, InitBackupTargetToString(), "thirdPartyRepositoryPassword")
	target, err := DecodeBackupTarget(`{"type":"s3","thirdPartyBackupEnabled":true,` +
		`"thirdPartyRepositoryPassword":"  literal$$$(AWS_SECRET_ACCESS_KEY)  "}`)
	require.NoError(t, err)
	require.Equal(t, "  literal$$$(AWS_SECRET_ACCESS_KEY)  ", target.RepoPassword)
	value, err := json.Marshal(target)
	require.NoError(t, err)
	decoded, err := DecodeBackupTarget(string(value))
	require.NoError(t, err)
	require.Equal(t, target, decoded)

	_, err = DecodeBackupTarget(`{"thirdPartyRepositoryPassword":"sensitive-value",`)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "sensitive-value")
}
