// Copyright © 2021 Cisco Systems, Inc. and/or its affiliates
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

// +name:"SQS"
// +weight:"200"
type _hugoSQS interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[SQS Output](https://github.com/ixixi/fluent-plugin-sqs)"
// Fluentd output plugin for SQS.
type _docSQS interface{} //nolint:deadcode,unused

// +name:"SQS"
// +url:"https://github.com/ixixi/fluent-plugin-sqs"
// +version:"v2.1.0"
// +description:"Output plugin writes fluent-events as queue messages to Amazon SQS"
// +status:"Testing"
type _metaSQS interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type SQSOutputConfig struct {
	// SQS queue url e.g. https://sqs.us-west-2.amazonaws.com/123456789012/myqueue
	SQSUrl string `json:"sqs_url,omitempty"`
	// SQS queue name - required if sqs_url is not set
	QueueName string `json:"queue_name,omitempty"`
	// AWS access key id
	AWSKeyId *secret.Secret `json:"aws_key_id,omitempty"`
	// AWS secret key
	AWSSecKey *secret.Secret `json:"aws_sec_key,omitempty"`
	// Create SQS queue (default: true)
	CreateQueue *bool `json:"create_queue,omitempty"`
	// AWS region (default: ap-northeast-1)
	Region string `json:"region,omitempty"`
	// Message group id for FIFO queue
	MessageGroupId string `json:"message_group_id,omitempty"`
	// Delivery delay seconds (default: 0)
	DelaySeconds int `json:"delay_seconds,omitempty"`
	// Include tag (default: true)
	IncludeTag *bool `json:"include_tag,omitempty"`
	// Tags property name in json (default: '__tag')
	TagPropertyName string `json:"tag_property_name,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

// ## Example `SQS` output configurations
// ```yaml
// apiVersion: logging.banzaicloud.io/v1beta1
// kind: Output
// metadata:
//
//	name: sqs-output-sample
//
// spec:
//
//	sqs:
//	  queue_name: some-aws-sqs-queue
//	  create_queue: false
//	  region: us-east-1
//
// ```
//
// #### Fluentd Config Result
// ```
//
//	<match **>
//	    @type sqs
//	    @id test_sqs
//	    queue_name some-aws-sqs-queue
//	    create_queue false
//	    region us-east-1
//	</match>
//
// ```
type _expSQS interface{} //nolint:deadcode,unused

func (s *SQSOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	pluginType := "sqs"
	sqs := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(s); err != nil {
		return nil, err
	} else {
		sqs.Params = params
	}
	if s.Buffer == nil {
		s.Buffer = &Buffer{}
	}
	if buffer, err := s.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		sqs.SubDirectives = append(sqs.SubDirectives, buffer)
	}

	return sqs, nil
}
