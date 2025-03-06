// Copyright © 2022 Cisco Systems, Inc. and/or its affiliates
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

// +name:"OpenSearch"
// +weight:"200"
type _hugoOpenSearch interface{} //nolint:deadcode,unused

// +docName:"OpenSearch output plugin for Fluentd"
/*
For details, see [https://github.com/fluent/fluent-plugin-opensearch](https://github.com/fluent/fluent-plugin-opensearch).

For an example deployment, see [Save all logs to OpenSearch](../../../../quickstarts/es-nginx/).

## Example output configurations

```yaml
spec:
  opensearch:
    host: opensearch-cluster.default.svc.cluster.local
    port: 9200
    scheme: https
    ssl_verify: false
    ssl_version: TLSv1_2
    buffer:
      timekey: 1m
      timekey_wait: 30s
      timekey_use_utc: true
```
*/
type _docOpenSearch interface{} //nolint:deadcode,unused

// +name:"OpenSearch"
// +url:"https://github.com/fluent/fluent-plugin-opensearch/releases/tag/v1.0.5"
// +version:"1.0.5"
// +description:"Send your logs to OpenSearch"
// +status:"GA"
type _metaOpenSearch interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"OpenSearch"
// Send your logs to OpenSearch
type OpenSearchOutput struct {
	// You can specify OpenSearch host by this parameter. (default:localhost)
	Host string `json:"host,omitempty"`
	// You can specify OpenSearch port by this parameter.(default: 9200)
	Port int `json:"port,omitempty"`
	// User for HTTP Basic authentication. This plugin will escape required URL encoded characters within %{} placeholders. e.g. `%{demo+}`
	User string `json:"user,omitempty"`
	// Password for HTTP Basic authentication.
	// +docLink:"Secret,../secret/"
	Password *secret.Secret `json:"password,omitempty"`
	// Path for HTTP Basic authentication.
	Path string `json:"path,omitempty"`
	// Connection scheme (default: http)
	Scheme string `json:"scheme,omitempty"`
	// You can specify multiple OpenSearch hosts with separator ",". If you specify hosts option, host and port options are ignored.
	Hosts string `json:"hosts,omitempty"`
	// Tell this plugin to find the index name to write to in the record under this key in preference to other mechanisms. Key can be specified as path to nested record using dot ('.') as a separator.
	TargetIndexKey string `json:"target_index_key,omitempty"`
	// The format of the time stamp field (@timestamp or what you specify with `time_key`). This parameter only has an effect when logstash_format is true as it only affects the name of the index we write to.
	TimeKeyFormat string `json:"time_key_format,omitempty"`
	// Should the record not include a time_key, define the degree of sub-second time precision to preserve from the time portion of the routed event.
	TimePrecision string `json:"time_precision,omitempty"`
	// Adds a @timestamp field to the log, following all settings logstash_format does, except without the restrictions on index_name. This allows one to log to an alias in OpenSearch and utilize the rollover API.(default: false)
	IncludeTimestamp bool `json:"include_timestamp,omitempty"`
	// Enable Logstash log format.(default: false)
	LogstashFormat bool `json:"logstash_format,omitempty"`
	// Set the Logstash prefix.(default: logstash)
	LogstashPrefix string `json:"logstash_prefix,omitempty"`
	// Set the Logstash prefix separator.(default: -)
	LogstashPrefixSeparator string `json:"logstash_prefix_separator,omitempty"`
	// Set the Logstash date format.(default: %Y.%m.%d)
	LogstashDateformat string `json:"logstash_dateformat,omitempty"`
	// By default, the records inserted into index logstash-YYMMDD with UTC (Coordinated Universal Time). This option allows to use local time if you describe `utc_index` to false.
	// +kubebuilder:validation:Optional
	UtcIndex *bool `json:"utc_index,omitempty" plugin:"default:true"`
	// Suppress type name to avoid warnings in OpenSearch
	SuppressTypeName *bool `json:"suppress_type_name,omitempty"`
	// The index name to write events to (default: fluentd)
	IndexName string `json:"index_name,omitempty"`
	// Field on your data to identify the data uniquely
	IdKey string `json:"id_key,omitempty"`
	// The write_operation can be any of: (index,create,update,upsert)(default: index)
	WriteOperation string `json:"write_operation,omitempty"`
	// parent_key
	ParentKey string `json:"parent_key,omitempty"`
	// routing_key
	RoutingKey string `json:"routing_key,omitempty"`
	// You can specify HTTP request timeout.(default: 5s)
	RequestTimeout string `json:"request_timeout,omitempty"`
	// You can tune how the OpenSearch-transport host reloading feature works.(default: true)
	// +kubebuilder:validation:Optional
	ReloadConnections *bool `json:"reload_connections,omitempty" plugin:"default:true"`
	// Indicates that the OpenSearch-transport will try to reload the nodes addresses if there is a failure while making the request, this can be useful to quickly remove a dead node from the list of addresses.(default: false)
	ReloadOnFailure bool `json:"reload_on_failure,omitempty"`
	// This setting allows custom routing of messages in response to bulk request failures. The default behavior is to emit failed records using the same tag that was provided.
	RetryTag string `json:"retry_tag,omitempty"`
	// You can set in the OpenSearch-transport how often dead connections from the OpenSearch-transport's pool will be resurrected.(default: 60s)
	ResurrectAfter string `json:"resurrect_after,omitempty"`
	// By default, when inserting records in Logstash format, @timestamp is dynamically created with the time at log ingestion. If you'd like to use a custom time, include an @timestamp with your record.
	TimeKey string `json:"time_key,omitempty"`
	// time_key_exclude_timestamp (default: false)
	TimeKeyExcludeTimestamp bool `json:"time_key_exclude_timestamp,omitempty"`
	// +kubebuilder:validation:Optional
	// Skip ssl verification (default: true)
	SslVerify *bool `json:"ssl_verify,omitempty" plugin:"default:true"`
	// Client certificate key
	SSLClientCertKey *secret.Secret `json:"client_key,omitempty"`
	// Client certificate
	SSLClientCert *secret.Secret `json:"client_cert,omitempty"`
	// Client key password
	SSLClientCertKeyPass *secret.Secret `json:"client_key_pass,omitempty"`
	// CA certificate
	SSLCACert *secret.Secret `json:"ca_file,omitempty"`
	// If you want to configure SSL/TLS version, you can specify ssl_version parameter. [SSLv23, TLSv1, TLSv1_1, TLSv1_2]
	SslVersion string `json:"ssl_version,omitempty"`
	// Remove keys on update will not update the configured keys in OpenSearch when a record is being updated. This setting only has any effect if the write operation is update or upsert.
	RemoveKeysOnUpdate string `json:"remove_keys_on_update,omitempty"`
	// This setting allows remove_keys_on_update to be configured with a key in each record, in much the same way as target_index_key works.
	RemoveKeysOnUpdateKey string `json:"remove_keys_on_update_key,omitempty"`
	// [https://github.com/fluent/fluent-plugin-opensearch#hash-flattening](https://github.com/fluent/fluent-plugin-opensearch#hash-flattening)
	FlattenHashes bool `json:"flatten_hashes,omitempty"`
	// Flatten separator
	FlattenHashesSeparator string `json:"flatten_hashes_separator,omitempty"`
	// The name of the template to define. If a template by the name given is already present, it will be left unchanged, unless template_overwrite is set, in which case the template will be updated.
	TemplateName string `json:"template_name,omitempty"`
	// The path to the file containing the template to install.
	// +docLink:"Secret,../secret/"
	TemplateFile *secret.Secret `json:"template_file,omitempty"`
	// Always update the template, even if it already exists.(default: false)
	TemplateOverwrite bool `json:"template_overwrite,omitempty"`
	// Specify the string and its value to be replaced in form of hash. Can contain multiple key value pair that would be replaced in the specified template_file. This setting only creates template and to add rollover index please check the rollover_index configuration.
	CustomizeTemplate string `json:"customize_template,omitempty"`
	// Specify this to override the index date pattern for creating a rollover index.(default: now/d)
	IndexDatePattern *string `json:"index_date_pattern,omitempty"`
	// index_separator (default: -)
	IndexSeparator string `json:"index_separator,omitempty"`
	// Specify the application name for the rollover index to be created.(default: default)
	ApplicationName *string `json:"application_name,omitempty"`
	// Specify index templates in form of hash. Can contain multiple templates.
	Templates string `json:"templates,omitempty"`
	// You can specify times of retry putting template.(default: 10)
	MaxRetryPuttingTemplate string `json:"max_retry_putting_template,omitempty"`
	// Indicates whether to fail when max_retry_putting_template is exceeded. If you have multiple output plugin, you could use this property to do not fail on Fluentd statup.(default: true)
	// +kubebuilder:validation:Optional
	FailOnPuttingTemplateRetryExceed *bool `json:"fail_on_putting_template_retry_exceed,omitempty" plugin:"default:true"`
	// fail_on_detecting_os_version_retry_exceed (default: true)
	// +kubebuilder:validation:Optional
	FailOnDetectingOsVersionRetryExceed *bool `json:"fail_on_detecting_os_version_retry_exceed,omitempty" plugin:"default:true"`
	//  max_retry_get_os_version (default: 15)
	MaxRetryGetOsVersion int `json:"max_retry_get_os_version,omitempty"`
	// This will add the Fluentd tag in the JSON record.(default: false)
	IncludeTagKey bool `json:"include_tag_key,omitempty"`
	// This will add the Fluentd tag in the JSON record.(default: tag)
	TagKey string `json:"tag_key,omitempty"`
	// With logstash_format true, OpenSearch plugin parses timestamp field for generating index name. If the record has invalid timestamp value, this plugin emits an error event to @ERROR label with `time_parse_error_tag` configured tag.
	TimeParseErrorTag string `json:"time_parse_error_tag,omitempty"`
	// Indicates that the plugin should reset connection on any error (reconnect on next send). By default it will reconnect only on "host unreachable exceptions". We recommended to set this true in the presence of OpenSearch shield.(default: false)
	ReconnectOnError bool `json:"reconnect_on_error,omitempty"`
	// This param is to set a pipeline ID of your OpenSearch to be added into the request, you can configure ingest node.
	Pipeline string `json:"pipeline,omitempty"`
	// This is debugging purpose option to enable to obtain transporter layer log. (default: false)
	WithTransporterLog bool `json:"with_transporter_log,omitempty"`
	//  emit_error_for_missing_id (default: false)
	EmitErrorForMissingID bool `json:"emit_error_for_missing_id,omitempty"`
	// The default Sniffer used by the OpenSearch::Transport class works well when Fluentd has a direct connection to all of the OpenSearch servers and can make effective use of the _nodes API. This doesn't work well when Fluentd must connect through a load balancer or proxy. The `sniffer_class_name` parameter gives you the ability to provide your own Sniffer class to implement whatever connection reload logic you require. In addition, there is a new Fluent::Plugin::OpenSearchSimpleSniffer class which reuses the hosts given in the configuration, which is typically the hostname of the load balancer or proxy. For example, a configuration like this would cause connections to logging-os to reload every 100 operations: [https://github.com/fluent/fluent-plugin-opensearch#sniffer-class-name](https://github.com/fluent/fluent-plugin-opensearch#sniffer-class-name).
	SnifferClassName string `json:"sniffer_class_name,omitempty"`
	// selector_class_name
	SelectorClassName string `json:"selector_class_name,omitempty"`
	// When reload_connections true, this is the integer number of operations after which the plugin will reload the connections. The default value is 10000.
	ReloadAfter string `json:"reload_after,omitempty"`
	// With this option set to true, Fluentd manifests the index name in the request URL (rather than in the request body). You can use this option to enforce an URL-based access control.
	IncludeIndexInUrl bool `json:"include_index_in_url,omitempty"`
	// With http_backend typhoeus, the opensearch plugin uses typhoeus faraday http backend. Typhoeus can handle HTTP keepalive. (default: excon)
	HttpBackend string `json:"http_backend,omitempty"`
	// http_backend_excon_nonblock
	// +kubebuilder:validation:Optional
	HttpBackendExconNonblock *bool `json:"http_backend_excon_nonblock,omitempty" plugin:"default:true"`
	// When you use mismatched OpenSearch server and client libraries, fluent-plugin-opensearch cannot send data into OpenSearch.  (default: false)
	ValidateClientVersion bool `json:"validate_client_version,omitempty"`
	// With default behavior, OpenSearch client uses Yajl as JSON encoder/decoder. Oj is the alternative high performance JSON encoder/decoder. When this parameter sets as true, OpenSearch client uses Oj as JSON encoder/decoder. (default: false)
	PreferOjSerializer bool `json:"prefer_oj_serializer,omitempty"`
	// Default unrecoverable_error_types parameter is set up strictly. Because rejected_execution_exception is caused by exceeding OpenSearch's thread pool capacity. Advanced users can increase its capacity, but normal users should follow default behavior.
	UnrecoverableErrorTypes string `json:"unrecoverable_error_types,omitempty"`
	// unrecoverable_record_types
	UnrecoverableRecordTypes string `json:"unrecoverable_record_types,omitempty"`
	// emit_error_label_event (default: true)
	// +kubebuilder:validation:Optional
	EmitErrorLabelEvent *bool `json:"emit_error_label_event,omitempty" plugin:"default:true"`
	// verify_os_version_at_startup (default: true)
	// +kubebuilder:validation:Optional
	VerifyOsVersionAtStartup *bool `json:"verify_os_version_at_startup,omitempty" plugin:"default:true"`
	//  max_retry_get_os_version (default: 1)
	DefaultOpensearchVersion int `json:"default_opensearch_version,omitempty"`
	// log_os_400_reason (default: false)
	LogOs400Reason bool `json:"log_os_400_reason,omitempty"`
	// This parameter adds additional headers to request. Example: `{"token":"secret"}` (default: {})
	CustomHeaders string `json:"custom_headers,omitempty"`
	// By default, record body is wrapped by 'doc'. This behavior can not handle update script requests. You can set this to suppress doc wrapping and allow record body to be untouched. (default: false)
	SuppressDocWrap bool `json:"suppress_doc_wrap,omitempty"`
	// A list of exception that will be ignored - when the exception occurs the chunk will be discarded and the buffer retry mechanism won't be called. It is possible also to specify classes at higher level in the hierarchy.
	IgnoreExceptions string `json:"ignore_exceptions,omitempty"`
	// Indicates whether to backup chunk when ignore exception occurs.
	// +kubebuilder:validation:Optional
	ExceptionBackup *bool `json:"exception_backup,omitempty" plugin:"default:true"`
	// Configure bulk_message request splitting threshold size.
	// Default value is 20MB. (20 * 1024 * 1024)
	// If you specify this size as negative number, bulk_message request splitting feature will be disabled. (default: 20MB)
	BulkMessageRequestThreshold string `json:"bulk_message_request_threshold,omitempty"`
	// compression_level
	CompressionLevel string `json:"compression_level,omitempty"`
	// truncate_caches_interval
	TruncateCachesInterval string `json:"truncate_caches_interval,omitempty"`
	// Specify wether to use legacy template or not. (default: true)
	// +kubebuilder:validation:Optional
	UseLegacyTemplate *bool `json:"use_legacy_template,omitempty"`
	// catch_transport_exception_on_retry (default: true)
	// +kubebuilder:validation:Optional
	CatchTransportExceptionOnRetry *bool `json:"catch_transport_exception_on_retry,omitempty" plugin:"default:true"`
	// target_index_affinity (default: false)
	TargetIndexAffinity bool `json:"target_index_affinity,omitempty"`

	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, Fluentd logs a warning message and increases the  `fluentd_output_status_slow_flush_count` metric.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`

	// Use @type opensearch_data_stream
	DataStreamEnable *bool `json:"data_stream_enable,omitempty" plugin:"hidden"`
	// You can specify Opensearch data stream name by this parameter. This parameter is mandatory for opensearch_data_stream.
	DataStreamName string `json:"data_stream_name,omitempty"`
	// Specify an existing index template for the data stream. If not present, a new template is created and named after the data stream. (default: data_stream_name)
	DataStreamTemplateName string `json:"data_stream_template_name,omitempty"`

	// AWS Endpoint Credentials
	Endpoint *OpenSearchEndpointCredentials `json:"endpoint,omitempty"`
}

func (o *OpenSearchEndpointCredentials) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	return types.NewFlatDirective(types.PluginMeta{
		Directive: "endpoint",
	}, o, secretLoader)
}

func (e *OpenSearchOutput) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	pluginType := "opensearch"
	if e.DataStreamEnable != nil && *e.DataStreamEnable {
		pluginType = "opensearch_data_stream"
	}
	opensearch := &types.OutputPlugin{
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
		opensearch.Params = params
	}
	if e.Endpoint != nil {
		if assumeRoleCredentials, err := e.Endpoint.ToDirective(secretLoader, id); err != nil {
			return nil, err
		} else {
			opensearch.SubDirectives = append(opensearch.SubDirectives, assumeRoleCredentials)
		}
	}
	if e.Buffer == nil {
		e.Buffer = &Buffer{}
	}
	if buffer, err := e.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		opensearch.SubDirectives = append(opensearch.SubDirectives, buffer)
	}
	return opensearch, nil
}

// +kubebuilder:object:generate=true
type OpenSearchEndpointCredentials struct {
	// AWS region. It should be in form like us-east-1, us-west-2. Default nil, which means try to find from environment variable AWS_REGION.
	Region string `json:"region,omitempty"`

	// AWS connection url.
	Url string `json:"url"`

	// AWS access key id. This parameter is required when your agent is not running on EC2 instance with an IAM Role.
	AccessKeyId *secret.Secret `json:"access_key_id,omitempty"`

	// AWS secret key. This parameter is required when your agent is not running on EC2 instance with an IAM Role.
	SecretAccessKey *secret.Secret `json:"secret_access_key,omitempty"`

	// Typically, you can use AssumeRole for cross-account access or federation.
	AssumeRoleArn *secret.Secret `json:"assume_role_arn,omitempty"`

	// Set with AWS_CONTAINER_CREDENTIALS_RELATIVE_URI environment variable value
	EcsContainerCredentialsRelativeUri *secret.Secret `json:"ecs_container_credentials_relative_uri,omitempty"`

	// [AssumeRoleWithWebIdentity](https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRoleWithWebIdentity.html)
	AssumeRoleSessionName *secret.Secret `json:"assume_role_session_name,omitempty"`

	// [AssumeRoleWithWebIdentity](https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRoleWithWebIdentity.html)
	AssumeRoleWebIdentityTokenFile *secret.Secret `json:"assume_role_web_identity_token_file,omitempty"`

	// By default, the AWS Security Token Service (AWS STS) is available as a global service, and all AWS STS requests go to a single endpoint at https://sts.amazonaws.com. AWS recommends using Regional AWS STS endpoints instead of the global endpoint to reduce latency, build in redundancy, and increase session token validity. https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_enable-regions.html
	StsCredentialsRegion *secret.Secret `json:"sts_credentials_region,omitempty"`
}
