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
	"github.com/cisco-open/operator-tools/pkg/secret"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Amazon CloudWatch"
// +weight:"200"
type _hugoCloudWatch interface{} //nolint:deadcode,unused

// +docName:"CloudWatch output plugin for Fluentd"
/*
This plugin outputs logs or metrics to Amazon CloudWatch. For details, see [https://github.com/fluent-plugins-nursery/fluent-plugin-cloudwatch-logs](https://github.com/fluent-plugins-nursery/fluent-plugin-cloudwatch-logs).

## Example output configurations
```yaml
spec:
cloudwatch:
  aws_key_id:
    valueFrom:
      secretKeyRef:
        name: logging-s3
        key: awsAccessKeyId
  aws_sec_key:
    valueFrom:
      secretKeyRef:
        name: logging-s3
        key: awsSecretAccessKey
  log_group_name: operator-log-group
  log_stream_name: operator-log-stream
  region: us-east-1
  auto_create_stream true
  buffer:
    timekey: 30s
    timekey_wait: 30s
    timekey_use_utc: true
```
*/
type _docCloudWatch interface{} //nolint:deadcode,unused

// +name:"Amazon CloudWatch"
// +url:"https://github.com/fluent-plugins-nursery/fluent-plugin-cloudwatch-logs/releases/tag/v0.14.2"
// +version:"0.14.2"
// +description:"Send your logs to AWS CloudWatch"
// +status:"GA"
type _metaCloudWatch interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type CloudWatchOutput struct {

	//  Create log group and stream automatically. (default: false)
	AutoCreateStream bool `json:"auto_create_stream,omitempty"`
	// AWS access key id
	// +docLink:"Secret,../secret/"
	AwsAccessKey *secret.Secret `json:"aws_key_id,omitempty"`
	// AWS secret key.
	// +docLink:"Secret,../secret/"
	AwsSecretKey *secret.Secret `json:"aws_sec_key,omitempty"`
	// Instance Profile Credentials call retries (default: nil)
	AwsInstanceProfileCredentialsRetries int `json:"aws_instance_profile_credentials_retries,omitempty"`
	// Enable AssumeRoleCredentials to authenticate, rather than the default credential hierarchy. See 'Cross-Account Operation' below for more detail.
	AwsUseSts bool `json:"aws_use_sts,omitempty"`
	// The role ARN to assume when using cross-account sts authentication
	AwsStsRoleArn string `json:"aws_sts_role_arn,omitempty"`
	// The session name to use with sts authentication  (default: 'fluentd')
	AwsStsSessionName string `json:"aws_sts_session_name,omitempty"`
	// Use to set the number of threads pushing data to CloudWatch. (default: 1)
	Concurrency int `json:"concurrency,omitempty"`
	// Use this parameter to connect to the local API endpoint (for testing)
	Endpoint string `json:"endpoint,omitempty"`
	// Use to set an optional HTTP proxy
	HttpProxy string `json:"http_proxy,omitempty"`
	// Include time key as part of the log entry (default: UTC)
	IncludeTimeKey bool `json:"include_time_key,omitempty"`
	// Name of the library to be used to handle JSON data. For now, supported libraries are json (default) and yaml
	JsonHandler string `json:"json_handler,omitempty"`
	// Use localtime timezone for include_time_key output (overrides UTC default)
	Localtime bool `json:"localtime,omitempty"`
	// Set a hash with keys and values to tag the log group resource
	LogGroupAwsTags string `json:"log_group_aws_tags,omitempty"`
	// Specified field of records as AWS tags for the log group
	LogGroupAwsTagsKey string `json:"log_group_aws_tags_key,omitempty"`
	// Name of log group to store logs
	LogGroupName string `json:"log_group_name,omitempty"`
	// Specified field of records as log group name
	LogGroupNameKey string `json:"log_group_name_key,omitempty"`
	// Output rejected_log_events_info request log. (default: false)
	LogRejectedRequest string `json:"log_rejected_request,omitempty"`
	// Name of log stream to store logs
	LogStreamName string `json:"log_stream_name,omitempty"`
	// Specified field of records as log stream name
	LogStreamNameKey string `json:"log_stream_name_key,omitempty"`
	// Maximum number of events to send at once (default: 10000)
	MaxEventsPerBatch int `json:"max_events_per_batch,omitempty"`
	// Maximum length of the message
	MaxMessageLength int `json:"max_message_length,omitempty"`
	// Keys to send messages as events
	MessageKeys string `json:"message_keys,omitempty"`
	// If true, put_log_events_retry_limit will be ignored
	PutLogEventsDisableRetryLimit bool `json:"put_log_events_disable_retry_limit,omitempty"`
	// Maximum count of retry (if exceeding this, the events will be discarded)
	PutLogEventsRetryLimit int `json:"put_log_events_retry_limit,omitempty"`
	// Time before retrying PutLogEvents (retry interval increases exponentially like put_log_events_retry_wait * (2 ^ retry_count))
	PutLogEventsRetryWait string `json:"put_log_events_retry_wait,omitempty"`
	// AWS Region
	Region string `json:"region"`
	// Remove field specified by log_group_aws_tags_key
	RemoveLogGroupAwsTagsKey string `json:"remove_log_group_aws_tags_key,omitempty"`
	// Remove field specified by log_group_name_key
	RemoveLogGroupNameKey string `json:"remove_log_group_name_key,omitempty"`
	// Remove field specified by log_stream_name_key
	RemoveLogStreamNameKey string `json:"remove_log_stream_name_key,omitempty"`
	// Remove field specified by retention_in_days
	RemoveRetentionInDays string `json:"remove_retention_in_days,omitempty"`
	// Use to set the expiry time for log group when created with auto_create_stream. (default to no expiry)
	RetentionInDays string `json:"retention_in_days,omitempty"`
	// Use specified field of records as retention period
	RetentionInDaysKey string `json:"retention_in_days_key,omitempty"`
	// Use tag as a group name
	UseTagAsGroup bool `json:"use_tag_as_group,omitempty"`
	// Use tag as a stream name
	UseTagAsStream bool `json:"use_tag_as_stream,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
	// +docLink:"Format,../format/"
	Format *Format `json:"format,omitempty"`
}

func (c *CloudWatchOutput) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "cloudwatch_logs"
	cloudwatch := &types.OutputPlugin{
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
		cloudwatch.Params = params
	}
	if c.Buffer == nil {
		c.Buffer = &Buffer{}
	}
	if buffer, err := c.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		cloudwatch.SubDirectives = append(cloudwatch.SubDirectives, buffer)
	}
	if c.Format != nil {
		if format, err := c.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			cloudwatch.SubDirectives = append(cloudwatch.SubDirectives, format)
		}
	}
	return cloudwatch, nil
}
