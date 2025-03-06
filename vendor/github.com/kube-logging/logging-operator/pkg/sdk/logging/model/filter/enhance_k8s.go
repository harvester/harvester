// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package filter

import (
	"github.com/cisco-open/operator-tools/pkg/secret"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Enhance K8s Metadata"
// +weight:"200"
type _hugoEnhanceK8s interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Enhance K8s Metadata](https://github.com/SumoLogic/sumologic-kubernetes-fluentd/tree/main/fluent-plugin-enhance-k8s-metadata)"
// Fluentd Filter plugin to fetch several metadata for a Pod
type _docEnhanceK8s interface{} //nolint:deadcode,unused

// +name:"Enhance K8s Metadata"
// +url:"https://github.com/SumoLogic/sumologic-kubernetes-fluentd/tree/main/fluent-plugin-enhance-k8s-metadata"
// +version:"2.0.0"
// +description:"Fluentd output plugin to add extra Kubernetes metadata to the events."
// +status:"GA"
type _metaEnhanceK8s interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type EnhanceK8s struct {
	// parameters for read/write record (default: `['$.namespace']`)
	InNamespacePath []string `json:"in_namespace_path,omitempty"`
	// (default: `['$.pod','$.pod_name']`)
	InPodPath []string `json:"in_pod_path,omitempty"`
	// Sumologic data type (default: metrics)
	DataType string `json:"data_type,omitempty"`
	// Kubernetes API URL (default: nil)
	KubernetesUrl string `json:"kubernetes_url,omitempty"`
	// Kubernetes API Client certificate (default: nil)
	ClientCert secret.Secret `json:"client_cert,omitempty"`
	// // Kubernetes API Client certificate key (default: nil)
	ClientKey secret.Secret `json:"client_key,omitempty"`
	// Kubernetes API CA file (default: nil)
	CaFile secret.Secret `json:"ca_file,omitempty"`
	// Service account directory (default: /var/run/secrets/kubernetes.io/serviceaccount)
	SecretDir string `json:"secret_dir,omitempty"`
	// Bearer token path (default: nil)
	BearerTokenFile string `json:"bearer_token_file,omitempty"`
	// Verify SSL (default: true)
	VerifySSL *bool `json:"verify_ssl,omitempty"`
	// Kubernetes core API version (for different Kubernetes versions) (default: ['v1'])
	CoreAPIVersions []string `json:"core_api_versions,omitempty"`
	// Kubernetes resources api groups (default: `["apps/v1", "extensions/v1beta1"]`)
	APIGroups []string `json:"api_groups,omitempty"`
	// If `ca_file` is for an intermediate CA, or otherwise we do not have the
	// root CA and want to trust the intermediate CA certs we do have, set this
	// to `true` - this corresponds to the openssl s_client -partial_chain flag
	// and X509_V_FLAG_PARTIAL_CHAIN (default: false)
	SSLPartialChain *bool `json:"ssl_partial_chain,omitempty"`
	// Cache size  (default: 1000)
	CacheSize int `json:"cache_size,omitempty"`
	// Cache TTL (default: 60*60*2)
	CacheTTL int `json:"cache_ttl,omitempty"`
	// Cache refresh (default: 60*60)
	CacheRefresh int `json:"cache_refresh,omitempty"`
	// Cache refresh variation (default: 60*15)
	CacheRefreshVariation int `json:"cache_refresh_variation,omitempty"`
}

//
/*
## Example `EnhanceK8s` filter configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Logging
metadata:
  name: demo-flow
spec:
  globalFilters:
    - enhanceK8s: {}
{{</ highlight >}}

Fluentd config result:

{{< highlight xml >}}
<filter **>
  @type enhance_k8s_metadata
  @id test_enhanceK8s
</filter>
{{</ highlight >}}
*/
type _expEnhanceK8s interface{} //nolint:deadcode,unused

func (c *EnhanceK8s) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "enhance_k8s_metadata"
	enhanceK8s := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "filter",
			Tag:       "**",
			Id:        id,
		},
	}
	enhanceK8sConfig := c.DeepCopy()

	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(enhanceK8sConfig); err != nil {
		return nil, err
	} else {
		enhanceK8s.Params = params
	}
	return enhanceK8s, nil
}
