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
	"encoding/json"

	"github.com/cisco-open/operator-tools/pkg/secret"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Kafka"
// +weight:"200"
type _hugoKafka interface{} //nolint:deadcode,unused

// +docName:"Kafka output plugin for Fluentd"
//
/*
For details, see [https://github.com/fluent/fluent-plugin-kafka](https://github.com/fluent/fluent-plugin-kafka).

For an example deployment, see [Transport Nginx Access Logs into Kafka with Logging operator](../../../../quickstarts/kafka-nginx/).

## Example output configurations

```yaml
spec:
  kafka:
    brokers: kafka-headless.kafka.svc.cluster.local:29092
    default_topic: topic
    sasl_over_ssl: false
    format:
      type: json
    buffer:
      tags: topic
      timekey: 1m
      timekey_wait: 30s
      timekey_use_utc: true
```
*/
type _docKafka interface{} //nolint:deadcode,unused

// +name:"Kafka"
// +url:"https://github.com/fluent/fluent-plugin-kafka/releases/tag/v0.17.5"
// +version:"0.17.5"
// +description:"Send your logs to Kafka"
// +status:"GA"
type _metaKafka interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Kafka"
// Send your logs to Kafka.
// Setting use_rdkafka to true opts for rdkafka2, which offers higher performance compared to ruby-kafka.
// (Note: requires fluentd image version v1.16-4.9-full or higher)
// -[more info](https://github.com/fluent/fluent-plugin-kafka#output-plugin)
type KafkaOutputConfig struct {
	// Use rdkafka2 instead of the legacy kafka2 output plugin. This plugin requires fluentd image version v1.16-4.9-full or higher.
	UseRdkafka bool `json:"use_rdkafka,omitempty"`
	// RdkafkaOptions represents the global configuration properties for librdkafka.
	RdkafkaOptions *RdkafkaOptions `json:"rdkafka_options,omitempty"`
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
	SaslOverSSL *bool          `json:"sasl_over_ssl,omitempty"`
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
	// Max byte size to send message to avoid MessageSizeTooLarge. Messages over the limit will be dropped (default: no limit)
	MaxSendLimitBytes int `json:"max_send_limit_bytes,omitempty"`
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
	// If chunk flush takes longer time than this threshold, Fluentd logs a warning message and increases the  `fluentd_output_status_slow_flush_count` metric.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

// RdkafkaOptions represents the global configuration properties for librdkafka.
type RdkafkaOptions struct {
	// Indicates the builtin features for this build of librdkafka. An application can either query this value or attempt to set it with its list of required features to check for library support.
	BuiltinFeatures string `json:"builtin.features,omitempty"`
	// Client identifier.
	ClientID string `json:"client.id,omitempty"`
	// Initial list of brokers as a CSV list of broker host or host:port. The application may also use `rd_kafka_brokers_add()` to add brokers during runtime.
	MetadataBrokerList string `json:"metadata.broker.list,omitempty"`
	// Alias for `metadata.broker.list`: Initial list of brokers as a CSV list of broker host or host:port. The application may also use `rd_kafka_brokers_add()` to add brokers during runtime.
	BootstrapServers string `json:"bootstrap.servers,omitempty"`
	// Maximum Kafka protocol request message size. Due to differing framing overhead between protocol versions the producer is unable to reliably enforce a strict max message limit at produce time and may exceed the maximum size by one message in protocol ProduceRequests, the broker will enforce the the topic's `max.message.bytes` limit (see Apache Kafka documentation).
	MessageMaxBytes int `json:"message.max.bytes,omitempty"`
	// Maximum size for message to be copied to buffer. Messages larger than this will be passed by reference (zero-copy) at the expense of larger iovecs.
	MessageCopyMaxBytes int `json:"message.copy.max.bytes,omitempty"`
	// Maximum Kafka protocol response message size. This serves as a safety precaution to avoid memory exhaustion in case of protocol hickups. This value must be at least `fetch.max.bytes`  + 512 to allow for protocol overhead; the value is adjusted automatically unless the configuration property is explicitly set.
	ReceiveMessageMaxBytes int `json:"receive.message.max.bytes,omitempty"`
	// Maximum number of in-flight requests per broker connection. This is a generic property applied to all broker communication, however it is primarily relevant to produce requests. In particular, note that other mechanisms limit the number of outstanding consumer fetch request per broker to one.
	MaxInFlightRequestsPerConnection int `json:"max.in.flight.requests.per.connection,omitempty"`
	// Alias for `max.in.flight.requests.per.connection`: Maximum number of in-flight requests per broker connection. This is a generic property applied to all broker communication, however it is primarily relevant to produce requests. In particular, note that other mechanisms limit the number of outstanding consumer fetch request per broker to one.
	MaxInFlight int `json:"max.in.flight,omitempty"`
	// Period of time in milliseconds at which topic and broker metadata is refreshed in order to proactively discover any new brokers, topics, partitions or partition leader changes. Use -1 to disable the intervalled refresh (not recommended). If there are no locally referenced topics (no topic objects created, no messages produced, no subscription or no assignment) then only the broker list will be refreshed every interval but no more often than every 10s.
	TopicMetadataRefreshIntervalMs int `json:"topic.metadata.refresh.interval.ms,omitempty"`
	// Metadata cache max age. Defaults to topic.metadata.refresh.interval.ms * 3
	MetadataMaxAgeMs int `json:"metadata.max.age.ms,omitempty"`
	// When a topic loses its leader a new metadata request will be enqueued immediately and then with this initial interval, exponentially increasing upto `retry.backoff.max.ms`, until the topic metadata has been refreshed. If not set explicitly, it will be defaulted to `retry.backoff.ms`. This is used to recover quickly from transitioning leader brokers.
	TopicMetadataRefreshFastIntervalMs int `json:"topic.metadata.refresh.fast.interval.ms,omitempty"`
	// Sparse metadata requests (consumes less network bandwidth)
	TopicMetadataRefreshSparse bool `json:"topic.metadata.refresh.sparse,omitempty"`
	// Apache Kafka topic creation is asynchronous and it takes some time for a new topic to propagate throughout the cluster to all brokers. If a client requests topic metadata after manual topic creation but before the topic has been fully propagated to the broker the client is requesting metadata from, the topic will seem to be non-existent and the client will mark the topic as such, failing queued produced messages with `ERR__UNKNOWN_TOPIC`. This setting delays marking a topic as non-existent until the configured propagation max time has passed. The maximum propagation time is calculated from the time the topic is first referenced in the client, e.g., on produce().
	TopicMetadataPropagationMaxMs int `json:"topic.metadata.propagation.max.ms,omitempty"`
	// Topic blacklist, a comma-separated list of regular expressions for matching topic names that should be ignored in broker metadata information as if the topics did not exist.
	TopicBlacklist string `json:"topic.blacklist,omitempty"`
	// A comma-separated list of debug contexts to enable. Detailed Producer debugging: broker,topic,msg. Consumer: consumer,cgrp,topic,fetch
	Debug string `json:"debug,omitempty"`
	// Default timeout for network requests. Producer: ProduceRequests will use the lesser value of `socket.timeout.ms` and remaining `message.timeout.ms` for the first message in the batch. Consumer: FetchRequests will use `fetch.wait.max.ms` + `socket.timeout.ms`. Admin: Admin requests will use `socket.timeout.ms` or explicitly set `rd_kafka_AdminOptions_set_operation_timeout()` value.
	SocketTimeoutMs int `json:"socket.timeout.ms,omitempty"`
	// DEPRECATED No longer used.
	SocketBlockingMaxMs int `json:"socket.blocking.max.ms,omitempty"`
	// Broker socket send buffer size. System default is used if 0.
	SocketSendBufferBytes int `json:"socket.send.buffer.bytes,omitempty"`
	// Broker socket receive buffer size. System default is used if 0.
	SocketReceiveBufferBytes int `json:"socket.receive.buffer.bytes,omitempty"`
	// Enable TCP keep-alives (SO_KEEPALIVE) on broker sockets
	SocketKeepaliveEnable bool `json:"socket.keepalive.enable,omitempty"`
	// Disable the Nagle algorithm (TCP_NODELAY) on broker sockets.
	SocketNagleDisable bool `json:"socket.nagle.disable,omitempty"`
	// Disconnect from broker when this number of send failures (e.g., timed out requests) is reached. Disable with 0. WARNING: It is highly recommended to leave this setting at its default value of 1 to avoid the client and broker to become desynchronized in case of request timeouts. NOTE: The connection is automatically re-established.
	SocketMaxFails int `json:"socket.max.fails,omitempty"`
	// How long to cache the broker address resolving results (milliseconds).
	BrokerAddressTTl int `json:"broker.address.ttl,omitempty"`
	// Allowed broker IP address families: any, v4, v6
	BrokerAddressFamily string `json:"broker.address.family,omitempty"`
	// Maximum time allowed for broker connection setup (TCP connection setup as well SSL and SASL handshake). If the connection to the broker is not fully functional after this the connection will be closed and retried.
	SocketConnectionSetupTimeoutMs int `json:"socket.connection.setup.timeout.ms,omitempty"`
	// Close broker connections after the specified time of inactivity. Disable with 0. If this property is left at its default value some heuristics are performed to determine a suitable default value, this is currently limited to identifying brokers on Azure (see librdkafka issue #3109 for more info).
	ConnectionsMaxIdleMs int `json:"connections.max.idle.ms,omitempty"`
	// The initial time to wait before reconnecting to a broker after the connection has been closed. The time is increased exponentially until `reconnect.backoff.max.ms` is reached. -25% to +50% jitter is applied to each reconnect backoff. A value of 0 disables the backoff and reconnects immediately.
	ReconnectBackoffMs int `json:"reconnect.backoff.ms,omitempty"`
	// The maximum time to wait before reconnecting to a broker after the connection has been closed.
	ReconnectBackoffMaxMs int `json:"reconnect.backoff.max.ms,omitempty"`
	// librdkafka statistics emit interval. The application also needs to register a stats callback using `rd_kafka_conf_set_stats_cb()`. The granularity is 1000ms. A value of 0 disables statistics.
	StatisticsIntervalMs int `json:"statistics.interval.ms,omitempty"`
	// See `rd_kafka_conf_set_events()`
	EnabledEvents int `json:"enabled_events,omitempty"`
	// Error callback (set with rd_kafka_conf_set_error_cb())
	ErrorCb string `json:"error_cb,omitempty"`
	// Throttle callback (set with rd_kafka_conf_set_throttle_cb())
	ThrottleCb string `json:"throttle_cb,omitempty"`
	// Statistics callback (set with rd_kafka_conf_set_stats_cb())
	StatsCb string `json:"stats_cb,omitempty"`
	// Log callback (set with rd_kafka_conf_set_log_cb())
	LogCb string `json:"log_cb,omitempty"`
	// Logging level (syslog(3) levels)
	LogLevel int `json:"log_level,omitempty"`
	// Disable spontaneous log_cb from internal librdkafka threads, instead enqueue log messages on queue set with `rd_kafka_set_log_queue()` and serve log callbacks or events through the standard poll APIs. **NOTE**: Log messages will linger in a temporary queue until the log queue has been set.
	LogQueue bool `json:"log.queue,omitempty"`
	// Print internal thread name in log messages (useful for debugging librdkafka internals)
	LogThreadName bool `json:"log.thread.name,omitempty"`
	// If enabled librdkafka will initialize the PRNG with srand(current_time.milliseconds) on the first invocation of rd_kafka_new() (required only if rand_r() is not available on your platform). If disabled the application must call srand() prior to calling rd_kafka_new().
	EnableRandomSeed bool `json:"enable.random.seed,omitempty"`
	// Log broker disconnects. It might be useful to turn this off when interacting with 0.9 brokers with an aggressive `connections.max.idle.ms` value.
	LogConnectionClose bool `json:"log.connection.close,omitempty"`
	// Background queue event callback (set with rd_kafka_conf_set_background_event_cb())
	BackgroundEventCb string `json:"background_event_cb,omitempty"`
	// Socket creation callback to provide race-free CLOEXEC
	SocketCb string `json:"socket_cb,omitempty"`
	// Socket connect callback
	ConnectCb string `json:"connect_cb,omitempty"`
	// Socket close callback
	CloseSocketCb string `json:"closesocket_cb,omitempty"`
	// File open callback to provide race-free CLOEXEC
	OpenCb string `json:"open_cb,omitempty"`
	// Address resolution callback (set with rd_kafka_conf_set_resolve_cb())
	ResolveCb string `json:"resolve_cb,omitempty"`
	// Application opaque (set with rd_kafka_conf_set_opaque())
	Opaque string `json:"opaque,omitempty"`
	// Default topic configuration for automatically subscribed topics
	DefaultTopicConf string `json:"default_topic_conf,omitempty"`
	// Signal that librdkafka will use to quickly terminate on rd_kafka_destroy(). If this signal is not set then there will be a delay before rd_kafka_wait_destroyed() returns true as internal threads are timing out their system calls. If this signal is set however the delay will be minimal. The application should mask this signal as an internal signal handler is installed.
	InternalTerminationSignal int `json:"internal.termination.signal,omitempty"`
	// Request broker's supported API versions to adjust functionality to available protocol features. If set to false, or the ApiVersionRequest fails, the fallback version `broker.version.fallback` will be used. **NOTE**: Depends on broker version >=0.10.0. If the request is not supported by (an older) broker the `broker.version.fallback` fallback is used.
	ApiVersionRequest bool `json:"api.version.request,omitempty"`
	// Timeout for broker API version requests.
	ApiVersionRequestTimeoutMs int `json:"api.version.request.timeout.ms,omitempty"`
	// Dictates how long the `broker.version.fallback` fallback is used in the case the ApiVersionRequest fails.
	ApiVersionFallbackMs int `json:"api.version.fallback.ms,omitempty"`
	// Older broker versions (before 0.10.0) provide no way for a client to query for supported protocol features (ApiVersionRequest, see `api.version.request`) making it impossible for the client to know what features it may use. As a workaround a user may set this property to the expected broker version and the client will automatically adjust its feature set accordingly if the ApiVersionRequest fails (or is disabled). The fallback broker version will be used for `api.version.fallback.ms`. Valid values are: 0.9.0, 0.8.2, 0.8.1, 0.8.0. Any other value >= 0.10, such as 0.10.2.1, enables ApiVersionRequests.
	BrokerVersionFallback string `json:"broker.version.fallback,omitempty"`
	// Allow automatic topic creation on the broker when subscribing to or assigning non-existent topics. The broker must also be configured with `auto.create.topics.enable=true` for this configuration to take effect. Note: the default value (true) for the producer is different from the default value (false) for the consumer. Further, the consumer default value is different from the Java consumer (true), and this property is not supported by the Java producer. Requires broker version >= 0.11.0.0, for older broker versions only the broker configuration applies.
	AllowAutoCreateTopics bool `json:"allow.auto.create.topics,omitempty"`
	// Protocol used to communicate with brokers.
	SecurityProtocol string `json:"security.protocol,omitempty"`
	// A cipher suite is a named combination of authentication, encryption, MAC and key exchange algorithm used to negotiate the security settings for a network connection using TLS or SSL network protocol. See manual page for `ciphers(1)` and `SSL_CTX_set_cipher_list(3).
	SSLCipherSuites string `json:"ssl.cipher.suites,omitempty"`
	// The supported-curves extension in the TLS ClientHello message specifies the curves (standard/named, or 'explicit' GF(2^k) or GF(p)) the client is willing to have the server use. See manual page for `SSL_CTX_set1_curves_list(3)`. OpenSSL >= 1.0.2 required.
	SSLCurvesList string `json:"ssl.curves.list,omitempty"`
	// The client uses the TLS ClientHello signature_algorithms extension to indicate to the server which signature/hash algorithm pairs may be used in digital signatures. See manual page for `SSL_CTX_set1_sigalgs_list(3)`. OpenSSL >= 1.0.2 required.
	SSLSigalgsList string `json:"ssl.sigalgs.list,omitempty"`
	// Path to client's private key (PEM) used for authentication.
	SSLKeyLocation string `json:"ssl.key.location,omitempty"`
	// Private key passphrase (for use with `ssl.key.location` and `set_ssl_cert()`).
	SSLKeyPassword string `json:"ssl.key.password,omitempty"`
	// Client's private key string (PEM format) used for authentication.
	SSLKeyPem string `json:"ssl.key.pem,omitempty"`
	// Path to client's public key (PEM) used for authentication.
	SSLCertificateLocation string `json:"ssl.certificate.location,omitempty"`
	// Client's public key string (PEM format) used for authentication.
	SSLCertificatePem string `json:"ssl.certificate.pem,omitempty"`
	// File or directory path to CA certificate(s) for verifying the broker's key. Defaults: On Windows the system's CA certificates are automatically looked up in the Windows Root certificate store. On Mac OSX this configuration defaults to `probe`. It is recommended to install openssl using Homebrew, to provide CA certificates. On Linux install the distribution's ca-certificates package. If OpenSSL is statically linked or `ssl.ca.location` is set to `probe` a list of standard paths will be probed and the first one found will be used as the default CA certificate location path. If OpenSSL is dynamically linked the OpenSSL library's default path will be used (see `OPENSSLDIR` in `openssl version -a`).
	SSLCaLocation string `json:"ssl.ca.location,omitempty"`
	// CA certificate string (PEM format) for verifying the broker's key.
	SSLCaPem string `json:"ssl.ca.pem,omitempty"`
	// Path to CRL for verifying broker's certificate validity.
	SSLCrlLocation string `json:"ssl.crl.location,omitempty"`
	// Path to client's keystore (PKCS#12) used for authentication.
	SSLKeystoreLocation string `json:"ssl.keystore.location,omitempty"`
	// Client's keystore (PKCS#12) password.
	SSLKeystorePassword string `json:"ssl.keystore.password,omitempty"`
	// Comma-separated list of OpenSSL 3.0.x implementation providers. E.g., "default,legacy".
	SSLProviders string `json:"ssl.providers,omitempty"`
	// **DEPRECATED** Path to OpenSSL engine library. OpenSSL >= 1.1.x required. DEPRECATED: OpenSSL engine support is deprecated and should be replaced by OpenSSL 3 providers.
	SSLEngineLocation string `json:"ssl.engine.location,omitempty"`
	// OpenSSL engine id is the name used for loading engine.
	SSLEngineId string `json:"ssl.engine.id,omitempty"`
	// Enable OpenSSL's builtin broker (server) certificate verification. This verification can be extended by the application by implementing a certificate_verify_cb.
	EnableSSLCertificateVerification bool `json:"enable.ssl.certificate.verification,omitempty"`
	// Endpoint identification algorithm to validate broker hostname using broker certificate. https - Server (broker) hostname verification as specified in RFC2818. none - No endpoint verification. OpenSSL >= 1.0.2 required.
	SSLEndpointIdentificationAlgorithm string `json:"ssl.endpoint.identification.algorithm,omitempty"`
	// SASL mechanism to use for authentication. Supported: GSSAPI, PLAIN, SCRAM-SHA-256, SCRAM-SHA-512, OAUTHBEARER. NOTE: Despite the name only one mechanism must be configured.
	SaslMechanisms string `json:"sasl.mechanisms,omitempty" `
	// Kerberos principal name that Kafka runs as, not including /hostname@REALM.
	SaslKerberosServiceName string `json:"sasl.kerberos.service.name,omitempty" `
	// This client's Kerberos principal name. (Not supported on Windows, will use the logon user's principal).
	SaslKerberosPrincipal string `json:"sasl.kerberos.principal,omitempty" `
	// Shell command to refresh or acquire the client's Kerberos ticket. This command is executed on client creation and every sasl.kerberos.min.time.before.relogin (0=disable).
	SaslKerberosKinitCmd string `json:"sasl.kerberos.kinit.cmd,omitempty" `
	// Path to Kerberos keytab file. This configuration property is only used as a variable in sasl.kerberos.kinit.cmd as  ... -t "%{sasl.kerberos.keytab}".
	SaslKerberosKeytab string `json:"sasl.kerberos.keytab,omitempty" `
	// Minimum time in milliseconds between key refresh attempts. Disable automatic key refresh by setting this property to 0.
	SaslKerberosMinTimeBeforeRelogin int `json:"sasl.kerberos.min.time.before.relogin,omitempty" `
	// SASL username for use with the PLAIN and SASL-SCRAM-.. mechanisms.
	SaslUsername string `json:"sasl.username,omitempty" `
	// SASL password for use with the PLAIN and SASL-SCRAM-.. mechanism.
	SaslPassword string `json:"sasl.password,omitempty" `
	// SASL/OAUTHBEARER configuration. The format is implementation-dependent and must be parsed accordingly. The default unsecured token implementation (see https://tools.ietf.org/html/rfc7515#appendix-A.5) recognizes space-separated name=value pairs with valid names including principalClaimName, principal, scopeClaimName, scope, and lifeSeconds. The default value for principalClaimName is "sub", the default value for scopeClaimName is "scope", and the default value for lifeSeconds is 3600. The scope value is CSV format with the default value being no/empty scope. For example: principalClaimName=azp principal=admin scopeClaimName=roles scope=role1,role2 lifeSeconds=600. In addition, SASL extensions can be communicated to the broker via extension_NAME=value. For example: principal=admin extension_traceId=123.
	SaslOauthbearerConfig string `json:"sasl.oauthbearer.config,omitempty" `
	// Enable the builtin unsecure JWT OAUTHBEARER token handler if no oauthbearer_refresh_cb has been set. This builtin handler should only be used for development or testing, and not in production.
	EnableSaslOauthbearerUnsecureJwt bool `json:"enable.sasl.oauthbearer.unsecure.jwt,omitempty" `
	// SASL/OAUTHBEARER token refresh callback (set with rd_kafka_conf_set_oauthbearer_token_refresh_cb(), triggered by rd_kafka_poll(), et.al. This callback will be triggered when it is time to refresh the client's OAUTHBEARER token. Also see rd_kafka_conf_enable_sasl_queue().
	OauthbearerTokenRefreshCb string `json:"oauthbearer_token_refresh_cb,omitempty" `
	// Set to "default" or "oidc" to control which login method to be used. If set to "oidc", the following properties must also be specified: sasl.oauthbearer.client.id, sasl.oauthbearer.client.secret, and sasl.oauthbearer.token.endpoint.url.
	SaslOauthbearerMethod string `json:"sasl.oauthbearer.method,omitempty" `
	// Public identifier for the application. Must be unique across all clients that the authorization server handles. Only used when sasl.oauthbearer.method is set to "oidc".
	SaslOauthbearerClientId string `json:"sasl.oauthbearer.client.id,omitempty" `
	// Client secret only known to the application and the authorization server. This should be a sufficiently random string that is not guessable. Only used when sasl.oauthbearer.method is set to "oidc".
	SaslOauthbearerClientSecret string `json:"sasl.oauthbearer.client.secret,omitempty" `
	// Client use this to specify the scope of the access request to the broker. Only used when sasl.oauthbearer.method is set to "oidc".
	SaslOauthbearerScope string `json:"sasl.oauthbearer.scope,omitempty" `
	// Allow additional information to be provided to the broker. Comma-separated list of key=value pairs. E.g., "supportFeatureX=true,organizationId=sales-emea".Only used when sasl.oauthbearer.method is set to "oidc".
	SaslOauthbearerExtensions string `json:"sasl.oauthbearer.extensions,omitempty" `
	// OAuth/OIDC issuer token endpoint HTTP(S) URI used to retrieve token. Only used when sasl.oauthbearer.method is set to "oidc".
	SaslOauthbearerTokenEndpointUrl string `json:"sasl.oauthbearer.token.endpoint.url,omitempty" `
	// List of plugin libraries to load (; separated). The library search path is platform dependent (see dlopen(3) for Unix and LoadLibrary() for Windows). If no filename extension is specified the platform-specific extension (such as .dll or .so) will be appended automatically.
	PluginLibraryPaths string `json:"plugin.library.paths,omitempty" `
	// Interceptors added through rd_kafka_conf_interceptor_add_..() and any configuration handled by interceptors.
	Interceptors string `json:"interceptors,omitempty"`
}

func (e *KafkaOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	pluginType := "kafka2"
	if e.UseRdkafka {
		pluginType = "rdkafka2"
	}
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

	if e.RdkafkaOptions != nil {
		if rdkafkaOptions, err := types.NewStructToStringMapper(secretLoader).StringsMap(e.RdkafkaOptions); err != nil {
			return nil, err
		} else {
			if len(rdkafkaOptions) > 0 {
				marshaledRdkafkaOptions, err := json.Marshal(rdkafkaOptions)
				if err != nil {
					return nil, err
				}
				kafka.Params["rdkafka_options"] = string(marshaledRdkafkaOptions)
			}
		}
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

	// remove use_rdkafka from params, it is not a valid parameter for plugin config
	delete(kafka.Params, "use_rdkafka")
	return kafka, nil
}
