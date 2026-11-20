package backup_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestTargetOptions(t *testing.T) {
	for _, tc := range []struct {
		name, cert, ca, proxy, want string
		virtual                     bool
	}{
		{name: "defaults"},
		{name: "certificate", cert: "private-ca", want: "backup-target.cert"},
		{name: "virtual host", virtual: true, want: "virtualHostedStyle=true"},
		{name: "additional CA", ca: "private-ca", want: "additional-ca"},
		{name: "HTTP proxy", proxy: `{"httpProxy":"http://user:private-password@proxy"}`, want: "http-proxy"},
		{name: "HTTPS proxy", proxy: `{"httpsProxy":"http://user:private-password@proxy"}`, want: "http-proxy"},
		{name: "no proxy only", proxy: `{"noProxy":"localhost,.svc"}`},
		{name: "blank options", cert: " \n", ca: " \n", proxy: `{"httpProxy":" ","httpsProxy":" "}`},
		{name: "null proxy", proxy: "null"},
		{name: "invalid proxy", proxy: `{"httpsProxy":"private-password"`, want: "invalid http-proxy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(
				&harvesterv1.Setting{ObjectMeta: metav1.ObjectMeta{Name: settings.AdditionalCASettingName}, Value: tc.ca},
				&harvesterv1.Setting{ObjectMeta: metav1.ObjectMeta{Name: settings.HTTPProxySettingName}, Value: tc.proxy},
			)
			reader := backuputil.TargetClient{Settings: fakeclients.HarvesterSettingClient(client.HarvesterhciV1beta1().Settings)}
			err := reader.CheckThirdPartyOptions(&settings.BackupTarget{Cert: tc.cert, VirtualHostedStyle: tc.virtual})
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.want)
			require.NotContains(t, err.Error(), "private-")
		})
	}
}

func TestOptionsUseSettingDefaults(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{settings.AdditionalCASettingName, "private-ca"},
		{settings.HTTPProxySettingName, `{"httpProxy":"http://proxy"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(&harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: tc.name}, Default: tc.value,
			})
			require.ErrorContains(t, credentialReader(client).CheckThirdPartyOptions(&settings.BackupTarget{}), tc.name)
		})
	}
}

func TestCredentialOptionsReadLiveSettings(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{settings.AdditionalCASettingName, "private-ca"},
		{settings.HTTPProxySettingName, `{"httpsProxy":"http://proxy"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, target := credentialFixture(t)
			reader := credentialReader(client)
			_, err := reader.ThirdPartyCredentials(target, backuputil.ResticPasswordKey)
			require.NoError(t, err)
			_, err = reader.Settings.Create(&harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: tc.name}, Value: tc.value,
			})
			require.NoError(t, err)
			for _, key := range []string{backuputil.ResticPasswordKey, backuputil.KopiaPasswordKey} {
				client.ClearActions()
				_, err = reader.ThirdPartyCredentials(target, key)
				require.ErrorContains(t, err, tc.name)
				for _, action := range client.Actions() {
					require.NotEqual(t, "secrets", action.GetResource().Resource)
				}
			}
			// Native Longhorn uses its existing transport support.
			require.NoError(t, reader.CheckLonghornReady())
		})
	}
}

func TestOptionReadFailure(t *testing.T) {
	client, target := credentialFixture(t)
	client.PrependReactor("get", "settings", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.(clienttesting.GetAction).GetName() == settings.HTTPProxySettingName {
			return true, nil, fmt.Errorf("setting lookup failed")
		}
		return false, nil, nil
	})
	_, err := credentialReader(client).ThirdPartyCredentials(target, backuputil.ResticPasswordKey)
	require.ErrorContains(t, err, "setting lookup failed")
}
