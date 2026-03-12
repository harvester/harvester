package setting

import (
	"fmt"
	"strings"
	"testing"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/rancher/wrangler/v3/pkg/webhook"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/storagenetwork"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	networkutil "github.com/harvester/harvester/pkg/util/network"
	whTypes "github.com/harvester/harvester/pkg/webhook/types"
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

func Test_validateSupportBundleFileName(t *testing.T) {
	tests := []struct {
		name        string
		args        *v1beta1.Setting
		expectedErr bool
	}{
		{
			name: "empty default",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleFileNameSettingName},
				Default:    "",
			},
			expectedErr: false,
		},
		{
			name: "valid name with hyphens",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleFileNameSettingName},
				Value:      "my-cluster-01",
			},
			expectedErr: false,
		},
		{
			name: "invalid name starting with hyphen",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleFileNameSettingName},
				Value:      "-invalid",
			},
			expectedErr: true,
		},
		{
			name: "invalid name ending with hyphen",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleFileNameSettingName},
				Value:      "invalid-",
			},
			expectedErr: true,
		},
		{
			name: "invalid name with uppercase letters",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleFileNameSettingName},
				Value:      "Invalid",
			},
			expectedErr: true,
		},
		{
			name: "invalid name with underscore",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleFileNameSettingName},
				Value:      "test_name",
			},
			expectedErr: true,
		},
		{
			name: "invalid name with special characters",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleFileNameSettingName},
				Value:      "test@name",
			},
			expectedErr: true,
		},
		{
			name: "invalid name exceeding max length (64 chars)",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleFileNameSettingName},
				Value:      "a1234567890123456789012345678901234567890123456789012345678901234",
			},
			expectedErr: true,
		},
		{
			name: "invalid name with spaces",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleFileNameSettingName},
				Value:      "my cluster",
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSupportBundleFileName(tt.args)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.Nil(t, err)
			}
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
	givenNodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-0",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
			},
		},
	}
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
			name: "node upgrade with auto mode",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"sequential"}},"nodeUpgradeOption":{"strategy":{"mode":"auto"}}}`,
			},
			expectedErr: false,
		},
		{
			name: "node upgrade with manual mode",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"sequential"}},"nodeUpgradeOption":{"strategy":{"mode":"auto"}}}`,
			},
			expectedErr: false,
		},
		{
			name: "node upgrade with manual mode for node-1 only",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"sequential"}},"nodeUpgradeOption":{"strategy":{"mode":"auto","pauseNodes":["node-1"]}}}`,
			},
			expectedErr: false,
		},
		{
			name: "node upgrade with duplicated pause nodes specified should be rejected",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"sequential"}},"nodeUpgradeOption":{"strategy":{"mode":"auto","pauseNodes":["node-1","node-1","node-2"]}}}`,
			},
			expectedErr: true,
		},
		{
			name: "node upgrade with manual mode for a non-existing node should be rejected",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"sequential"}},"nodeUpgradeOption":{"strategy":{"mode":"auto","pauseNodes":["node-100"]}}}`,
			},
			expectedErr: true,
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
		{
			name: "LogeReadyTimeout in expected range",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":2}},"logReadyTimeout": "10"}`,
			},
			expectedErr: false,
		},
		{
			name: "LogReadyTimeout not in expected range",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.UpgradeConfigSettingName},
				Value:      `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":2}}, "logReadyTimeout": "21"}`,
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := make([]runtime.Object, 0, len(givenNodes))
			for _, node := range givenNodes {
				nodes = append(nodes, node)
			}

			clientset := fake.NewSimpleClientset(nodes...)
			v := &settingValidator{
				settingCache: fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
				nodeCache:    fakeclients.NodeCache(clientset.CoreV1().Nodes),
			}

			err := v.validateUpgradeConfig(tt.args)
			assert.Equal(t, tt.expectedErr, err != nil, err)
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

func Test_checkStorageNetworkNotBlockedByRWX(t *testing.T) {
	clearSetting := &v1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
		Value:      "",
	}

	tests := []struct {
		name        string
		rwxValue    string
		expectedErr bool
	}{
		{
			name:        "clear storage-network while share-storage-network=true -> blocked",
			rwxValue:    `{"share-storage-network":true,"network":{}}`,
			expectedErr: true,
		},
		{
			name:        "clear storage-network while share-storage-network=false -> allowed",
			rwxValue:    `{"share-storage-network":false,"network":{}}`,
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(&v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      tt.rwxValue,
			})
			v := &settingValidator{
				settingCache: fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
			}

			err := v.checkStorageNetworkNotBlockedByRWX(clearSetting)
			assert.Equal(t, tt.expectedErr, err != nil, err)
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

func Test_validateStorageNetwork_Update_InProgress(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	v := NewValidator(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings), nil, nil, nil, nil, fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines), nil, nil, nil, fakeclients.LonghornVolumeCache(clientset.LonghornV1beta2().Volumes), fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims), nil, nil, nil, fakeclients.LonghornNodeCache(clientset.LonghornV1beta2().Nodes), nil)

	t.Run("reject update when 'In Progress'", func(t *testing.T) {
		oldSetting := &v1beta1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
			Default:    settings.StorageNetwork.Default,
			Value:      `{"vlan":50,"clusterNetwork":"mgmt","range":"192.168.50.0/24","exclude":["192.168.50.1/32","192.168.50.2/32"]}`,
		}
		v1beta1.SettingConfigured.False(oldSetting)
		v1beta1.SettingConfigured.Reason(oldSetting, storagenetwork.ReasonInProgress)
		v1beta1.SettingConfigured.Message(oldSetting, "waiting for all volumes detached: pvc-12375075-487e-4add-9dfb-4e9c3881370d,pvc-736e8599-daf4-458e-8af2-3bbb038b0d46,pvc-d29243f2-e8e8-408a-98cc-e441dc83085d")

		newSetting := oldSetting.DeepCopy()
		newSetting.Value = `{"vlan":50,"clusterNetwork":"mgmt","range":"192.168.50.0/24","exclude":["192.168.50.1/32"]}`

		err := v.Update(nil, oldSetting, newSetting)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "cannot update the setting \"storage-network\" because it is still being configured")
	})

	t.Run("do not reject update when 'In Progress'", func(t *testing.T) {
		oldSetting := &v1beta1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
			Default:    settings.StorageNetwork.Default,
			Value:      `{"vlan":1, "clusterNetwork":"mgmt", "range":"10.0.0.0/24"}`,
		}
		v1beta1.SettingConfigured.False(oldSetting)
		v1beta1.SettingConfigured.Reason(oldSetting, storagenetwork.ReasonInProgress)
		v1beta1.SettingConfigured.Message(oldSetting, "waiting for all volumes detached: pvc-12375075-487e-4add-9dfb-4e9c3881370d,pvc-736e8599-daf4-458e-8af2-3bbb038b0d46,pvc-d29243f2-e8e8-408a-98cc-e441dc83085d")

		newSetting := oldSetting.DeepCopy()
		v1beta1.SettingConfigured.True(newSetting)
		v1beta1.SettingConfigured.Reason(newSetting, storagenetwork.ReasonCompleted)

		err := v.Update(nil, oldSetting, newSetting)
		assert.NoError(t, err)
	})

	t.Run("allow update when 'Completed'", func(t *testing.T) {
		oldSetting := &v1beta1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
			Default:    settings.StorageNetwork.Default,
			Value:      `{"vlan":1, "clusterNetwork":"mgmt", "range":"10.0.0.0/24"}`,
		}
		v1beta1.SettingConfigured.True(oldSetting)
		v1beta1.SettingConfigured.Reason(oldSetting, storagenetwork.ReasonCompleted)

		newSetting := oldSetting.DeepCopy()
		newSetting.Value = `{"vlan":2, "clusterNetwork":"mgmt", "range":"10.0.0.0/24"}`

		err := v.Update(nil, oldSetting, newSetting)
		assert.NoError(t, err)
	})

	t.Run("reject update default when 'In Progress'", func(t *testing.T) {
		oldSetting := &v1beta1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
			Default:    settings.StorageNetwork.Default,
			Value:      `{"vlan":50,"clusterNetwork":"mgmt","range":"192.168.50.0/24","exclude":["192.168.50.1/32","192.168.50.2/32"]}`,
		}
		v1beta1.SettingConfigured.False(oldSetting)
		v1beta1.SettingConfigured.Reason(oldSetting, storagenetwork.ReasonInProgress)
		v1beta1.SettingConfigured.Message(oldSetting, "waiting for all volumes detached: ...")

		newSetting := oldSetting.DeepCopy()
		newSetting.Default = `{"vlan":50,"clusterNetwork":"mgmt","range":"192.168.50.0/24","exclude":[]}`

		err := v.Update(nil, oldSetting, newSetting)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "cannot update the setting")
	})

	t.Run("allow update default when 'Completed'", func(t *testing.T) {
		oldSetting := &v1beta1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
			Default:    settings.StorageNetwork.Default,
		}
		v1beta1.SettingConfigured.True(oldSetting)
		v1beta1.SettingConfigured.Reason(oldSetting, storagenetwork.ReasonCompleted)

		newSetting := oldSetting.DeepCopy()
		newSetting.Default = `{"vlan":50,"clusterNetwork":"mgmt","range":"192.168.50.0/24","exclude":[]}`

		err := v.Update(nil, oldSetting, newSetting)
		assert.NoError(t, err)
	})

	t.Run("allow update to default when 'In Progress'", func(t *testing.T) {
		oldSetting := &v1beta1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
			Default:    settings.StorageNetwork.Default,
			Value:      `{"vlan":50,"clusterNetwork":"mgmt","range":"192.168.50.0/24","exclude":["192.168.50.1/32","192.168.50.2/32"]}`,
		}
		v1beta1.SettingConfigured.False(oldSetting)
		v1beta1.SettingConfigured.Reason(oldSetting, storagenetwork.ReasonInProgress)
		v1beta1.SettingConfigured.Message(oldSetting, "waiting for all volumes detached: ...")

		newSetting := oldSetting.DeepCopy()
		newSetting.Value = settings.StorageNetwork.Default

		err := v.Update(nil, oldSetting, newSetting)
		assert.NoError(t, err)
	})

	t.Run("allow update to default when 'Completed'", func(t *testing.T) {
		oldSetting := &v1beta1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
			Default:    settings.StorageNetwork.Default,
			Value:      `{"vlan":50,"clusterNetwork":"mgmt","range":"192.168.50.0/24","exclude":["192.168.50.1/32","192.168.50.2/32"]}`,
		}
		v1beta1.SettingConfigured.True(oldSetting)
		v1beta1.SettingConfigured.Reason(oldSetting, storagenetwork.ReasonCompleted)

		newSetting := oldSetting.DeepCopy()
		newSetting.Value = settings.StorageNetwork.Default

		err := v.Update(nil, oldSetting, newSetting)
		assert.NoError(t, err)
	})
}

func Test_validateUpdateRWXNetwork(t *testing.T) {
	// composite format helpers
	rwxDedicated := func(vlan int, clusterNetwork, ipRange string) string {
		return fmt.Sprintf(`{"share-storage-network":false,"network":{"vlan":%d,"clusterNetwork":%q,"range":%q}}`, vlan, clusterNetwork, ipRange)
	}
	rwxShare := `{"share-storage-network":true,"network":{}}`
	storageNetworkValue := `{"vlan":100,"clusterNetwork":"vlan","range":"192.168.0.0/24"}`

	attachedRWXVolume := &lhv1beta2.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rwx-vol-attached",
			Namespace: "longhorn-system",
		},
		Spec: lhv1beta2.VolumeSpec{
			AccessMode: lhv1beta2.AccessModeReadWriteMany,
		},
		Status: lhv1beta2.VolumeStatus{
			State: lhv1beta2.VolumeStateAttached,
		},
	}

	tests := []struct {
		name             string
		oldSetting       *v1beta1.Setting
		newSetting       *v1beta1.Setting
		existingSettings []*v1beta1.Setting
		existingVolumes  []*lhv1beta2.Volume
		expectedErr      bool
	}{
		{
			// Same effective value -> early return, no further checks
			name: "no change -> ok",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      "",
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      "",
			},
			expectedErr: false,
		},
		{
			name: "dedicated->share, storage-network set, volumes attached -> not ok",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxDedicated(101, "vlan", "192.168.1.0/24"),
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxShare,
			},
			existingSettings: []*v1beta1.Setting{
				{
					ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
					Value:      storageNetworkValue,
				},
			},
			existingVolumes: []*lhv1beta2.Volume{attachedRWXVolume},
			expectedErr:     true,
		},
		{
			// share=false->false, network changed, no attached volumes -> ok
			name: "dedicated network changed, volumes detached -> ok",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxDedicated(101, "mgmt", "192.168.1.0/24"),
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxDedicated(102, "mgmt", "192.168.2.0/24"),
			},
			expectedErr: false,
		},
		{
			// share=false->false, network changed, volumes attached -> not ok
			name: "dedicated network changed, volumes attached -> not ok",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxDedicated(101, "mgmt", "192.168.1.0/24"),
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxDedicated(102, "mgmt", "192.168.2.0/24"),
			},
			existingVolumes: []*lhv1beta2.Volume{attachedRWXVolume},
			expectedErr:     true,
		},
		{
			// share=false->true: storage-network not set -> not ok
			name: "dedicated->share, storage-network not set -> not ok",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxDedicated(101, "vlan", "192.168.1.0/24"),
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxShare,
			},
			existingSettings: []*v1beta1.Setting{
				{
					ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNetworkName},
					Value:      "",
				},
			},
			expectedErr: true,
		},
		{
			// share=true->false: no attached volumes -> ok
			name: "share->dedicated, volumes detached -> ok",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxShare,
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxDedicated(101, "mgmt", "192.168.1.0/24"),
			},
			expectedErr: false,
		},
		{
			// share=true->false: volumes attached -> not ok
			name: "share->dedicated, volumes attached -> not ok",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxShare,
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxDedicated(101, "mgmt", "192.168.1.0/24"),
			},
			existingVolumes: []*lhv1beta2.Volume{attachedRWXVolume},
			expectedErr:     true,
		},
		{
			// empty->valid dedicated JSON (mgmt cluster bypasses VlanStatus/VC checks), no volumes -> ok
			name: "empty->valid dedicated JSON, no volumes -> ok",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      "",
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      rwxDedicated(100, "mgmt", "192.168.0.0/24"),
			},
			expectedErr: false,
		},
		{
			// empty->invalid JSON: unmarshal fails -> error
			name: "empty->invalid JSON -> error",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      "",
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      `{invalid}`,
			},
			expectedErr: true,
		},
		{
			// dedicated with VLAN ID > 4094: checkNetworkVlanValid fails -> error
			name: "dedicated->invalid VLAN ID -> error",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      "",
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      `{"share-storage-network":false,"network":{"vlan":5000,"clusterNetwork":"mgmt","range":"192.168.0.0/24"}}`,
			},
			expectedErr: true,
		},
		{
			// dedicated with non-CIDR range: checkNetworkRangeValid fails -> error
			name: "dedicated->invalid range -> error",
			oldSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      "",
			},
			newSetting: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.RWXNetworkSettingName},
				Value:      `{"share-storage-network":false,"network":{"vlan":100,"clusterNetwork":"mgmt","range":"not-a-cidr"}}`,
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := make([]runtime.Object, 0, len(tt.existingSettings)+len(tt.existingVolumes))
			for _, s := range tt.existingSettings {
				objects = append(objects, s)
			}
			for _, vol := range tt.existingVolumes {
				objects = append(objects, vol)
			}

			clientset := fake.NewSimpleClientset(objects...)
			v := &settingValidator{
				settingCache:  fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
				lhVolumeCache: fakeclients.LonghornVolumeCache(clientset.LonghornV1beta2().Volumes),
				nodeCache:     fakeclients.NodeCache(clientset.CoreV1().Nodes),
			}

			req := &whTypes.Request{Request: &webhook.Request{}}
			err := v.validateUpdateRWXNetwork(req, tt.oldSetting, tt.newSetting)
			assert.Equal(t, tt.expectedErr, err != nil, err)
		})
	}
}

func Test_checkNetworkOverlap(t *testing.T) {
	tests := []struct {
		name    string
		c1Name  string
		c1      *networkutil.Config
		c2      map[string]*networkutil.Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "c1 is nil, skip check",
			c1Name:  "storage-network",
			c1:      nil,
			c2:      map[string]*networkutil.Config{"vm-migration-network": {Range: "192.168.1.0/24"}},
			wantErr: false,
		},
		{
			name:    "c2 contains only nil configs, no overlap",
			c1Name:  "storage-network",
			c1:      &networkutil.Config{Range: "192.168.1.0/24"},
			c2:      map[string]*networkutil.Config{"vm-migration-network": nil},
			wantErr: false,
		},
		{
			name:    "non-overlapping CIDRs, no error",
			c1Name:  "storage-network",
			c1:      &networkutil.Config{Range: "192.168.1.0/24"},
			c2:      map[string]*networkutil.Config{"vm-migration-network": {Range: "192.168.2.0/24"}},
			wantErr: false,
		},
		{
			name:    "overlapping CIDRs, return error",
			c1Name:  "storage-network",
			c1:      &networkutil.Config{Range: "192.168.1.0/24"},
			c2:      map[string]*networkutil.Config{"vm-migration-network": {Range: "192.168.1.0/24"}},
			wantErr: true,
			errMsg:  "storage-network: the network configuration is overlapped with vm-migration-network",
		},
		{
			name:   "c1 exclude removes overlap, no error",
			c1Name: "storage-network",
			c1: &networkutil.Config{
				Range:   "192.168.1.0/30",
				Exclude: []string{"192.168.1.1/32", "192.168.1.2/32"},
			},
			c2:      map[string]*networkutil.Config{"vm-migration-network": {Range: "192.168.1.1/32"}},
			wantErr: false,
		},
		{
			name:    "invalid CIDR in c1, return error",
			c1Name:  "storage-network",
			c1:      &networkutil.Config{Range: "not-a-cidr"},
			c2:      map[string]*networkutil.Config{"vm-migration-network": {Range: "192.168.1.0/24"}},
			wantErr: true,
		},
		{
			name:   "multiple c2 configs, one overlaps, return error",
			c1Name: "storage-network",
			c1:     &networkutil.Config{Range: "10.0.0.0/24"},
			c2: map[string]*networkutil.Config{
				"vm-migration-network": {Range: "192.168.1.0/24"},
				"rwx-network":          {Range: "10.0.0.0/24"},
			},
			wantErr: true,
			errMsg:  "storage-network: the network configuration is overlapped with rwx-network",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkNetworkOverlap(tt.c1Name, tt.c1, tt.c2)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.EqualError(t, err, tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
