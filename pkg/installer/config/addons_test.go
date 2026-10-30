package config

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	addonrender "github.com/harvester/harvester/pkg/addons/render"
	"github.com/harvester/harvester/pkg/installer/util"
)

// TestBuiltInAddonsIPFamilyPolicy renders the assembled built-in addon
// templates the same way the build and the installer do: the "<< >>" version
// placeholders first, then the "{{ }}" runtime toggles against a
// HarvesterConfig, and checks what the dual-stack toggles produce.
func TestBuiltInAddonsIPFamilyPolicy(t *testing.T) {
	const addonsDir = "../../../addons"

	metas, err := addonrender.Discover(addonsDir)
	require.NoError(t, err)
	assembled, err := addonrender.AssembleTemplate(addonsDir, metas)
	require.NoError(t, err)

	// version values are irrelevant here, unresolved keys render as empty
	versionTmpl, err := template.New("").Delims("<<", ">>").Parse(assembled)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, versionTmpl.Execute(&buf, map[string]string{}))

	ipFamilyPolicyAddons := []string{
		"harvester-seeder",
		"kubeovn-operator",
		"pcidevices-controller",
		"vm-import-controller",
	}

	testCases := []struct {
		name            string
		ipFamilies      []string
		wantPolicy      string
		wantNetStack    string
		wantDualStackCR bool
	}{
		{
			name:         "IPv4-only renders SingleStack",
			ipFamilies:   []string{IPFamilyIPv4},
			wantPolicy:   "SingleStack",
			wantNetStack: "ipv4",
		},
		{
			name:         "unset IPFamilies renders SingleStack",
			ipFamilies:   nil,
			wantPolicy:   "SingleStack",
			wantNetStack: "ipv4",
		},
		{
			name:         "IPv6-only renders SingleStack",
			ipFamilies:   []string{IPFamilyIPv6},
			wantPolicy:   "SingleStack",
			wantNetStack: "ipv4",
		},
		{
			name:            "dual-stack renders PreferDualStack",
			ipFamilies:      []string{IPFamilyIPv4, IPFamilyIPv6},
			wantPolicy:      "PreferDualStack",
			wantNetStack:    "dual_stack",
			wantDualStackCR: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conf := &HarvesterConfig{}
			conf.Install.IPFamilies = tc.ipFamilies

			rendered, err := util.RenderTemplate(buf.String(), conf)
			require.NoError(t, err)

			var res struct {
				Resources []struct {
					Metadata struct {
						Name string `yaml:"name"`
					} `yaml:"metadata"`
					Spec struct {
						ValuesContent string `yaml:"valuesContent"`
					} `yaml:"spec"`
				} `yaml:"resources"`
			}
			require.NoError(t, yaml.Unmarshal([]byte(rendered), &res))

			values := map[string]map[string]any{}
			for _, r := range res.Resources {
				v := map[string]any{}
				require.NoError(t, yaml.Unmarshal([]byte(r.Spec.ValuesContent), &v), r.Metadata.Name)
				values[r.Metadata.Name] = v
			}

			for _, name := range ipFamilyPolicyAddons {
				v, ok := values[name]
				require.True(t, ok, "addon %s not found", name)
				global, ok := v["global"].(map[string]any)
				require.True(t, ok, "addon %s has no global values", name)
				assert.Equal(t, tc.wantPolicy, global["ipFamilyPolicy"], name)
			}

			spec, ok := values["kubeovn-operator"]["configurationSpec"].(map[string]any)
			require.True(t, ok)
			networking, ok := spec["networking"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tc.wantNetStack, networking["netStack"])
			_, hasDualStack := spec["dualStack"]
			assert.Equal(t, tc.wantDualStackCR, hasDualStack)
		})
	}
}
