package console

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harvester/harvester/pkg/installer/config"
)

// TODO(weihanglo): do not re-implement logic in test.
type FakeValidator struct {
	hasDevices []string
}

func (v FakeValidator) Validate(cfg *config.HarvesterConfig) error {
	if err := v.checkMgmtInterface(cfg.Install.ManagementInterface); err != nil {
		return err
	}
	if err := v.checkDevice(cfg.Install.Device); err != nil {
		return err
	}
	return nil
}

func (v FakeValidator) checkMgmtInterface(network config.Network) error {
	if len(network.Interfaces) > 0 {
		return nil
	}
	return prettyError(ErrMsgMgmtInterfaceNotSpecified, config.MgmtInterfaceName)
}

func (v FakeValidator) checkDevice(device string) error {
	for _, d := range v.hasDevices {
		if d == device {
			return nil
		}
	}
	return prettyError(ErrMsgDeviceNotFound, device)
}

func createDefaultFakeValidator() FakeValidator {
	return FakeValidator{
		hasDevices: []string{"/dev/vda"},
	}
}

func TestValidateConfig(t *testing.T) {
	createCreateConfig := func() *config.HarvesterConfig {
		return &config.HarvesterConfig{
			Token: "token",
			OS: config.OS{
				SSHAuthorizedKeys: []string{"github: someuser"},
				Password:          "password",
			},
			Install: config.Install{
				Mode: config.ModeCreate,
				ManagementInterface: config.Network{
					Interfaces: []config.NetworkInterface{
						{Name: "eth0"},
					},
				},
				Device: "/dev/vda",
			},
		}
	}

	createJoinConfig := func() *config.HarvesterConfig {
		c := createCreateConfig()
		c.ServerURL = "https://somewhere"
		c.Mode = config.ModeJoin
		return c
	}

	testCases := []struct {
		name     string
		cfg      *config.HarvesterConfig
		preApply func(c *config.HarvesterConfig)
		errMsg   string
	}{
		{
			name: "valid create config",
			cfg:  createCreateConfig(),
		},
		{
			name: "invalid create config: contains server URL",
			cfg:  createCreateConfig(),
			preApply: func(c *config.HarvesterConfig) {
				c.ServerURL = "https://somewhere"
			},
			errMsg: ErrMsgModeCreateContainsServerURL,
		},
		{
			name: "invalid config: unknown mode",
			cfg:  createCreateConfig(),
			preApply: func(c *config.HarvesterConfig) {
				c.Mode = "asdf"
			},
			errMsg: ErrMsgModeUnknown,
		},
		{
			name: "valid join config",
			cfg:  createJoinConfig(),
		},
		{
			name: "invalid join config: no server URL",
			cfg:  createJoinConfig(),
			preApply: func(c *config.HarvesterConfig) {
				c.ServerURL = ""
			},
			errMsg: ErrMsgModeJoinServerURLNotSpecified,
		},
		{
			name: "invalid create config: contains no credential",
			cfg:  createCreateConfig(),
			preApply: func(c *config.HarvesterConfig) {
				c.SSHAuthorizedKeys = nil
				c.Password = ""
			},
			errMsg: ErrMsgNoCredentials,
		},
		{
			name: "invalid create config: device not found",
			cfg:  createCreateConfig(),
			preApply: func(c *config.HarvesterConfig) {
				c.Device = "/dev/vdb"
			},
			errMsg: ErrMsgDeviceNotFound,
		},
		{
			name: "invalid create config: interface not found",
			cfg:  createCreateConfig(),
			preApply: func(c *config.HarvesterConfig) {
				c.Install.ManagementInterface.Interfaces = nil
			},
			errMsg: ErrMsgMgmtInterfaceNotSpecified,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.preApply != nil {
				testCase.preApply(testCase.cfg)
			}
			err := validateConfig(createDefaultFakeValidator(), testCase.cfg)
			if testCase.errMsg == "" {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			}
		})
	}
}

func TestContainerdRegistrySettingValidation(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErrText string
	}{
		{
			name:        "empty object",
			input:       "{}",
			wantErrText: "",
		},
		{
			name:        "invalid JSON",
			input:       `{"error"}`,
			wantErrText: ErrContainerdRegistrySettingNotValidJSON,
		},
		{
			name:        "invalid config type",
			input:       `{"Configs": 1}`,
			wantErrText: ErrContainerdRegistrySettingNotValidJSON,
		},
		{
			name:        "invalid mirrors type",
			input:       `{"Mirrors": 1}`,
			wantErrText: ErrContainerdRegistrySettingNotValidJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkSystemSettings(map[string]string{
				"containerd-registry": tt.input,
			})
			if got == nil {
				assert.Equal(t, tt.wantErrText, "")
			} else {
				assert.Equal(t, tt.wantErrText, got.Error())
			}
		})
	}
}

func TestCheckToken(t *testing.T) {
	testCases := []struct {
		name       string
		tokenValue string
		expectErr  bool
	}{
		{
			name:       "Some regular string",
			tokenValue: "AlphanumericValue12345",
			expectErr:  false,
		},
		{
			name:       "Some special characters",
			tokenValue: "Hello, @Harvester, you're \"awesome\"! [md](url)",
			expectErr:  false,
		},
		{
			name:       "Non-ASCII characters are invalid",
			tokenValue: "Äöé",
			expectErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkToken(tc.tokenValue)
			if tc.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestCheckPersistentStatePath(t *testing.T) {
	testCases := []struct {
		name                string
		persistentStatePath string
		expectErr           bool
	}{
		{
			name:                "empty path",
			persistentStatePath: "",
			expectErr:           true,
		},
		{
			name:                "not a abs path",
			persistentStatePath: "var/lib/test",
			expectErr:           true,
		},
		{
			name:                "valid path",
			persistentStatePath: "/var/lib/test",
			expectErr:           false,
		},
		{
			name:                "tmp path1",
			persistentStatePath: "/tmp",
			expectErr:           true,
		},
		{
			name:                "tmp path2",
			persistentStatePath: "/tmp/",
			expectErr:           true,
		},
		{
			name:                "tmp path3",
			persistentStatePath: "/tmp/test",
			expectErr:           true,
		},
		{
			name:                "not tmp path1",
			persistentStatePath: "/tmptest",
			expectErr:           false,
		},
		{
			name:                "not tmp path2",
			persistentStatePath: "/tmptest/",
			expectErr:           false,
		},
		{
			name:                "not tmp path3",
			persistentStatePath: "/tmptest/tmp",
			expectErr:           false,
		},
		{
			name:                "not tmp path4",
			persistentStatePath: "/tmptest/tmp/",
			expectErr:           false,
		},
		{
			name:                "not tmp path5",
			persistentStatePath: "/tmptest/tmp/foo",
			expectErr:           false,
		},
		{
			name:                "not tmp path6",
			persistentStatePath: "/tmptest/tmp/foo/",
			expectErr:           false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkPersistentStatePath(tc.persistentStatePath)
			if tc.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
