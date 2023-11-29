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

// +name:"Kafka"
// +weight:"200"
type _hugoKafka interface{} //nolint:deadcode,unused

// +docName:"Kafka output plugin for Fluentd"
//
//	More info at https://github.com/fluent/fluent-plugin-kafka
//
// >Example Deployment: [Transport Nginx Access Logs into Kafka with Logging Operator](../../../../quickstarts/kafka-nginx/)
//
// ## Example output configurations
// ```yaml
// spec:
//
//	kafka:
//	  brokers: kafka-headless.kafka.svc.cluster.local:29092
//	  default_topic: topic
//	  sasl_over_ssl: false
//	  format:
//	    type: json
//	  buffer:
//	    tags: topic
//	    timekey: 1m
//	    timekey_wait: 30s
//	    timekey_use_utc: true
//
// ```
type _docKafka interface{} //nolint:deadcode,unused

// +name:"Kafka"
// +url:"https://github.com/fluent/fluent-plugin-kafka/releases/tag/v0.17.5"
// +version:"0.17.5"
// +description:"Send your logs to Kafka"
// +status:"GA"
type _metaKafka interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Kafka"
// Send your logs to Kafka
type KafkaOutputConfig struct {

	// The list of all seed brokers, with their host and port information.
	Brokers string `json:"brokers"`
	// Topic Key (default: "topic")
	TopicKey string `json:"topic_key,omitempty"`
	// Partition (default: "partition")
	PartitionKey string `json:"partition_key,omitempty"`
	// Partition Key (default: "partition_key")
	PartitionKeyKey string `json:"partition_key_key,omitempty"`
	// Message Key (default: "message_key")
	MessageKeyKey string `json:"message_key_key,omitempty"`
	// Client ID (default: "kafka")
	ClientId string `json:"client_id,omitempty"`
	// The name of default topic (default: nil).
	DefaultTopic string `json:"default_topic,omitempty"`
	// The name of default partition key (default: nil).
	DefaultPartitionKey string `json:"default_partition_key,omitempty"`
	// The name of default message key (default: nil).
	DefaultMessageKey string `json:"default_message_key,omitempty"`
	// Exclude Topic key (default: false)
	ExcludeTopicKey bool `json:"exclude_topic_key,omitempty"`
	// Exclude Partition key (default: false)
	ExcludePartitionKey bool `json:"exclude_partion_key,omitempty"`
	// Get Kafka Client log (default: false)
	GetKafkaClientLog bool `json:"get_kafka_client_log,omitempty"`
	// Headers (default: {})
	Headers map[string]string `json:"headers,omitempty"`
	// Headers from Record (default: {})
	HeadersFromRecord map[string]string `json:"headers_from_record,omitempty"`
	// Use default for unknown topics (default: false)
	UseDefaultForUnknownTopic bool `json:"use_default_for_unknown_topic,omitempty"`
	// Idempotent (default: false)
	Idempotent bool `json:"idempotent,omitempty"`
	// SASL over SSL (default: true)
	// +kubebuilder:validation:Optional
	SaslOverSSL bool           `json:"sasl_over_ssl"`
	Principal   string         `json:"principal,omitempty"`
	Keytab      *secret.Secret `json:"keytab,omitempty"`
	// Username when using PLAIN/SCRAM SASL authentication
	Username *secret.Secret `json:"username,omitempty"`
	// Password when using PLAIN/SCRAM SASL authentication
	Password *secret.Secret `json:"password,omitempty"`
	// If set, use SCRAM authentication with specified mechanism. When unset, default to PLAIN authentication
	ScramMechanism string `json:"scram_mechanism,omitempty"`
	// Number of times to retry sending of messages to a leader (default: 1)
	MaxSendRetries int `json:"max_send_retries,omitempty"`
	// The number of acks required per request (default: -1).
	RequiredAcks int `json:"required_acks,omitempty"`
	// How long the producer waits for acks. The unit is seconds (default: nil => Uses default of ruby-kafka library)
	AckTimeout int `json:"ack_timeout,omitempty"`
	// The codec the producer uses to compress messages (default: nil). The available options are gzip and snappy.
	CompressionCodec string `json:"compression_codec,omitempty"`
	// Maximum value of total message size to be included in one batch transmission. (default: 4096).
	KafkaAggMaxBytes int `json:"kafka_agg_max_bytes,omitempty"`
	// Maximum number of messages to include in one batch transmission. (default: nil).
	KafkaAggMaxMessages int `json:"kafka_agg_max_messages,omitempty"`
	// Discard the record where Kafka DeliveryFailed occurred (default: false)
	DiscardKafkaDeliveryFailed bool `json:"discard_kafka_delivery_failed,omitempty"`
	// System's CA cert store (default: false)
	SSLCACertsFromSystem *bool `json:"ssl_ca_certs_from_system,omitempty"`
	// CA certificate
	SSLCACert *secret.Secret `json:"ssl_ca_cert,omitempty"`
	// Client certificate
	SSLClientCert *secret.Secret `json:"ssl_client_cert,omitempty"`
	// Client certificate chain
	SSLClientCertChain *secret.Secret `json:"ssl_client_cert_chain,omitempty"`
	// Client certificate key
	SSLClientCertKey *secret.Secret `json:"ssl_client_cert_key,omitempty"`
	// Verify certificate hostname
	SSLVerifyHostname *bool `json:"ssl_verify_hostname,omitempty"`
	// +docLink:"Format,../format/"
	Format *Format `json:"format"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

func (e *KafkaOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "kafka2"
	kafka := &types.OutputPlugin{
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
		kafka.Params = params
	}
	if e.Buffer == nil {
		e.Buffer = &Buffer{}
	}
	if buffer, err := e.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		kafka.SubDirectives = append(kafka.SubDirectives, buffer)
	}

	if e.Format != nil {
		if format, err := e.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			kafka.SubDirectives = append(kafka.SubDirectives, format)
		}
	}
	return kafka, nil
}
