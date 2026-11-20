package setting

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

func TestReadinessIsControllerOwned(t *testing.T) {
	old := &harvesterv1.Setting{ObjectMeta: metav1.ObjectMeta{Name: settings.BackupTargetSettingName}}
	next := old.DeepCopy()
	next.Annotations = map[string]string{backuputil.ReadyAnnotation: "forged"}
	require.Error(t, validateSetting(targetRequest(false), next))
	require.Error(t, checkReadyAnnotation(targetRequest(false), old, next))
	require.Error(t, checkReadyAnnotation(targetRequest(false), next, old))
	require.NoError(t, checkReadyAnnotation(targetRequest(false), next, next.DeepCopy()))
	require.NoError(t, checkReadyAnnotation(targetRequest(true), old, next))
	require.NoError(t, checkReadyAnnotation(targetRequest(true), next, old))
}
