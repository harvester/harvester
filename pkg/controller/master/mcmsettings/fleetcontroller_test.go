package mcmsettings

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/util"
)

const (
	fleetConfig = `    {
      "systemDefaultRegistry": "",
      "agentImage": "rancher/fleet-agent:v0.7.0",
      "agentImagePullPolicy": "IfNotPresent",
      "apiServerURL": "https://10.53.84.99",
      "apiServerCA": "ZmFrZUNBCg==",
      "agentCheckinInterval": "15m",
      "ignoreClusterRegistrationLabels": false,
      "bootstrap": {
        "paths": "",
        "repo": "",
        "secret": "",
        "branch":  "master",
        "namespace": "fleet-local",
        "agentNamespace": "cattle-fleet-local-system",
      },
      "webhookReceiverURL": "",
      "githubURLPrefix": ""
    }`
)

var (
	fleetControllerConfigMap = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fleet-controller",
			Namespace: "cattle-fleet-system",
		},
		Data: map[string]string{
			"config": fleetConfig,
		},
	}
)

func Test_patchFleetControllerConfigMap(t *testing.T) {
	var tests = []struct {
		key  string
		val  string
		name string
	}{
		{
			name: "patch apiServerURL",
			key:  util.APIServerURLKey,
			val:  "https://10.53.0.1",
		},
		{
			name: "patch apiServerCA",
			key:  util.APIServerCAKey,
			val:  "bmV3RmFrZUNBCg==",
		},
	}
	assert := require.New(t)

	for _, v := range tests {
		cm := fleetControllerConfigMap.DeepCopy()
		retCM, err := patchConfigMap(cm, v.key, v.val)
		assert.NoError(err, fmt.Sprintf("error patching value test case %s", v.name))
		retVal, err := getConfigMayKey(retCM, v.key)
		assert.NoError(err, fmt.Sprintf("error fetching value for test case %s", v.name))
		assert.Equal(retVal, v.val, fmt.Sprintf("expected new ret value to match for test case %s", v.name))
	}
}
