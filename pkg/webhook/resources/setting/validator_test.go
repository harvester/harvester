package setting

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_validateOvercommitConfig(t *testing.T) {
	tests := []struct {
		name   string
		args   *v1beta1.Setting
		errMsg string
	}{
		{
			name: "invalid json default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: "overcommit-config"},
				Default:    `{"cpu":100,"memory":100,"storage":100`,
			},
			errMsg: `Invalid JSON: {"cpu":100,"memory":100,"storage":100`,
		},
		{
			name: "cpu undercommmit default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: "overcommit-config"},
				Default:    `{"cpu":99,"memory":100,"storage":100}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 99`,
		},
		{
			name: "memory undercommmit default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: "overcommit-config"},
				Default:    `{"cpu":100,"memory":98,"storage":100}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 98`,
		},
		{
			name: "storage undercommmit default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: "overcommit-config"},
				Default:    `{"cpu":100,"memory":100,"storage":97}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 97`,
		},
		{
			name: "invalid json",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":100,"memory":100,"storage":100`,
			},
			errMsg: `Invalid JSON: {"cpu":100,"memory":100,"storage":100`,
		},
		{
			name: "cpu undercommmit",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":99,"memory":100,"storage":100}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 99`,
		},
		{
			name: "memory undercommmit",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":100,"memory":98,"storage":100}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 98`,
		},
		{
			name: "storage undercommmit",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":100,"memory":100,"storage":97}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 97`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOvercommitConfig(tt.args)
			if tt.errMsg != "" {
				assert.Equal(t, tt.errMsg, err.Error())
			}
		})

	}
}

func Test_validateSupportBundleTimeout(t *testing.T) {
	tests := []struct {
		name        string
		args        *v1beta1.Setting
		expectedErr bool
	}{
		{
			name: "invalid int default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Default:    "not int",
			},
			expectedErr: true,
		},
		{
			name: "negative int default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Default:    "-1",
			},
			expectedErr: true,
		},
		{
			name: "invalid int value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Value:      "not int",
			},
			expectedErr: true,
		},
		{
			name: "negative int value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Value:      "-1",
			},
			expectedErr: true,
		},
		{
			name: "input 0",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Default:    "0",
				Value:      "0",
			},
			expectedErr: false,
		},
		{
			name: "empty input",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Value:      "",
			},
			expectedErr: false,
		},
		{
			name: "positive int",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Default:    "1",
				Value:      "1",
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSupportBundleTimeout(tt.args)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func Test_validateSupportBundleExpiration(t *testing.T) {
	tests := []struct {
		name        string
		args        *v1beta1.Setting
		expectedErr bool
	}{
		{
			name: "invalid int default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleExpirationSettingName},
				Default:    "not int",
			},
			expectedErr: true,
		},
		{
			name: "negative int default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleExpirationSettingName},
				Default:    "-1",
			},
			expectedErr: true,
		},
		{
			name: "invalid int",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleExpirationSettingName},
				Value:      "not int",
			},
			expectedErr: true,
		},
		{
			name: "negative int",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleExpirationSettingName},
				Value:      "-1",
			},
			expectedErr: true,
		},
		{
			name: "empty input",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleExpirationSettingName},
				Default:    "",
				Value:      "",
			},
			expectedErr: false,
		},
		{
			name: "positive int",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleExpirationSettingName},
				Default:    "10",
				Value:      "10",
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSupportBundleExpiration(tt.args)
			assert.Equal(t, tt.expectedErr, err != nil)
		})
	}
}

func Test_validateSupportBundleNodeCollectionTimeout(t *testing.T) {
	tests := []struct {
		name        string
		args        *v1beta1.Setting
		expectedErr bool
	}{
		{
			name: "invalid int default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Default:    "not int",
			},
			expectedErr: true,
		},
		{
			name: "negative int default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Default:    "-1",
			},
			expectedErr: true,
		},
		{
			name: "invalid int value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Value:      "not int",
			},
			expectedErr: true,
		},
		{
			name: "negative int value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Value:      "-1",
			},
			expectedErr: true,
		},
		{
			name: "empty input",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Default:    "",
				Value:      "",
			},
			expectedErr: false,
		},
		{
			name: "positive int",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Default:    "10",
				Value:      "10",
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSupportBundleNodeCollectionTimeout(tt.args)
			assert.Equal(t, tt.expectedErr, err != nil)
		})
	}
}

func Test_validateSSLProtocols(t *testing.T) {
	tests := []struct {
		name        string
		args        *settings.SSLParameter
		expectedErr bool
	}{
		{
			name:        "Supported protocol 'TLSv1.2'",
			args:        &settings.SSLParameter{Protocols: "TLSv1.2"},
			expectedErr: false,
		},
		{
			name:        "Unsupported protocol 'MyTLSv99.9'",
			args:        &settings.SSLParameter{Protocols: "MyTLSv99.9"},
			expectedErr: true,
		},
		{
			name:        "A list of supported protocols separated by whitespace",
			args:        &settings.SSLParameter{Protocols: "TLSv1.1 TLSv1.2"},
			expectedErr: false,
		},
		{
			name:        "A list of supported protocols separated by multiple whitespace",
			args:        &settings.SSLParameter{Protocols: "  TLSv1.1    TLSv1.2  "},
			expectedErr: false,
		},
		{
			name:        "One unsupported protocol in a list",
			args:        &settings.SSLParameter{Protocols: "TLSv1.2 TLSv1.1 MyTLSv99.9"},
			expectedErr: true,
		},
		{
			name:        "Protocols separate by characters other than whitespace is invalid",
			args:        &settings.SSLParameter{Protocols: "TLSv1.1,TLSv1.2,TLSv1.3"},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSSLProtocols(tt.args)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func Test_validateNoProxy_1(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node0",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "192.168.0.30",
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "192.168.0.31",
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "192.168.0.32",
					},
				},
			},
		},
	}

	tests := []struct {
		name           string
		noProxy        string
		expectedErr    bool
		expectedErrMsg string
	}{
		{
			name:           "Empty noProxy",
			noProxy:        "",
			expectedErr:    true,
			expectedErrMsg: "noProxy should contain the node's IP addresses or CIDR. The node(s) 192.168.0.30, 192.168.0.31, 192.168.0.32 are not covered.",
		},
		{
			name:        "noProxy=192.168.0.0/24",
			noProxy:     "192.168.0.0/24",
			expectedErr: false,
		},
		{
			name:           "noProxy=10.1.2.0/24",
			noProxy:        "10.1.2.0/24,foo.bar",
			expectedErr:    true,
			expectedErrMsg: "noProxy should contain the node's IP addresses or CIDR. The node(s) 192.168.0.30, 192.168.0.31, 192.168.0.32 are not covered.",
		},
		{
			name:           "noProxy=192.168.0.0/27",
			noProxy:        "192.168.0.0/27",
			expectedErr:    true,
			expectedErrMsg: "noProxy should contain the node's IP addresses or CIDR. The node(s) 192.168.0.32 are not covered.",
		},
		{
			name:           "noProxy=192.168.0.30,192.168.0.31,",
			noProxy:        "192.168.0.30,192.168.0.31,",
			expectedErr:    true,
			expectedErrMsg: "noProxy should contain the node's IP addresses or CIDR. The node(s) 192.168.0.32 are not covered.",
		},
		{
			name:        "noProxy=192.168.0.30, 192.168.0.31 , 192.168.0.32",
			noProxy:     "192.168.0.30, 192.168.0.31 , 192.168.0.32",
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNoProxy(tt.noProxy, nodes)
			if tt.expectedErr {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErrMsg, err.Error())
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func Test_validateNoProxy_2(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node0",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "fda2:a25d:2a87:5bf7::30",
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "fda2:a25d:2a87:5bf7::31",
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "fda2:a25d:2a87:5bf7::32",
					},
				},
			},
		},
	}

	tests := []struct {
		name           string
		noProxy        string
		expectedErr    bool
		expectedErrMsg string
	}{
		{
			name:           "Empty noProxy",
			noProxy:        "",
			expectedErr:    true,
			expectedErrMsg: "noProxy should contain the node's IP addresses or CIDR. The node(s) fda2:a25d:2a87:5bf7::30, fda2:a25d:2a87:5bf7::31, fda2:a25d:2a87:5bf7::32 are not covered.",
		},
		{
			name:        "noProxy=fda2:a25d:2a87:5bf7::/48",
			noProxy:     "fda2:a25d:2a87:5bf7::/48",
			expectedErr: false,
		},
		{
			name:           "noProxy=fda2:a25d:2a87:5bf7::/123",
			noProxy:        "fda2:a25d:2a87:5bf7::/123",
			expectedErr:    true,
			expectedErrMsg: "noProxy should contain the node's IP addresses or CIDR. The node(s) fda2:a25d:2a87:5bf7::30, fda2:a25d:2a87:5bf7::31, fda2:a25d:2a87:5bf7::32 are not covered.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNoProxy(tt.noProxy, nodes)
			if tt.expectedErr {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErrMsg, err.Error())
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func Test_validateHTTPProxyHelper(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node0",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "192.168.0.30",
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "192.168.0.31",
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "192.168.0.32",
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		value       string
		expectedErr bool
	}{
		{
			name:        "empty string",
			value:       "",
			expectedErr: false,
		},
		{
			name:        "empty JSON object",
			value:       "{}",
			expectedErr: false,
		},
		{
			name:        "empty httpProxy/httpsProxy/noProxy",
			value:       `{"httpProxy": "", "httpsProxy": "", "noProxy": ""}`,
			expectedErr: false,
		},
		{
			name:        "empty httpProxy/httpsProxy",
			value:       `{"httpProxy": "", "httpsProxy": "", "noProxy": "xyz"}`,
			expectedErr: false,
		},
		{
			name:        "not empty httpProxy/noProxy - failure",
			value:       `{"httpProxy": "foo", "httpsProxy": "", "noProxy": "xyz"}`,
			expectedErr: true,
		},
		{
			name:        "not empty httpsProxy/noProxy - failure",
			value:       `{"httpProxy": "", "httpsProxy": "bar", "noProxy": "xyz"}`,
			expectedErr: true,
		},
		{
			name:        "not empty httpProxy/httpsProxy/noProxy - failure",
			value:       `{"httpProxy": "foo", "httpsProxy": "bar", "noProxy": "xyz"}`,
			expectedErr: true,
		},
		{
			name:        "not empty httpProxy/noProxy - success",
			value:       `{"httpProxy": "foo", "httpsProxy": "", "noProxy": "192.168.0.0/24"}`,
			expectedErr: false,
		},
		{
			name:        "not empty httpsProxy/noProxy - success",
			value:       `{"httpProxy": "", "httpsProxy": "bar", "noProxy": "192.168.0.0/24"}`,
			expectedErr: false,
		},
		{
			name:        "not empty httpProxy/httpsProxy/noProxy - success",
			value:       `{"httpProxy": "foo", "httpsProxy": "bar", "noProxy": "192.168.0.0/24"}`,
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHTTPProxyHelper(tt.value, nodes)
			assert.Equal(t, tt.expectedErr, err != nil)
		})
	}
}

func Test_validateKubeconfigTTLSetting(t *testing.T) {
	tests := []struct {
		name        string
		args        *v1beta1.Setting
		expectedErr bool
	}{
		{
			name: "invalid int default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.KubeconfigDefaultTokenTTLMinutesSettingName},
				Default:    "not int",
			},
			expectedErr: true,
		},
		{
			name: "negative int default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.KubeconfigDefaultTokenTTLMinutesSettingName},
				Default:    "-1",
			},
			expectedErr: true,
		},
		{
			name: "invalid int value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.KubeconfigDefaultTokenTTLMinutesSettingName},
				Value:      "not int",
			},
			expectedErr: true,
		},
		{
			name: "negative int value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.KubeconfigDefaultTokenTTLMinutesSettingName},
				Value:      "-1",
			},
			expectedErr: true,
		},
		{
			name: "empty input",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.KubeconfigDefaultTokenTTLMinutesSettingName},
				Default:    "",
				Value:      "",
			},
			expectedErr: false,
		},
		{
			name: "positive int",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.KubeconfigDefaultTokenTTLMinutesSettingName},
				Default:    "10",
				Value:      "10",
			},
			expectedErr: false,
		},
		{
			name: "exceeds 100 years",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.KubeconfigDefaultTokenTTLMinutesSettingName},
				Default:    "10",
				Value:      "52560001",
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKubeConfigTTLSetting(tt.args)
			assert.Equal(t, tt.expectedErr, err != nil)
		})
	}
}

func Test_validateNTPServers(t *testing.T) {
	testCases := []struct {
		name        string
		args        *v1beta1.Setting
		expectedErr string
	}{
		{
			name: "valid empty ntp servers",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Value:      "",
			},
			expectedErr: "",
		},
		{
			name: "valid ntp servers - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Default:    `{"ntpServers":["0.suse.pool.ntp.org", "1.suse.pool.ntp.org"]}`,
			},
			expectedErr: "",
		},
		{
			name: "valid ntp servers - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Value:      `{"ntpServers":["0.suse.pool.ntp.org", "1.suse.pool.ntp.org"]}`,
			},
			expectedErr: "",
		},
		{
			name: "invalid ntp servers json string - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Default:    `foobar`,
			},
			expectedErr: "failed to parse NTP settings: invalid character 'o' in literal false (expecting 'a')",
		},
		{
			name: "invalid ntp servers json string - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Value:      `foobar`,
			},
			expectedErr: "failed to parse NTP settings: invalid character 'o' in literal false (expecting 'a')",
		},
		{
			name: "invalid ntp servers start with http - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Default:    `{"ntpServers":["http://1.suse.pool.ntp.org"]}`,
			},
			expectedErr: "ntp server http://1.suse.pool.ntp.org should not start with http:// or https://",
		},
		{
			name: "invalid ntp servers start with http - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Value:      `{"ntpServers":["http://1.suse.pool.ntp.org"]}`,
			},
			expectedErr: "ntp server http://1.suse.pool.ntp.org should not start with http:// or https://",
		},
		{
			name: "invalid ntp servers start with https - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Default:    `{"ntpServers":["https://1.suse.pool.ntp.org"]}`,
			},
			expectedErr: "ntp server https://1.suse.pool.ntp.org should not start with http:// or https://",
		},
		{
			name: "invalid ntp servers start with https - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Value:      `{"ntpServers":["https://1.suse.pool.ntp.org"]}`,
			},
			expectedErr: "ntp server https://1.suse.pool.ntp.org should not start with http:// or https://",
		},
		{
			name: "invalid ntp servers not match FQDN - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Default:    `{"ntpServers":["$.suse.pool.ntp.org"]}`,
			},
			expectedErr: "invalid NTP server: $.suse.pool.ntp.org",
		},
		{
			name: "invalid ntp servers not match FQDN - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Value:      `{"ntpServers":["$.suse.pool.ntp.org"]}`,
			},
			expectedErr: "invalid NTP server: $.suse.pool.ntp.org",
		},
		{
			name: "duplicate ntp servers - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Default:    `{"ntpServers":["0.suse.pool.ntp.org", "0.suse.pool.ntp.org", "1.suse.pool.ntp.org"]}`,
			},
			expectedErr: "duplicate NTP server: [0.suse.pool.ntp.org]",
		},
		{
			name: "duplicate ntp servers - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.NTPServersSettingName},
				Value:      `{"ntpServers":["0.suse.pool.ntp.org", "0.suse.pool.ntp.org", "1.suse.pool.ntp.org"]}`,
			},
			expectedErr: "duplicate NTP server: [0.suse.pool.ntp.org]",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateNTPServers(testCase.args)
			if len(testCase.expectedErr) > 0 {
				assert.Equal(t, testCase.expectedErr, err.Error())
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func Test_validateUpgradeConfig(t *testing.T) {
	tests := []struct {
		name        string
		args        *v1beta1.Setting
		expectedErr bool
	}{
		{
			name: "empty config - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Default:    "{}",
			},
			expectedErr: true,
		},
		{
			name: "empty config - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      "{}",
			},
			expectedErr: true,
		},
		{
			name: "invalid string - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Default:    "random string",
			},
			expectedErr: true,
		},
		{
			name: "invalid string - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      "random string",
			},
			expectedErr: true,
		},
		{
			name: "skip image preload - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Default:    `{"imagePreloadOption":{"strategy":{"type":"skip"}}}`,
			},
			expectedErr: false,
		},
		{
			name: "skip image preload - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"skip"}}}`,
			},
			expectedErr: false,
		},
		{
			name: "do image preload node by node - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Default:    `{"imagePreloadOption":{"strategy":{"type":"sequential"}}}`,
			},
			expectedErr: false,
		},
		{
			name: "do image preload node by node - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"sequential"}}}`,
			},
			expectedErr: false,
		},
		{
			name: "do image preload in parallel - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Default:    `{"imagePreloadOption":{"strategy":{"type":"parallel"}}}`,
			},
			expectedErr: false,
		},
		{
			name: "do image preload in parallel - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"parallel"}}}`,
			},
			expectedErr: false,
		},
		{
			name: "do image preload in parallel with negative value for concurrency - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Default:    `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":-1}}}`,
			},
			expectedErr: true,
		},
		{
			name: "do image preload in parallel with negative value for concurrency - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":-1}}}`,
			},
			expectedErr: true,
		},
		{
			name: "do image preload in parallel (all nodes) - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Default:    `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":0}}}`,
			},
			expectedErr: false,
		},
		{
			name: "do image preload in parallel (all nodes) - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":0}}}`,
			},
			expectedErr: false,
		},
		{
			name: "do image preload in parallel with concurrency set to 1 - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Default:    `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":1}}}`,
			},
			expectedErr: false,
		},
		{
			name: "do image preload in parallel with concurrency set to 1 - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":1}}}`,
			},
			expectedErr: false,
		},
		{
			name: "do image preload in parallel with concurrency set to 2 - default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Default:    `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":2}}}`,
			},
			expectedErr: false,
		},
		{
			name: "do image preload in parallel with concurrency set to 2 - value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":2}}}`,
			},
			expectedErr: false,
		},
		{
			name: "enable restoreVM",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":2}}, "restoreVM": true}`,
			},
			expectedErr: false,
		},
		{
			name: "disable restoreVM",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":2}}, "restoreVM": false}`,
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUpgradeConfigFields(tt.args)
			assert.Equal(t, tt.expectedErr, err != nil)
		})
	}
}

func Test_validateAdditionalGuestMemoryOverheadRatio(t *testing.T) {
	tests := []struct {
		name        string
		args        *v1beta1.Setting
		expectedErr bool
	}{
		{
			name: "valid default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.AdditionalGuestMemoryOverheadRatioName},
				Default:    settings.AdditionalGuestMemoryOverheadRatioDefault,
			},
			expectedErr: false,
		},
		{
			name: "valid value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.AdditionalGuestMemoryOverheadRatioName},
				Default:    "2.8",
			},
			expectedErr: false,
		},
		{
			name: "invalid float",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Default:    "invalid float",
			},
			expectedErr: true,
		},
		{
			name: "invalid negative value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Value:      "-1.0",
			},
			expectedErr: true,
		},
		{
			name: "invalid less than limitation",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Value:      fmt.Sprintf("%v", settings.AdditionalGuestMemoryOverheadRatioMinValue-0.1),
			},
			expectedErr: true,
		},
		{
			name: "invalid greater than limitation",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Value:      fmt.Sprintf("%v", settings.AdditionalGuestMemoryOverheadRatioMaxValue+0.1),
			},
			expectedErr: true,
		},
		{
			name: "empty input but is valid",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleNodeCollectionTimeoutName},
				Default:    "",
				Value:      "",
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAdditionalGuestMemoryOverheadRatio(tt.args)
			assert.Equal(t, tt.expectedErr, err != nil)
		})
	}
}

func Test_validateStorageNetworkConfig(t *testing.T) {
	tests := []struct {
		name   string
		args   *v1beta1.Setting
		errMsg string
	}{
		{
			name: "ok to create storge-network with none values",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
			},
			errMsg: "",
		},
		{
			name: "ok to create storge-network with empty default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
				Default:    "",
			},
			errMsg: "",
		},
		{
			name: "ok to create storge-network with empty default and value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
				Default:    "",
				Value:      "",
			},
			errMsg: "",
		},
		{
			name: "fail to create storge-network with invalid json",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
				Default:    "",
				Value:      `{"invalid"}`,
			},
			errMsg: "failed to unmarshal the setting value",
		},
		{
			name: "fail to create storge-network with invalid vlan id 4095",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
				Default:    "",
				Value:      `{"vlan":4095}`,
			},
			errMsg: "the valid value range for VLAN IDs",
		},
		{
			name: "fail to create storge-network with invalid vlan id 65536",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
				Default:    "",
				Value:      `{"vlan":65536}`, // invalid uint16
			},
			errMsg: "failed to unmarshal the setting value",
		},
		{
			name: "fail to create storge-network with invalid vlan id -1",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
				Default:    "",
				Value:      `{"vlan":-1}`, // invalid uint16
			},
			errMsg: "failed to unmarshal the setting value",
		},
		{
			name: "fail to create storge-network with mgmt clusternetwork",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
				Default:    "",
				Value:      `{"vlan":1, "clusterNetwork":"mgmt"}`,
			},
			errMsg: "not allowed on",
		},
		// more tests are depending on a bunch of fake objects
	}

	clientset := fake.NewSimpleClientset()
	v := NewValidator(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Create(nil, tt.args)
			if tt.errMsg != "" {
				assert.True(t, strings.Contains(err.Error(), tt.errMsg))
			}
		})

	}
}

func Test_validateMaxHotplugRatio(t *testing.T) {
	tests := []struct {
		name   string
		args   *v1beta1.Setting
		errMsg string
	}{
		{
			name: "ok to create max-hotplug-ratio with none values",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.MaxHotplugRatioSettingName},
			},
			errMsg: "",
		},
		{
			name: "ok to create max-hotplug-ratio with empty default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.MaxHotplugRatioSettingName},
				Default:    "",
			},
			errMsg: "",
		},
		{
			name: "ok to create max-hotplug-ratio with empty default and value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.MaxHotplugRatioSettingName},
				Default:    "",
				Value:      "",
			},
			errMsg: "",
		},
		{
			name: "fail to create max-hotplug-ratio with invalid value -1",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.MaxHotplugRatioSettingName},
				Default:    "",
				Value:      "-1",
			},
			errMsg: "failed to parse",
		},
		{
			name: "fail to create max-hotplug-ratio with invalid value 3.5",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.MaxHotplugRatioSettingName},
				Default:    "3.5",
				Value:      "",
			},
			errMsg: "failed to parse",
		},
		{
			name: "fail to create max-hotplug-ratio with invalid value 21",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.MaxHotplugRatioSettingName},
				Default:    "21",
				Value:      "",
			},
			errMsg: "must be in range",
		},
		{
			name: "ok to create max-hotplug-ratio with valid value",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.MaxHotplugRatioSettingName},
				Default:    "5",
				Value:      "2",
			},
			errMsg: "",
		},
	}

	v := NewValidator(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Create(nil, tt.args)
			if tt.errMsg != "" {
				assert.True(t, strings.Contains(err.Error(), tt.errMsg))
			}
		})

	}
}
