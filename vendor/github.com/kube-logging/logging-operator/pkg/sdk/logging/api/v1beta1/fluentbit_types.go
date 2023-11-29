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

package v1beta1

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/cisco-open/operator-tools/pkg/typeoverride"
	"github.com/cisco-open/operator-tools/pkg/volume"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// +name:"FluentbitSpec"
// +weight:"200"
type _hugoFluentbitSpec interface{} //nolint:deadcode,unused

// +name:"FluentbitSpec"
// +version:"v1beta1"
// +description:"FluentbitSpec defines the desired state of Fluentbit"
type _metaFluentbitSpec interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true

// FluentbitSpec defines the desired state of Fluentbit
type FluentbitSpec struct {
	DaemonSetAnnotations map[string]string `json:"daemonsetAnnotations,omitempty"`
	Annotations          map[string]string `json:"annotations,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
	EnvVars              []corev1.EnvVar   `json:"envVars,omitempty"`
	Image                ImageSpec         `json:"image,omitempty"`
	TLS                  *FluentbitTLS     `json:"tls,omitempty"`
	TargetHost           string            `json:"targetHost,omitempty"`
	TargetPort           int32             `json:"targetPort,omitempty"`
	// Set the flush time in seconds.nanoseconds. The engine loop uses a Flush timeout to define when is required to flush the records ingested by input plugins through the defined output plugins. (default: 1)
	Flush int32 `json:"flush,omitempty"  plugin:"default:1"`
	// Set the grace time in seconds as Integer value. The engine loop uses a Grace timeout to define wait time on exit (default: 5)
	Grace int32 `json:"grace,omitempty" plugin:"default:5"`
	// Set the logging verbosity level. Allowed values are: error, warn, info, debug and trace. Values are accumulative, e.g: if 'debug' is set, it will include error, warning, info and debug.  Note that trace mode is only available if Fluent Bit was built with the WITH_TRACE option enabled. (default: info)
	LogLevel string `json:"logLevel,omitempty" plugin:"default:info"`
	// Set the coroutines stack size in bytes. The value must be greater than the page size of the running system. Don't set too small value (say 4096), or coroutine threads can overrun the stack buffer.
	//Do not change the default value of this parameter unless you know what you are doing. (default: 24576)
	CoroStackSize int32                       `json:"coroStackSize,omitempty" plugin:"default:24576"`
	Resources     corev1.ResourceRequirements `json:"resources,omitempty"`
	Tolerations   []corev1.Toleration         `json:"tolerations,omitempty"`
	NodeSelector  map[string]string           `json:"nodeSelector,omitempty"`
	Affinity      *corev1.Affinity            `json:"affinity,omitempty"`
	Metrics       *Metrics                    `json:"metrics,omitempty"`
	Security      *Security                   `json:"security,omitempty"`
	// +docLink:"volume.KubernetesVolume,https://github.com/cisco-open/operator-tools/tree/master/docs/types"
	PositionDB volume.KubernetesVolume `json:"positiondb,omitempty"`
	// Deprecated, use positiondb
	PosisionDBLegacy  *volume.KubernetesVolume `json:"position_db,omitempty"`
	MountPath         string                   `json:"mountPath,omitempty"`
	ExtraVolumeMounts []*VolumeMount           `json:"extraVolumeMounts,omitempty"`
	InputTail         InputTail                `json:"inputTail,omitempty"`
	FilterAws         *FilterAws               `json:"filterAws,omitempty"`
	FilterModify      []FilterModify           `json:"filterModify,omitempty"`
	// Deprecated, use inputTail.parser
	Parser string `json:"parser,omitempty"`
	// Parameters for Kubernetes metadata filter
	FilterKubernetes FilterKubernetes `json:"filterKubernetes,omitempty"`
	// Disable Kubernetes metadata filter
	DisableKubernetesFilter *bool         `json:"disableKubernetesFilter,omitempty"`
	BufferStorage           BufferStorage `json:"bufferStorage,omitempty"`
	// +docLink:"volume.KubernetesVolume,https://github.com/cisco-open/operator-tools/tree/master/docs/types"
	BufferStorageVolume     volume.KubernetesVolume        `json:"bufferStorageVolume,omitempty"`
	BufferVolumeMetrics     *Metrics                       `json:"bufferVolumeMetrics,omitempty"`
	BufferVolumeImage       ImageSpec                      `json:"bufferVolumeImage,omitempty"`
	BufferVolumeArgs        []string                       `json:"bufferVolumeArgs,omitempty"`
	CustomConfigSecret      string                         `json:"customConfigSecret,omitempty"`
	PodPriorityClassName    string                         `json:"podPriorityClassName,omitempty"`
	LivenessProbe           *corev1.Probe                  `json:"livenessProbe,omitempty"`
	LivenessDefaultCheck    bool                           `json:"livenessDefaultCheck,omitempty"`
	ReadinessProbe          *corev1.Probe                  `json:"readinessProbe,omitempty"`
	Network                 *FluentbitNetwork              `json:"network,omitempty"`
	ForwardOptions          *ForwardOptions                `json:"forwardOptions,omitempty"`
	EnableUpstream          bool                           `json:"enableUpstream,omitempty"`
	ServiceAccountOverrides *typeoverride.ServiceAccount   `json:"serviceAccount,omitempty"`
	DNSPolicy               corev1.DNSPolicy               `json:"dnsPolicy,omitempty"`
	DNSConfig               *corev1.PodDNSConfig           `json:"dnsConfig,omitempty"`
	HostNetwork             bool                           `json:"HostNetwork,omitempty"`
	SyslogNGOutput          *FluentbitTCPOutput            `json:"syslogng_output,omitempty"`
	UpdateStrategy          appsv1.DaemonSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

// +kubebuilder:object:generate=true

// FluentbitTLS defines the TLS configs
type FluentbitTLS struct {
	Enabled    *bool  `json:"enabled"`
	SecretName string `json:"secretName,omitempty"`
	SharedKey  string `json:"sharedKey,omitempty"`
}

// +kubebuilder:object:generate=true

// FluentbitTCPOutput defines the TLS configs
type FluentbitTCPOutput struct {
	JsonDateKey    string `json:"json_date_key,omitempty" plugin:"default:ts"`
	JsonDateFormat string `json:"json_date_format,omitempty" plugin:"default:iso8601"`
}

// FluentbitNetwork defines network configuration for fluentbit
type FluentbitNetwork struct {
	// Sets the timeout for connecting to an upstream (default: 10)
	ConnectTimeout *uint32 `json:"connectTimeout,omitempty"`
	// On connection timeout, specify if it should log an error. When disabled, the timeout is logged as a debug message (default: true)
	ConnectTimeoutLogError *bool `json:"connectTimeoutLogError,omitempty"`
	// Sets the primary transport layer protocol used by the asynchronous DNS resolver for connections established (default: UDP, UDP or TCP)
	DNSMode string `json:"dnsMode,omitempty"`
	// Prioritize IPv4 DNS results when trying to establish a connection (default: false)
	DNSPreferIPV4 *bool `json:"dnsPreferIpv4,omitempty"`
	// Select the primary DNS resolver type (default: ASYNC, LEGACY or ASYNC)
	DNSResolver string `json:"dnsResolver,omitempty"`
	// Whether or not TCP keepalive is used for the upstream connection (default: true)
	Keepalive *bool `json:"keepalive,omitempty"`
	// How long in seconds a TCP keepalive connection can be idle before being recycled (default: 30)
	KeepaliveIdleTimeout *uint32 `json:"keepaliveIdleTimeout,omitempty"`
	// How many times a TCP keepalive connection can be used before being recycled (default: 0, disabled)
	KeepaliveMaxRecycle *uint32 `json:"keepaliveMaxRecycle,omitempty"`
	// Specify network address (interface) to use for connection and data traffic. (default: disabled)
	SourceAddress string `json:"sourceAddress,omitempty"`
}

// GetPrometheusPortFromAnnotation gets the port value from annotation
func (spec FluentbitSpec) GetPrometheusPortFromAnnotation() int32 {
	var err error
	var port int64
	if spec.Annotations != nil {
		port, err = strconv.ParseInt(spec.Annotations["prometheus.io/port"], 10, 32)
		if err != nil {
			panic(err)
		}
	}
	return int32(port)
}

// BufferStorage is the Service Section Configuration of fluent-bit
type BufferStorage struct {
	// Set an optional location in the file system to store streams and chunks of data. If this parameter is not set, Input plugins can only use in-memory buffering.
	StoragePath string `json:"storage.path,omitempty"`
	// Configure the synchronization mode used to store the data into the file system. It can take the values normal or full. (default:normal)
	StorageSync string `json:"storage.sync,omitempty"`
	// Enable the data integrity check when writing and reading data from the filesystem. The storage layer uses the CRC32 algorithm. (default:Off)
	StorageChecksum string `json:"storage.checksum,omitempty"`
	// If storage.path is set, Fluent Bit will look for data chunks that were not delivered and are still in the storage layer, these are called backlog data. This option configure a hint of maximum value of memory to use when processing these records. (default:5M)
	StorageBacklogMemLimit string `json:"storage.backlog.mem_limit,omitempty"`
}

// InputTail defines Fluentbit tail input configuration The tail input plugin allows to monitor one or several text files. It has a similar behavior like tail -f shell command.
type InputTail struct {
	// Specify the buffering mechanism to use. It can be memory or filesystem. (default:memory)
	StorageType string `json:"storage.type,omitempty"`
	// Set the buffer size for HTTP client when reading responses from Kubernetes API server. The value must be according to the Unit Size specification. (default:32k)
	BufferChunkSize string `json:"Buffer_Chunk_Size,omitempty"`
	// Set the limit of the buffer size per monitored file. When a buffer needs to be increased (e.g: very long lines), this value is used to restrict how much the memory buffer can grow. If reading a file exceed this limit, the file is removed from the monitored file list. The value must be according to the Unit Size specification. (default:Buffer_Chunk_Size)
	BufferMaxSize string `json:"Buffer_Max_Size,omitempty"`
	// Pattern specifying a specific log files or multiple ones through the use of common wildcards.
	Path string `json:"Path,omitempty"`
	// If enabled, it appends the name of the monitored file as part of the record. The value assigned becomes the key in the map.
	PathKey string `json:"Path_Key,omitempty"`
	// Set one or multiple shell patterns separated by commas to exclude files matching a certain criteria, e.g: exclude_path=*.gz,*.zip
	ExcludePath string `json:"Exclude_Path,omitempty"`
	// For new discovered files on start (without a database offset/position), read the content from the head of the file, not tail.
	ReadFromHead bool `json:"Read_From_Head,omitempty"`
	// The interval of refreshing the list of watched files in seconds. (default:60)
	RefreshInterval string `json:"Refresh_Interval,omitempty"`
	// Specify the number of extra time in seconds to monitor a file once is rotated in case some pending data is flushed. (default:5)
	RotateWait string `json:"Rotate_Wait,omitempty"`
	// Ignores files that have been last modified before this time in seconds. Supports m,h,d (minutes, hours,days) syntax. Default behavior is to read all specified files.
	IgnoreOlder string `json:"Ignore_Older,omitempty"`
	// When a monitored file reach it buffer capacity due to a very long line (Buffer_Max_Size), the default behavior is to stop monitoring that file. Skip_Long_Lines alter that behavior and instruct Fluent Bit to skip long lines and continue processing other lines that fits into the buffer size. (default:Off)
	SkipLongLines string `json:"Skip_Long_Lines,omitempty"`
	// Specify the database file to keep track of monitored files and offsets.
	DB *string `json:"DB,omitempty"`
	// Set a default synchronization (I/O) method. Values: Extra, Full, Normal, Off. This flag affects how the internal SQLite engine do synchronization to disk, for more details about each option please refer to this section. (default:Full)
	DBSync string `json:"DB_Sync,omitempty"`
	// Specify that the database will be accessed only by Fluent Bit. Enabling this feature helps to increase performance when accessing the database but it restrict any external tool to query the content. (default: true)
	DBLocking *bool `json:"DB.locking,omitempty"`
	// sets the journal mode for databases (WAL). Enabling WAL provides higher performance. Note that WAL is not compatible with shared network file systems. (default: WAL)
	DBJournalMode string `json:"DB.journal_mode,omitempty"`
	// Set a limit of memory that Tail plugin can use when appending data to the Engine. If the limit is reach, it will be paused; when the data is flushed it resumes.
	MemBufLimit string `json:"Mem_Buf_Limit,omitempty"`
	// Specify the name of a parser to interpret the entry as a structured message.
	Parser string `json:"Parser,omitempty"`
	// When a message is unstructured (no parser applied), it's appended as a string under the key name log. This option allows to define an alternative name for that key. (default:log)
	Key string `json:"Key,omitempty"`
	// Set a tag (with regex-extract fields) that will be placed on lines read.
	Tag string `json:"Tag,omitempty"`
	// Set a regex to extract fields from the file.
	TagRegex string `json:"Tag_Regex,omitempty"`
	// If enabled, the plugin will try to discover multiline messages and use the proper parsers to compose the outgoing messages. Note that when this option is enabled the Parser option is not used. (default:Off)
	Multiline string `json:"Multiline,omitempty"`
	// Wait period time in seconds to process queued multiline messages (default:4)
	MultilineFlush string `json:"Multiline_Flush,omitempty"`
	// Name of the parser that machs the beginning of a multiline message. Note that the regular expression defined in the parser must include a group name (named capture)
	ParserFirstline string `json:"Parser_Firstline,omitempty"`
	// Optional-extra parser to interpret and structure multiline entries. This option can be used to define multiple parsers, e.g: Parser_1 ab1,  Parser_2 ab2, Parser_N abN.
	ParserN []string `json:"Parser_N,omitempty"`
	// If enabled, the plugin will recombine split Docker log lines before passing them to any parser as configured above. This mode cannot be used at the same time as Multiline. (default:Off)
	DockerMode string `json:"Docker_Mode,omitempty"`
	// Specify an optional parser for the first line of the docker multiline mode.
	DockerModeParser string `json:"Docker_Mode_Parser,omitempty"`
	//Wait period time in seconds to flush queued unfinished split lines. (default:4)
	DockerModeFlush string `json:"Docker_Mode_Flush,omitempty"`
	// Specify one or multiple parser definitions to apply to the content. Part of the new Multiline Core support in 1.8 (default: "")
	MultilineParser []string `json:"multiline.parser,omitempty"`
}

// FilterKubernetes Fluent Bit Kubernetes Filter allows to enrich your log files with Kubernetes metadata.
type FilterKubernetes struct {
	// Match filtered records (default:kube.*)
	Match string `json:"Match,omitempty" plugin:"default:kubernetes.*"`
	// Set the buffer size for HTTP client when reading responses from Kubernetes API server. The value must be according to the Unit Size specification. A value of 0 results in no limit, and the buffer will expand as-needed. Note that if pod specifications exceed the buffer limit, the API response will be discarded when retrieving metadata, and some kubernetes metadata will fail to be injected to the logs. If this value is empty we will set it "0". (default:"0")
	BufferSize string `json:"Buffer_Size,omitempty"`
	// API Server end-point (default:https://kubernetes.default.svc:443)
	KubeURL string `json:"Kube_URL,omitempty" plugin:"default:https://kubernetes.default.svc:443"`
	//	CA certificate file (default:/var/run/secrets/kubernetes.io/serviceaccount/ca.crt)
	KubeCAFile string `json:"Kube_CA_File,omitempty" plugin:"default:/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"`
	// Absolute path to scan for certificate files
	KubeCAPath string `json:"Kube_CA_Path,omitempty"`
	// Token file  (default:/var/run/secrets/kubernetes.io/serviceaccount/token)
	KubeTokenFile string `json:"Kube_Token_File,omitempty" plugin:"default:/var/run/secrets/kubernetes.io/serviceaccount/token"`
	// Token TTL configurable 'time to live' for the K8s token. By default, it is set to 600 seconds. After this time, the token is reloaded from Kube_Token_File or the Kube_Token_Command.  (default:"600")
	KubeTokenTTL string `json:"Kube_Token_TTL,omitempty" plugin:"default:600"`
	// When the source records comes from Tail input plugin, this option allows to specify what's the prefix used in Tail configuration. (default:kube.var.log.containers.)
	KubeTagPrefix string `json:"Kube_Tag_Prefix,omitempty" plugin:"default:kubernetes.var.log.containers"`
	// When enabled, it checks if the log field content is a JSON string map, if so, it append the map fields as part of the log structure. (default:Off)
	MergeLog string `json:"Merge_Log,omitempty" plugin:"default:On"`
	// When Merge_Log is enabled, the filter tries to assume the log field from the incoming message is a JSON string message and make a structured representation of it at the same level of the log field in the map. Now if Merge_Log_Key is set (a string name), all the new structured fields taken from the original log content are inserted under the new key.
	MergeLogKey string `json:"Merge_Log_Key,omitempty"`
	// When Merge_Log is enabled, trim (remove possible \n or \r) field values.  (default:On)
	MergeLogTrim string `json:"Merge_Log_Trim,omitempty"`
	// Optional parser name to specify how to parse the data contained in the log key. Recommended use is for developers or testing only.
	MergeParser string `json:"Merge_Parser,omitempty"`
	// When Keep_Log is disabled, the log field is removed from the incoming message once it has been successfully merged (Merge_Log must be enabled as well). (default:On)
	KeepLog string `json:"Keep_Log,omitempty"`
	// Debug level between 0 (nothing) and 4 (every detail). (default:-1)
	TLSDebug string `json:"tls.debug,omitempty"`
	// When enabled, turns on certificate validation when connecting to the Kubernetes API server. (default:On)
	TLSVerify string `json:"tls.verify,omitempty"`
	// When enabled, the filter reads logs coming in Journald format. (default:Off)
	UseJournal string `json:"Use_Journal,omitempty"`
	// When enabled, metadata will be fetched from K8s when docker_id is changed. (default:Off)
	CacheUseDockerId string `json:"Cache_Use_Docker_Id,omitempty"`
	// Set an alternative Parser to process record Tag and extract pod_name, namespace_name, container_name and docker_id. The parser must be registered in a parsers file (refer to parser filter-kube-test as an example).
	RegexParser string `json:"Regex_Parser,omitempty"`
	// Allow Kubernetes Pods to suggest a pre-defined Parser (read more about it in Kubernetes Annotations section) (default:Off)
	K8SLoggingParser string `json:"K8S-Logging.Parser,omitempty"`
	// Allow Kubernetes Pods to exclude their logs from the log processor (read more about it in Kubernetes Annotations section). (default:Off)
	K8SLoggingExclude string `json:"K8S-Logging.Exclude,omitempty"`
	// Include Kubernetes resource labels in the extra metadata. (default:On)
	Labels string `json:"Labels,omitempty"`
	// Include Kubernetes resource annotations in the extra metadata. (default:On)
	Annotations string `json:"Annotations,omitempty"`
	// If set, Kubernetes meta-data can be cached/pre-loaded from files in JSON format in this directory, named as namespace-pod.meta
	KubeMetaPreloadCacheDir string `json:"Kube_meta_preload_cache_dir,omitempty"`
	// If set, use dummy-meta data (for test/dev purposes) (default:Off)
	DummyMeta string `json:"Dummy_Meta,omitempty"`
	// DNS lookup retries N times until the network start working (default:6)
	DNSRetries string `json:"DNS_Retries,omitempty"`
	// DNS lookup interval between network status checks (default:30)
	DNSWaitTime string `json:"DNS_Wait_Time,omitempty"`
	// This is an optional feature flag to get metadata information from kubelet instead of calling Kube Server API to enhance the log. (default:Off)
	UseKubelet string `json:"Use_Kubelet,omitempty"`
	// kubelet port using for HTTP request, this only works when Use_Kubelet  set to On (default:10250)
	KubeletPort string `json:"Kubelet_Port,omitempty"`
	// Configurable TTL for K8s cached metadata. By default, it is set to 0 which means TTL for cache entries is disabled and cache entries are evicted at random when capacity is reached. In order to enable this option, you should set the number to a time interval. For example, set this value to 60 or 60s and cache entries which have been created more than 60s will be evicted. (default:0)
	KubeMetaCacheTTL string `json:"Kube_Meta_Cache_TTL,omitempty"`
}

// FilterAws The AWS Filter Enriches logs with AWS Metadata.
type FilterAws struct {
	// Specify which version of the instance metadata service to use. Valid values are 'v1' or 'v2' (default).
	ImdsVersion string `json:"imds_version,omitempty" plugin:"default:v2"`
	// The availability zone (default:true).
	AZ *bool `json:"az,omitempty" plugin:"default:true"`
	// The EC2 instance ID. (default:true)
	Ec2InstanceID *bool `json:"ec2_instance_id,omitempty" plugin:"default:true"`
	// The EC2 instance type. (default:false)
	Ec2InstanceType *bool `json:"ec2_instance_type,omitempty" plugin:"default:false"`
	// The EC2 instance private ip. (default:false)
	PrivateIP *bool `json:"private_ip,omitempty" plugin:"default:false"`
	// The EC2 instance image id. (default:false)
	AmiID *bool `json:"ami_id,omitempty" plugin:"default:false"`
	// The account ID for current EC2 instance. (default:false)
	AccountID *bool `json:"account_id,omitempty" plugin:"default:false"`
	// The hostname for current EC2 instance. (default:false)
	Hostname *bool `json:"hostname,omitempty" plugin:"default:false"`
	// The VPC ID for current EC2 instance. (default:false)
	VpcID *bool `json:"vpc_id,omitempty" plugin:"default:false"`
	// Match filtered records (default:*)
	Match string `json:"Match,omitempty" plugin:"default:*"`
}

// FilterModify The Modify Filter plugin allows you to change records using rules and conditions.
type FilterModify struct {
	// Fluentbit Filter Modification Rule
	Rules []FilterModifyRule `json:"rules,omitempty"`
	// Fluentbit Filter Modification Condition
	Conditions []FilterModifyCondition `json:"conditions,omitempty"`
}

// FilterModifyRule The Modify Filter plugin allows you to change records using rules and conditions.
type FilterModifyRule struct {
	// Add a key/value pair with key KEY and value VALUE. If KEY already exists, this field is overwritten
	Set *FilterKeyValue `json:"Set,omitempty"`
	// Add a key/value pair with key KEY and value VALUE if KEY does not exist
	Add *FilterKeyValue `json:"Add,omitempty" `
	// Remove a key/value pair with key KEY if it exists
	Remove *FilterKey `json:"Remove,omitempty" `
	// Remove all key/value pairs with key matching wildcard KEY
	RemoveWildcard *FilterKey `json:"Remove_wildcard,omitempty" `
	// Remove all key/value pairs with key matching regexp KEY
	RemoveRegex *FilterKey `json:"Remove_regex,omitempty" `
	// Rename a key/value pair with key KEY to RENAMED_KEY if KEY exists AND RENAMED_KEY does not exist
	Rename *FilterKeyValue `json:"Rename,omitempty" `
	// Rename a key/value pair with key KEY to RENAMED_KEY if KEY exists. If RENAMED_KEY already exists, this field is overwritten
	HardRename *FilterKeyValue `json:"Hard_rename,omitempty" `
	// Copy a key/value pair with key KEY to COPIED_KEY if KEY exists AND COPIED_KEY does not exist
	Copy *FilterKeyValue `json:"Copy,omitempty" `
	// Copy a key/value pair with key KEY to COPIED_KEY if KEY exists. If COPIED_KEY already exists, this field is overwritten
	HardCopy *FilterKeyValue `json:"Hard_copy,omitempty" `
}

// FilterModifyCondition The Modify Filter plugin allows you to change records using rules and conditions.
type FilterModifyCondition struct {
	// Is true if KEY exists
	KeyExists *FilterKey `json:"Key_exists,omitempty"`
	// Is true if KEY does not exist
	KeyDoesNotExist *FilterKeyValue `json:"Key_does_not_exist,omitempty"`
	// Is true if a key matches regex KEY
	AKeyMatches *FilterKey `json:"A_key_matches,omitempty"`
	// Is true if no key matches regex KEY
	NoKeyMatches *FilterKey `json:"No_key_matches,omitempty"`
	// Is true if KEY exists and its value is VALUE
	KeyValueEquals *FilterKeyValue `json:"Key_value_equals,omitempty"`
	// Is true if KEY exists and its value is not VALUE
	KeyValueDoesNotEqual *FilterKeyValue `json:"Key_value_does_not_equal,omitempty"`
	// Is true if key KEY exists and its value matches VALUE
	KeyValueMatches *FilterKeyValue `json:"Key_value_matches,omitempty"`
	// Is true if key KEY exists and its value does not match VALUE
	KeyValueDoesNotMatch *FilterKeyValue `json:"Key_value_does_not_match,omitempty"`
	// Is true if all keys matching KEY have values that match VALUE
	MatchingKeysHaveMatchingValues *FilterKeyValue `json:"Matching_keys_have_matching_values,omitempty"`
	// Is true if all keys matching KEY have values that do not match VALUE
	MatchingKeysDoNotHaveMatchingValues *FilterKeyValue `json:"Matching_keys_do_not_have_matching_values,omitempty"`
}

// Operation Doc stub
type Operation struct {
	Op    string `json:"Op,omitempty"`
	Key   string `json:"Key,omitempty"`
	Value string `json:"Value,omitempty"`
}

func getOperation(c interface{}) (result Operation) {
	vc := reflect.ValueOf(c)
	for i := 0; i < vc.NumField(); i++ {
		field := vc.Field(i)
		if !field.IsNil() {
			result.Op = strings.SplitN(vc.Type().Field(i).Tag.Get("json"), ",", 2)[0]
			switch f := field.Interface().(type) {
			case *FilterKey:
				result.Key = f.Key
			case *FilterKeyValue:
				result.Key, result.Value = f.Key, f.Value
			}
			return
		}
	}
	return
}

func (c FilterModifyCondition) Operation() (result Operation) {
	return getOperation(c)
}
func (r FilterModifyRule) Operation() (result Operation) {
	return getOperation(r)
}

type FilterKey struct {
	Key string `json:"key,omitempty"`
}
type FilterKeyValue struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

// VolumeMount defines source and destination folders of a hostPath type pod mount
type VolumeMount struct {
	// Source folder
	// +kubebuilder:validation:Pattern=^/.+$
	Source string `json:"source"`
	// Destination Folder
	// +kubebuilder:validation:Pattern=^/.+$
	Destination string `json:"destination"`
	// Mount Mode
	ReadOnly *bool `json:"readOnly,omitempty"`
}

// ForwardOptions defines custom forward output plugin options, see https://docs.fluentbit.io/manual/pipeline/outputs/forward
type ForwardOptions struct {
	TimeAsInteger      bool   `json:"Time_as_Integer,omitempty"`
	SendOptions        bool   `json:"Send_options,omitempty"`
	RequireAckResponse bool   `json:"Require_ack_response,omitempty"`
	Tag                string `json:"Tag,omitempty"`
	RetryLimit         string `json:"Retry_Limit,omitempty"`
	// `storage.total_limit_size` Limit the maximum number of Chunks in the filesystem for the current output logical destination.
	StorageTotalLimitSize string `json:"storage.total_limit_size,omitempty"`
}
