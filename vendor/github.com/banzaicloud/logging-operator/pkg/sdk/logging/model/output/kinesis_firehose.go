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

package output

import (
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

// +name:"Amazon Kinesis"
// +weight:"200"
type _hugoKinesisFirehose interface{} //nolint:deadcode,unused

// +docName:"Kinesis Firehose output plugin for Fluentd"
//
//	More info at https://github.com/awslabs/aws-fluent-plugin-kinesis#configuration-kinesis_firehose
//
// ## Example output configurations
// ```yaml
// spec:
//
//	kinesisFirehose:
//	  delivery_stream_name: example-stream-name
//	  region: us-east-1
//	  format:
//	    type: json
//
// ```
type _docKinesisFirehose interface{} //nolint:deadcode,unused

// +name:"Amazon Kinesis Firehose"
// +url:"https://github.com/awslabs/aws-fluent-plugin-kinesis/releases/tag/v3.4.2"
// +version:"3.4.2"
// +description:"Fluent plugin for Amazon Kinesis"
// +status:"Testing"
type _metaKinesisFirehose interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"KinesisStream"
// Send your logs to a Kinesis Stream
type KinesisFirehoseOutputConfig struct {
	// Name of the delivery stream to put data.
	DeliveryStreamName string `json:"delivery_stream_name"`

	// If it is enabled, the plugin adds new line character (\n) to each serialized record.
	//Before appending \n, plugin calls chomp and removes separator from the end of each record as chomp_record is true. Therefore, you don't need to enable chomp_record option when you use kinesis_firehose output with default configuration (append_new_line is true). If you want to set append_new_line false, you can choose chomp_record false (default) or true (compatible format with plugin v2). (Default:true)
	AppendNewLine *bool `json:"append_new_line,omitempty"`

	// AWS access key id. This parameter is required when your agent is not running on EC2 instance with an IAM Role.
	AWSKeyId *secret.Secret `json:"aws_key_id,omitempty"`

	// AWS secret key. This parameter is required when your agent is not running on EC2 instance with an IAM Role.
	AWSSECKey *secret.Secret `json:"aws_sec_key,omitempty"`

	// AWS session token. This parameter is optional, but can be provided if using MFA or temporary credentials when your agent is not running on EC2 instance with an IAM Role.
	AWSSESToken *secret.Secret `json:"aws_ses_token,omitempty"`

	// The number of attempts to make (with exponential backoff) when loading instance profile credentials from the EC2 metadata service using an IAM role. Defaults to 5 retries.
	AWSIAMRetries int `json:"aws_iam_retries,omitempty"`

	// Typically, you can use AssumeRole for cross-account access or federation.
	AssumeRoleCredentials *KinesisFirehoseAssumeRoleCredentials `json:"assume_role_credentials,omitempty"`

	// This loads AWS access credentials from an external process.
	ProcessCredentials *KinesisFirehoseProcessCredentials `json:"process_credentials,omitempty"`

	// AWS region of your stream. It should be in form like us-east-1, us-west-2. Default nil, which means try to find from environment variable AWS_REGION.
	Region string `json:"region,omitempty"`

	// The plugin will put multiple records to Amazon Kinesis Data Streams in batches using PutRecords. A set of records in a batch may fail for reasons documented in the Kinesis Service API Reference for PutRecords. Failed records will be retried retries_on_batch_request times
	RetriesOnBatchRequest int `json:"retries_on_batch_request,omitempty"`

	// Boolean, default true. If enabled, when after retrying, the next retrying checks the number of succeeded records on the former batch request and reset exponential backoff if there is any success. Because batch request could be composed by requests across shards, simple exponential backoff for the batch request wouldn't work some cases.
	ResetBackoffIfSuccess bool `json:"reset_backoff_if_success,omitempty"`

	// Integer, default 500. The number of max count of making batch request from record chunk. It can't exceed the default value because it's API limit.
	BatchRequestMaxCount int `json:"batch_request_max_count,omitempty"`

	// Integer. The number of max size of making batch request from record chunk. It can't exceed the default value because it's API limit.
	BatchRequestMaxSize int `json:"batch_request_max_size,omitempty"`

	// +docLink:"Format,../format/"
	Format *Format `json:"format,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Assume Role Credentials"
// assume_role_credentials
type KinesisFirehoseAssumeRoleCredentials struct {
	// The Amazon Resource Name (ARN) of the role to assume
	RoleArn string `json:"role_arn"`
	// An identifier for the assumed role session
	RoleSessionName string `json:"role_session_name"`
	// An IAM policy in JSON format
	Policy string `json:"policy,omitempty"`
	// The duration, in seconds, of the role session (900-3600)
	DurationSeconds string `json:"duration_seconds,omitempty"`
	// A unique identifier that is used by third parties when assuming roles in their customers' accounts.
	ExternalId string `json:"external_id,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Process Credentials"
// process_credentials
type KinesisFirehoseProcessCredentials struct {
	// Command more info: https://docs.aws.amazon.com/sdk-for-ruby/v3/api/Aws/ProcessCredentials.html
	Process string `json:"process"`
}

func (o *KinesisFirehoseProcessCredentials) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	return types.NewFlatDirective(types.PluginMeta{
		Directive: "process_credentials",
	}, o, secretLoader)
}

func (o *KinesisFirehoseAssumeRoleCredentials) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	return types.NewFlatDirective(types.PluginMeta{
		Directive: "assume_role_credentials",
	}, o, secretLoader)
}

func (e *KinesisFirehoseOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "kinesis_firehose"
	kinesis := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(e); err != nil {
		return nil, err
	} else {
		kinesis.Params = params
	}
	if e.AssumeRoleCredentials != nil {
		if assumeRoleCredentials, err := e.AssumeRoleCredentials.ToDirective(secretLoader, id); err != nil {
			return nil, err
		} else {
			kinesis.SubDirectives = append(kinesis.SubDirectives, assumeRoleCredentials)
		}
	}
	if e.ProcessCredentials != nil {
		if processCredentials, err := e.ProcessCredentials.ToDirective(secretLoader, id); err != nil {
			return nil, err
		} else {
			kinesis.SubDirectives = append(kinesis.SubDirectives, processCredentials)
		}
	}
	if e.Buffer == nil {
		e.Buffer = &Buffer{}
	}
	if buffer, err := e.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		kinesis.SubDirectives = append(kinesis.SubDirectives, buffer)
	}

	if e.Format != nil {
		if format, err := e.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			kinesis.SubDirectives = append(kinesis.SubDirectives, format)
		}
	}
	return kinesis, nil
}
