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

// +name:"Elasticsearch"
// +weight:"200"
type _hugoElasticsearch interface{} //nolint:deadcode,unused

// +docName:"Elasticsearch output plugin for Fluentd"
// More info at https://github.com/uken/fluent-plugin-elasticsearch
// >Example Deployment: [Save all logs to ElasticSearch](../../../../quickstarts/es-nginx/)
//
// ## Example output configurations
// ```yaml
// spec:
//
//	elasticsearch:
//	  host: elasticsearch-elasticsearch-cluster.default.svc.cluster.local
//	  port: 9200
//	  scheme: https
//	  ssl_verify: false
//	  ssl_version: TLSv1_2
//	  buffer:
//	    timekey: 1m
//	    timekey_wait: 30s
//	    timekey_use_utc: true
//
// ```
type _docElasticsearch interface{} //nolint:deadcode,unused

// +name:"Elasticsearch"
// +url:"https://github.com/uken/fluent-plugin-elasticsearch/releases/tag/v5.1.4"
// +version:"5.1.1"
// +description:"Send your logs to Elasticsearch"
// +status:"GA"
type _metaElasticsearch interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Elasticsearch"
// Send your logs to Elasticsearch
type ElasticsearchOutput struct {
	// You can specify Elasticsearch host by this parameter. (default:localhost)
	Host string `json:"host,omitempty"`
	// You can specify Elasticsearch port by this parameter.(default: 9200)
	Port int `json:"port,omitempty"`
	// You can specify multiple Elasticsearch hosts with separator ",". If you specify hosts option, host and port options are ignored.
	Hosts string `json:"hosts,omitempty"`
	// User for HTTP Basic authentication. This plugin will escape required URL encoded characters within %{} placeholders. e.g. %{demo+}
	User string `json:"user,omitempty"`
	// Password for HTTP Basic authentication.
	// +docLink:"Secret,../secret/"
	Password *secret.Secret `json:"password,omitempty"`
	// Path for HTTP Basic authentication.
	Path string `json:"path,omitempty"`
	// Connection scheme (default: http)
	Scheme string `json:"scheme,omitempty"`
	// +kubebuilder:validation:Optional
	// Skip ssl verification (default: true)
	SslVerify *bool `json:"ssl_verify,omitempty" plugin:"default:true"`
	// If you want to configure SSL/TLS version, you can specify ssl_version parameter. [SSLv23, TLSv1, TLSv1_1, TLSv1_2]
	SslVersion string `json:"ssl_version,omitempty"`
	// Specify min/max SSL/TLS version
	SslMaxVersion string `json:"ssl_max_version,omitempty"`
	SslMinVersion string `json:"ssl_min_version,omitempty"`
	// CA certificate
	SSLCACert *secret.Secret `json:"ca_file,omitempty"`
	// Client certificate
	SSLClientCert *secret.Secret `json:"client_cert,omitempty"`
	// Client certificate key
	SSLClientCertKey *secret.Secret `json:"client_key,omitempty"`
	// Client key password
	SSLClientCertKeyPass *secret.Secret `json:"client_key_pass,omitempty"`
	// Enable Logstash log format.(default: false)
	LogstashFormat bool `json:"logstash_format,omitempty"`
	// Adds a @timestamp field to the log, following all settings logstash_format does, except without the restrictions on index_name. This allows one to log to an alias in Elasticsearch and utilize the rollover API.(default: false)
	IncludeTimestamp bool `json:"include_timestamp,omitempty"`
	// Set the Logstash prefix.(default: logstash)
	LogstashPrefix string `json:"logstash_prefix,omitempty"`
	// Set the Logstash prefix separator.(default: -)
	LogstashPrefixSeparator string `json:"logstash_prefix_separator,omitempty"`
	// Set the Logstash date format.(default: %Y.%m.%d)
	LogstashDateformat string `json:"logstash_dateformat,omitempty"`
	// The index name to write events to (default: fluentd)
	IndexName string `json:"index_name,omitempty"`
	// Set the index type for elasticsearch. This is the fallback if `target_type_key` is missing. (default: fluentd)
	TypeName string `json:"type_name,omitempty"`
	// This param is to set a pipeline id of your elasticsearch to be added into the request, you can configure ingest node.
	Pipeline string `json:"pipeline,omitempty"`
	// The format of the time stamp field (@timestamp or what you specify with time_key). This parameter only has an effect when logstash_format is true as it only affects the name of the index we write to.
	TimeKeyFormat string `json:"time_key_format,omitempty"`
	// Should the record not include a time_key, define the degree of sub-second time precision to preserve from the time portion of the routed event.
	TimePrecision string `json:"time_precision,omitempty"`
	// By default, when inserting records in Logstash format, @timestamp is dynamically created with the time at log ingestion. If you'd like to use a custom time, include an @timestamp with your record.
	TimeKey string `json:"time_key,omitempty"`
	// By default, the records inserted into index logstash-YYMMDD with UTC (Coordinated Universal Time). This option allows to use local time if you describe utc_index to false.(default: true)
	// +kubebuilder:validation:Optional
	UtcIndex *bool `json:"utc_index,omitempty" plugin:"default:true"`
	// Suppress type name to avoid warnings in Elasticsearch 7.x
	SuppressTypeName *bool `json:"suppress_type_name,omitempty"`
	// Tell this plugin to find the index name to write to in the record under this key in preference to other mechanisms. Key can be specified as path to nested record using dot ('.') as a separator. https://github.com/uken/fluent-plugin-elasticsearch#target_index_key
	TargetIndexKey string `json:"target_index_key,omitempty"`
	// Similar to target_index_key config, find the type name to write to in the record under this key (or nested record). If key not found in record - fallback to type_name.(default: fluentd)
	TargetTypeKey string `json:"target_type_key,omitempty"`
	// The name of the template to define. If a template by the name given is already present, it will be left unchanged, unless template_overwrite is set, in which case the template will be updated.
	TemplateName string `json:"template_name,omitempty"`
	// The path to the file containing the template to install.
	// +docLink:"Secret,../secret/"
	TemplateFile *secret.Secret `json:"template_file,omitempty"`
	// Specify index templates in form of hash. Can contain multiple templates.
	Templates string `json:"templates,omitempty"`
	// Specify the string and its value to be replaced in form of hash. Can contain multiple key value pair that would be replaced in the specified template_file. This setting only creates template and to add rollover index please check the rollover_index configuration.
	CustomizeTemplate string `json:"customize_template,omitempty"`
	// Specify this as true when an index with rollover capability needs to be created.(default: false) https://github.com/uken/fluent-plugin-elasticsearch#rollover_index
	RolloverIndex bool `json:"rollover_index,omitempty"`
	// Specify this to override the index date pattern for creating a rollover index.(default: now/d)
	IndexDatePattern *string `json:"index_date_pattern,omitempty"`
	// Specify the deflector alias which would be assigned to the rollover index created. This is useful in case of using the Elasticsearch rollover API
	DeflectorAlias string `json:"deflector_alias,omitempty"`
	// Specify the index prefix for the rollover index to be created.(default: logstash)
	IndexPrefix string `json:"index_prefix,omitempty"`
	// Specify the application name for the rollover index to be created.(default: default)
	ApplicationName *string `json:"application_name,omitempty"`
	// Always update the template, even if it already exists.(default: false)
	TemplateOverwrite bool `json:"template_overwrite,omitempty"`
	// You can specify times of retry putting template.(default: 10)
	MaxRetryPuttingTemplate string `json:"max_retry_putting_template,omitempty"`
	// Indicates whether to fail when max_retry_putting_template is exceeded. If you have multiple output plugin, you could use this property to do not fail on fluentd statup.(default: true)
	// +kubebuilder:validation:Optional
	FailOnPuttingTemplateRetryExceed *bool `json:"fail_on_putting_template_retry_exceed,omitempty" plugin:"default:true"`
	// fail_on_detecting_es_version_retry_exceed (default: true)
	// +kubebuilder:validation:Optional
	FailOnDetectingEsVersionRetryExceed *bool `json:"fail_on_detecting_es_version_retry_exceed,omitempty" plugin:"default:true"`
	// You can specify times of retry obtaining Elasticsearch version.(default: 15)
	MaxRetryGetEsVersion string `json:"max_retry_get_es_version,omitempty"`
	// You can specify HTTP request timeout.(default: 5s)
	RequestTimeout string `json:"request_timeout,omitempty"`
	// You can tune how the elasticsearch-transport host reloading feature works.(default: true)
	// +kubebuilder:validation:Optional
	ReloadConnections *bool `json:"reload_connections,omitempty" plugin:"default:true"`
	//Indicates that the elasticsearch-transport will try to reload the nodes addresses if there is a failure while making the request, this can be useful to quickly remove a dead node from the list of addresses.(default: false)
	ReloadOnFailure bool `json:"reload_on_failure,omitempty"`
	// When reload_connections true, this is the integer number of operations after which the plugin will reload the connections. The default value is 10000.
	ReloadAfter string `json:"reload_after,omitempty"`
	// You can set in the elasticsearch-transport how often dead connections from the elasticsearch-transport's pool will be resurrected.(default: 60s)
	ResurrectAfter string `json:"resurrect_after,omitempty"`
	// This will add the Fluentd tag in the JSON record.(default: false)
	IncludeTagKey bool `json:"include_tag_key,omitempty"`
	// This will add the Fluentd tag in the JSON record.(default: tag)
	TagKey string `json:"tag_key,omitempty"`
	// https://github.com/uken/fluent-plugin-elasticsearch#id_key
	IdKey string `json:"id_key,omitempty"`
	// Similar to parent_key config, will add _routing into elasticsearch command if routing_key is set and the field does exist in input event.
	RoutingKey string `json:"routing_key,omitempty"`
	// https://github.com/uken/fluent-plugin-elasticsearch#remove_keys
	RemoveKeys string `json:"remove_keys,omitempty"`
	// Remove keys on update will not update the configured keys in elasticsearch when a record is being updated. This setting only has any effect if the write operation is update or upsert.
	RemoveKeysOnUpdate string `json:"remove_keys_on_update,omitempty"`
	// This setting allows remove_keys_on_update to be configured with a key in each record, in much the same way as target_index_key works.
	RemoveKeysOnUpdateKey string `json:"remove_keys_on_update_key,omitempty"`
	// This setting allows custom routing of messages in response to bulk request failures. The default behavior is to emit failed records using the same tag that was provided.
	RetryTag string `json:"retry_tag,omitempty"`
	// The write_operation can be any of: (index,create,update,upsert)(default: index)
	WriteOperation string `json:"write_operation,omitempty"`
	// Indicates that the plugin should reset connection on any error (reconnect on next send). By default it will reconnect only on "host unreachable exceptions". We recommended to set this true in the presence of elasticsearch shield.(default: false)
	ReconnectOnError bool `json:"reconnect_on_error,omitempty"`
	// This is debugging purpose option to enable to obtain transporter layer log. (default: false)
	WithTransporterLog bool `json:"with_transporter_log,omitempty"`
	// With content_type application/x-ndjson, elasticsearch plugin adds application/x-ndjson as Content-Profile in payload. (default: application/json)
	ContentType string `json:"content_type,omitempty"`
	//With this option set to true, Fluentd manifests the index name in the request URL (rather than in the request body). You can use this option to enforce an URL-based access control.
	IncludeIndexInUrl bool `json:"include_index_in_url,omitempty"`
	// With logstash_format true, elasticsearch plugin parses timestamp field for generating index name. If the record has invalid timestamp value, this plugin emits an error event to @ERROR label with time_parse_error_tag configured tag.
	TimeParseErrorTag string `json:"time_parse_error_tag,omitempty"`
	// With http_backend typhoeus, elasticsearch plugin uses typhoeus faraday http backend. Typhoeus can handle HTTP keepalive. (default: excon)
	HttpBackend string `json:"http_backend,omitempty"`
	// With default behavior, Elasticsearch client uses Yajl as JSON encoder/decoder. Oj is the alternative high performance JSON encoder/decoder. When this parameter sets as true, Elasticsearch client uses Oj as JSON encoder/decoder. (default: false)
	PreferOjSerializer bool `json:"prefer_oj_serializer,omitempty"`
	// Elasticsearch will complain if you send object and concrete values to the same field. For example, you might have logs that look this, from different places:
	//{"people" => 100} {"people" => {"some" => "thing"}}
	//The second log line will be rejected by the Elasticsearch parser because objects and concrete values can't live in the same field. To combat this, you can enable hash flattening.
	FlattenHashes bool `json:"flatten_hashes,omitempty"`
	// Flatten separator
	FlattenHashesSeparator string `json:"flatten_hashes_separator,omitempty"`
	// When you use mismatched Elasticsearch server and client libraries, fluent-plugin-elasticsearch cannot send data into Elasticsearch. (default: false)
	ValidateClientVersion bool `json:"validate_client_version,omitempty"`
	// Default unrecoverable_error_types parameter is set up strictly. Because es_rejected_execution_exception is caused by exceeding Elasticsearch's thread pool capacity. Advanced users can increase its capacity, but normal users should follow default behavior.
	// If you want to increase it and forcibly retrying bulk request, please consider to change unrecoverable_error_types parameter from default value.
	// Change default value of thread_pool.bulk.queue_size in elasticsearch.yml)
	UnrecoverableErrorTypes string `json:"unrecoverable_error_types,omitempty"`
	// Because Elasticsearch plugin should change behavior each of Elasticsearch major versions.
	// For example, Elasticsearch 6 starts to prohibit multiple type_names in one index, and Elasticsearch 7 will handle only _doc type_name in index.
	// If you want to disable to verify Elasticsearch version at start up, set it as false.
	// When using the following configuration, ES plugin intends to communicate into Elasticsearch 6. (default: true)
	// +kubebuilder:validation:Optional
	VerifyEsVersionAtStartup *bool `json:"verify_es_version_at_startup,omitempty" plugin:"default:true"`
	// This parameter changes that ES plugin assumes default Elasticsearch version.(default: 5)
	DefaultElasticsearchVersion string `json:"default_elasticsearch_version,omitempty"`
	// This parameter adds additional headers to request. Example: {"token":"secret"} (default: {})
	CustomHeaders string `json:"custom_headers,omitempty"`
	// api_key parameter adds authentication header.
	ApiKey *secret.Secret `json:"api_key,omitempty"`
	// By default, the error logger won't record the reason for a 400 error from the Elasticsearch API unless you set log_level to debug. However, this results in a lot of log spam, which isn't desirable if all you want is the 400 error reasons. You can set this true to capture the 400 error reasons without all the other debug logs. (default: false)
	LogEs400Reason bool `json:"log_es_400_reason,omitempty"`
	// By default, record body is wrapped by 'doc'. This behavior can not handle update script requests. You can set this to suppress doc wrapping and allow record body to be untouched. (default: false)
	SuppressDocWrap bool `json:"suppress_doc_wrap,omitempty"`
	// A list of exception that will be ignored - when the exception occurs the chunk will be discarded and the buffer retry mechanism won't be called. It is possible also to specify classes at higher level in the hierarchy. For example
	// `ignore_exceptions ["Elasticsearch::Transport::Transport::ServerError"]`
	// will match all subclasses of ServerError - Elasticsearch::Transport::Transport::Errors::BadRequest, Elasticsearch::Transport::Transport::Errors::ServiceUnavailable, etc.
	IgnoreExceptions string `json:"ignore_exceptions,omitempty"`
	// Indicates whether to backup chunk when ignore exception occurs. (default: true)
	// +kubebuilder:validation:Optional
	ExceptionBackup *bool `json:"exception_backup,omitempty" plugin:"default:true"`
	// Configure bulk_message request splitting threshold size.
	// Default value is 20MB. (20 * 1024 * 1024)
	// If you specify this size as negative number, bulk_message request splitting feature will be disabled. (default: 20MB)
	BulkMessageRequestThreshold string `json:"bulk_message_request_threshold,omitempty"`
	// The default Sniffer used by the Elasticsearch::Transport class works well when Fluentd has a direct connection to all of the Elasticsearch servers and can make effective use of the _nodes API. This doesn't work well when Fluentd must connect through a load balancer or proxy. The parameter sniffer_class_name gives you the ability to provide your own Sniffer class to implement whatever connection reload logic you require. In addition, there is a new Fluent::Plugin::ElasticsearchSimpleSniffer class which reuses the hosts given in the configuration, which is typically the hostname of the load balancer or proxy. https://github.com/uken/fluent-plugin-elasticsearch#sniffer-class-name
	SnifferClassName string `json:"sniffer_class_name,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
	// Enable Index Lifecycle Management (ILM).
	EnableIlm bool `json:"enable_ilm,omitempty"`
	// Specify ILM policy id.
	IlmPolicyID string `json:"ilm_policy_id,omitempty"`
	// Specify ILM policy contents as Hash.
	IlmPolicy string `json:"ilm_policy,omitempty"`
	// Specify whether overwriting ilm policy or not.
	IlmPolicyOverwrite bool `json:"ilm_policy_overwrite,omitempty"`
	// Use @type elasticsearch_data_stream
	DataStreamEnable *bool `json:"data_stream_enable,omitempty" plugin:"hidden"`
	// You can specify Elasticsearch data stream name by this parameter. This parameter is mandatory for elasticsearch_data_stream. There are some limitations about naming rule. For more details https://www.elastic.co/guide/en/elasticsearch/reference/master/indices-create-data-stream.html#indices-create-data-stream-api-path-params
	DataStreamName string `json:"data_stream_name,omitempty"`
	// Specify an existing index template for the data stream. If not present, a new template is created and named after the data stream. (default: data_stream_name) Further details here https://github.com/uken/fluent-plugin-elasticsearch#configuration---elasticsearch-output-data-stream
	DataStreamTemplateName string `json:"data_stream_template_name,omitempty"`
	// Specify an existing ILM policy to be applied to the data stream. If not present, either the specified template's or a new ILM default policy is applied. (default: data_stream_name) Further details here https://github.com/uken/fluent-plugin-elasticsearch#configuration---elasticsearch-output-data-stream
	DataStreamILMName string `json:"data_stream_ilm_name,omitempty"`
	// Specify data stream ILM policy contents as Hash.
	DataStreamIlmPolicy string `json:"data_stream_ilm_policy,omitempty"`
	// Specify whether overwriting data stream ilm policy or not.
	DataStreamIlmPolicyOverwrite bool `json:"data_stream_ilm_policy_overwrite,omitempty"`
}

func (e *ElasticsearchOutput) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	pluginType := "elasticsearch"
	if e.DataStreamEnable != nil && *e.DataStreamEnable {
		pluginType = "elasticsearch_data_stream"
	}
	elasticsearch := &types.OutputPlugin{
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
		elasticsearch.Params = params
	}
	if e.Buffer == nil {
		e.Buffer = &Buffer{}
	}
	if buffer, err := e.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		elasticsearch.SubDirectives = append(elasticsearch.SubDirectives, buffer)
	}
	return elasticsearch, nil
}
