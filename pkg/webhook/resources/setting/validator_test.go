package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
				ObjectMeta: v1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":100,"memory":100,"storage":100`,
			},
			errMsg: `Invalid JSON: {"cpu":100,"memory":100,"storage":100`,
		},
		{
			name: "cpu undercommmit",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":99,"memory":100,"storage":100}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 99`,
		},
		{
			name: "memory undercommmit",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":100,"memory":98,"storage":100}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 98`,
		},
		{
			name: "storage undercommmit",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: "overcommit-config"},
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
				ObjectMeta: v1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Value:      "not int",
			},
			expectedErr: true,
		},
		{
			name: "negative int",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Value:      "-1",
			},
			expectedErr: true,
		},
		{
			name: "input 0",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Value:      "0",
			},
			expectedErr: false,
		},
		{
			name: "empty input",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
				Value:      "",
			},
			expectedErr: false,
		},
		{
			name: "positive int",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: settings.SupportBundleTimeoutSettingName},
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
