package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

func Test_validateOvercommitConfig(t *testing.T) {
	tests := []struct {
		name   string
		args   *v1beta1.Setting
		errMsg string
	}{
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
			name: "invalid int",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Value:      "not int",
			},
			expectedErr: true,
		},
		{
			name: "negative int",
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
				Value:      "",
			},
			expectedErr: false,
		},
		{
			name: "positive int",
			args: &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SupportBundleExpirationSettingName},
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
