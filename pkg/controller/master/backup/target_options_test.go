package backup

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
)

func TestPreparationRejectsOptions(t *testing.T) {
	for _, tc := range []struct {
		name, cert, setting, value, want string
		virtual                          bool
	}{
		{name: "certificate", cert: "test-ca", want: "backup-target.cert"},
		{name: "virtual host", virtual: true, want: "virtualHostedStyle=true"},
		{name: "additional CA", setting: settings.AdditionalCASettingName, value: "test-ca", want: "additional-ca"},
		{name: "HTTP proxy", setting: settings.HTTPProxySettingName, value: `{"httpProxy":"http://proxy"}`, want: "http-proxy"},
		{name: "HTTPS proxy", setting: settings.HTTPProxySettingName, value: `{"httpsProxy":"http://proxy"}`, want: "http-proxy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, masked := range []bool{false, true} {
				h, setting, target, _ := credentialHandler(t)
				submitRepository(t, h, setting, target)
				target.Cert, target.VirtualHostedStyle = tc.cert, tc.virtual
				if masked {
					target.AccessKeyID, target.SecretAccessKey, target.RepoPassword = "", "", ""
				}
				setTargetValue(t, h, setting, target)
				if tc.setting != "" {
					_, err := h.settings.Create(&harvesterv1.Setting{
						ObjectMeta: metav1.ObjectMeta{Name: tc.setting}, Value: tc.value,
					})
					require.NoError(t, err)
				}
				original := readCredentials(t, h)
				calls := 0
				h.initRepos = func(context.Context, *settings.BackupTarget, map[string][]byte) error { calls++; return nil }
				h.connectRepos = h.initRepos
				_, err := h.OnBackupTargetChange(setting.Name, setting)
				require.NoError(t, err)
				require.Zero(t, calls)
				require.Equal(t, original, readCredentials(t, h))
				current := currentTarget(t, h)
				require.Equal(t, setting.Value, current.Value)
				require.True(t, harvesterv1.SettingConfigured.IsFalse(current))
				require.Contains(t, harvesterv1.SettingConfigured.GetMessage(current), tc.want)

				// Longhorn-only S3 targets retain their existing connection support.
				target.ThirdPartyBackupEnabled, target.RepoPassword = false, ""
				setTargetValue(t, h, current, target)
				_, err = h.OnBackupTargetChange(current.Name, current)
				require.NoError(t, err)
				require.Zero(t, calls)
			}
		})
	}
}

func TestMaintenanceStopsForProxy(t *testing.T) {
	prepareKopiaMaintenanceTest(t)
	client := kubefake.NewSimpleClientset()
	h := maintenanceHandler(t, client)
	target := kopiaMaintenanceTarget(true)
	require.NoError(t, h.reconcileKopiaMaintenance(target))
	_, err := h.settings.Create(&harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.HTTPProxySettingName},
		Value:      `{"httpsProxy":"http://proxy"}`,
	})
	require.NoError(t, err)
	require.NoError(t, h.reconcileKopiaMaintenance(target))
	for _, name := range []string{kopiautil.QuickMaintenanceCronJobName, kopiautil.FullMaintenanceCronJobName} {
		_, err := client.BatchV1().CronJobs(util.LonghornSystemNamespaceName).Get(h.ctx, name, metav1.GetOptions{})
		require.True(t, apierrors.IsNotFound(err))
	}
}
