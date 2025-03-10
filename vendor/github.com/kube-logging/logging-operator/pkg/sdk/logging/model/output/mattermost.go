// Copyright © 2020 Banzai Cloud
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

package output

import (
	"github.com/cisco-open/operator-tools/pkg/secret"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Mattermost"
// +weight:"200"
type _hugoMattermost interface{} //nolint:deadcode,unused

// +docName:"Mattermost plugin for Fluentd"
/*
Sends logs to Mattermost via webhooks.
For details, see [https://github.com/levigo-systems/fluent-plugin-mattermost](https://github.com/levigo-systems/fluent-plugin-mattermost).

## Example output configurations

```yaml
spec:
  mattermost:
    webhook_url: https://xxx.xx/hooks/xxxxxxxxxxxxxxx
    channel_id: xxxxxxxxxxxxxxx
    message_color: "#FFA500"
    enable_tls: false
```
*/
type _docMattermost interface{} //nolint:deadcode,unused

// +name:"Mattermost"
// +url:"https://github.com/levigo-systems/fluent-plugin-mattermost"
// +version:"0.2.2"
// +description:"Sends logs to Mattermost via webhooks."
// +status:"GA"
type _metaMattermost interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type MattermostOutputConfig struct {
	// Incoming Webhook URI (Required for Incoming Webhook mode).
	WebhookURL *secret.Secret `json:"webhook_url"`
	// The ID of the channel where you want to receive the information.
	ChannelID string `json:"channel_id,omitempty"`
	// Color of the message you are sending, in hexadecimal format. (default: #A9A9A9)
	MessageColor string `json:"message_color,omitempty"`
	// The title you want to add to the message. (default: fluent_title_default)
	MessageTitle string `json:"message_title,omitempty"`
	// The message you want to send. It can be a static message, which you add at this point, or you can receive the Fluentd infos with the %s
	Message string `json:"message,omitempty"`
	// You can set the communication channel if it uses TLS. (default: true)
	EnableTLS *bool `json:"enable_tls,omitempty"`
	// The path of the CA certificates.
	CAPath *secret.Secret `json:"ca_path,omitempty"`
}

func (c *MattermostOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "mattermost"
	mattermost := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(c); err != nil {
		return nil, err
	} else {
		mattermost.Params = params
	}
	return mattermost, nil
}
