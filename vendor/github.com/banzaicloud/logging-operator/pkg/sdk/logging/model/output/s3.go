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
	"errors"
	"fmt"

	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
	"github.com/banzaicloud/operator-tools/pkg/utils"
)

// +name:"Amazon S3"
// +weight:"200"
type _hugoS3 interface{} //nolint:deadcode,unused

// +docName:"Amazon S3 plugin for Fluentd"
// **s3** output plugin buffers event logs in local file and upload it to S3 periodically. This plugin splits files exactly by using the time of event logs (not the time when the logs are received). For example, a log '2011-01-02 message B' is reached, and then another log '2011-01-03 message B' is reached in this order, the former one is stored in "20110102.gz" file, and latter one in "20110103.gz" file.
// >Example: [S3 Output Deployment](../../../../quickstarts/example-s3/)
//
// ## Example output configurations
// ```yaml
// spec:
//
//	s3:
//	  aws_key_id:
//	    valueFrom:
//	      secretKeyRef:
//	        name: logging-s3
//	        key: awsAccessKeyId
//	  aws_sec_key:
//	    valueFrom:
//	      secretKeyRef:
//	        name: logging-s3
//	        key: awsSecretAccessKey
//	  s3_bucket: logging-amazon-s3
//	  s3_region: eu-central-1
//	  path: logs/${tag}/%Y/%m/%d/
//	  buffer:
//	    timekey: 10m
//	    timekey_wait: 30s
//	    timekey_use_utc: true
//
// ```
type _docS3 interface{} //nolint:deadcode,unused

// +name:"Amazon S3"
// +url:"https://github.com/fluent/fluent-plugin-s3/releases/tag/v1.6.1"
// +version:"1.6.1"
// +description:"Store logs in Amazon S3"
// +status:"GA"
type _metaS3 interface{} //nolint:deadcode,unused

const (
	OneEyePathTemplate    string = "%v/%%Y/%%m/%%d/${$.kubernetes.namespace_name}/${$.kubernetes.pod_name}/${$.kubernetes.container_name}/"
	OneEyeObjectKeyFormat string = "%{path}%H:%M_%{index}.%{file_extension}"
	OneEyeTags            string = "tag,time,$.kubernetes.namespace_name,$.kubernetes.pod_name,$.kubernetes.container_name"
)

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type S3OutputConfig struct {
	// AWS access key id
	// +docLink:"Secret,../secret/"
	AwsAccessKey *secret.Secret `json:"aws_key_id,omitempty"`
	// AWS secret key.
	// +docLink:"Secret,../secret/"
	AwsSecretKey *secret.Secret `json:"aws_sec_key,omitempty"`
	// Check AWS key on start
	CheckApikeyOnStart string `json:"check_apikey_on_start,omitempty"`
	// Allows grantee to read the object data and its metadata
	GrantRead string `json:"grant_read,omitempty"`
	// Overwrite already existing path
	Overwrite string `json:"overwrite,omitempty"`
	// Path prefix of the files on S3
	Path string `json:"path,omitempty"`
	// Allows grantee to write the ACL for the applicable object
	GrantWriteAcp string `json:"grant_write_acp,omitempty"`
	// Check bucket if exists or not
	CheckBucket string `json:"check_bucket,omitempty"`
	// Specifies the customer-provided encryption key for Amazon S3 to use in encrypting data
	SseCustomerKey string `json:"sse_customer_key,omitempty" default:"10m"`
	// Specifies the 128-bit MD5 digest of the encryption key according to RFC 1321
	SseCustomerKeyMd5 string `json:"sse_customer_key_md5,omitempty"`
	// AWS SDK uses MD5 for API request/response by default
	ComputeChecksums string `json:"compute_checksums,omitempty"`
	// Given a threshold to treat events as delay, output warning logs if delayed events were put into s3
	WarnForDelay string `json:"warn_for_delay,omitempty"`
	// Use aws-sdk-ruby bundled cert
	UseBundledCert string `json:"use_bundled_cert,omitempty"`
	// Custom S3 endpoint (like minio)
	S3Endpoint string `json:"s3_endpoint,omitempty"`
	// Specifies the AWS KMS key ID to use for object encryption
	SsekmsKeyId string `json:"ssekms_key_id,omitempty"`
	// Arbitrary S3 metadata headers to set for the object
	S3Metadata string `json:"s3_metadata,omitempty"`
	// If true, the bucket name is always left in the request URI and never moved to the host as a sub-domain
	ForcePathStyle string `json:"force_path_style,omitempty"`
	// Create S3 bucket if it does not exists
	AutoCreateBucket string `json:"auto_create_bucket,omitempty"`
	// `sprintf` format for `%{index}`
	IndexFormat string `json:"index_format,omitempty"`
	// Signature version for API Request (s3,v4)
	SignatureVersion string `json:"signature_version,omitempty"`
	// If true, S3 Transfer Acceleration will be enabled for uploads. IMPORTANT: You must first enable this feature on your destination S3 bucket
	EnableTransferAcceleration string `json:"enable_transfer_acceleration,omitempty"`
	// If false, the certificate of endpoint will not be verified
	SslVerifyPeer string `json:"ssl_verify_peer,omitempty"`
	// URI of proxy environment
	ProxyUri string `json:"proxy_uri,omitempty"`
	// Allows grantee to read the object ACL
	GrantReadAcp string `json:"grant_read_acp,omitempty"`
	// Check object before creation
	CheckObject string `json:"check_object,omitempty"`
	// Specifies the algorithm to use to when encrypting the object
	SseCustomerAlgorithm string `json:"sse_customer_algorithm,omitempty"`
	// The Server-side encryption algorithm used when storing this object in S3 (AES256, aws:kms)
	UseServerSideEncryption string `json:"use_server_side_encryption,omitempty"`
	// S3 region name
	S3Region string `json:"s3_region,omitempty"`
	// Permission for the object in S3
	Acl string `json:"acl,omitempty"`
	// Allows grantee READ, READ_ACP, and WRITE_ACP permissions on the object
	GrantFullControl string `json:"grant_full_control,omitempty"`
	// The length of `%{hex_random}` placeholder(4-16)
	HexRandomLength string `json:"hex_random_length,omitempty"`
	// The format of S3 object keys (default: %{path}%{time_slice}_%{uuid_hash}_%{index}.%{file_extension})
	S3ObjectKeyFormat string `json:"s3_object_key_format,omitempty" plugin:"default:%{path}%{time_slice}_%{uuid_hash}_%{index}.%{file_extension}"`
	// S3 bucket name
	S3Bucket string `json:"s3_bucket"`
	// Archive format on S3
	StoreAs string `json:"store_as,omitempty"`
	// The type of storage to use for the object, for example STANDARD, REDUCED_REDUNDANCY, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING, GLACIER, DEEP_ARCHIVE, OUTPOSTS, GLACIER_IR
	// For a complete list of possible values, see the [Amazon S3 API reference](https://docs.aws.amazon.com/AmazonS3/latest/API/API_PutObject.html#AmazonS3-PutObject-request-header-StorageClass).
	StorageClass string `json:"storage_class,omitempty"`
	// The number of attempts to load instance profile credentials from the EC2 metadata service using IAM role
	AwsIamRetries string `json:"aws_iam_retries,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
	// +docLink:"Format,../format/"
	Format *Format `json:"format,omitempty"`
	// +docLink:"Assume Role Credentials,#assume-role-credentials"
	AssumeRoleCredentials *S3AssumeRoleCredentials `json:"assume_role_credentials,omitempty"`
	// +docLink:"Instance Profile Credentials,#instance-profile-credentials"
	InstanceProfileCredentials *S3InstanceProfileCredentials `json:"instance_profile_credentials,omitempty"`
	// +docLink:"Shared Credentials,#shared-credentials"
	SharedCredentials *S3SharedCredentials `json:"shared_credentials,omitempty"`
	// Parquet compressor
	Compress *Compress `json:"compress,omitempty"`
	// One-eye format trigger (default:false)
	OneEyeFormat bool `json:"oneeye_format,omitempty"`
	// Custom cluster name (default:one-eye)
	ClusterName string `json:"clustername,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Assume Role Credentials"
// assume_role_credentials
type S3AssumeRoleCredentials struct {
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
// +docName:"Instance Profile Credentials"
// instance_profile_credentials
type S3InstanceProfileCredentials struct {
	// IP address (default:169.254.169.254)
	IpAddress string `json:"ip_address,omitempty"`
	// Port number (default:80)
	Port string `json:"port,omitempty"`
	// Number of seconds to wait for the connection to open
	HttpOpenTimeout string `json:"http_open_timeout,omitempty"`
	// Number of seconds to wait for one block to be read
	HttpReadTimeout string `json:"http_read_timeout,omitempty"`
	// Number of times to retry when retrieving credentials
	Retries string `json:"retries,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Shared Credentials"
// shared_credentials
type S3SharedCredentials struct {
	// Profile name. Default to 'default' or ENV['AWS_PROFILE']
	ProfileName string `json:"profile_name,omitempty"`
	// Path to the shared file. (default: $HOME/.aws/credentials)
	Path string `json:"path,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Parquet compressor"
// parquet compressor
type Compress struct {
	// Parquet compression codec. (uncompressed, snappy, gzip, lzo, brotli, lz4, zstd)(default: snappy)
	ParquetCompressionCodec string `json:"parquet_compression_codec,omitempty"`
	// Parquet file page size. (default: 8192 bytes)
	ParquetPageSize string `json:"parquet_page_size,omitempty"`
	// Parquet file row group size. (default: 128 MB)
	ParquetRowGroupSize string `json:"parquet_row_group_size,omitempty"`
	// Record data format type. (avro csv jsonl msgpack tsv msgpack json) (default: msgpack)
	RecordType string `json:"record_type,omitempty"`
	// Schema type. (avro, bigquery) (default: avro)
	SchemaType string `json:"schema_type,omitempty"`
	// Path to schema file.
	SchemaFile string `json:"schema_file,omitempty"`
}

func (c *S3OutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "s3"
	s3 := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	// Overwrite values when One Eye format is used

	if c.OneEyeFormat {
		clusterName := "one-eye"
		if c.ClusterName != "" {
			clusterName = c.ClusterName
		}
		if c.Buffer == nil {
			c.Buffer = new(Buffer)
		}
		c.Buffer.Tags = utils.StringPointer(OneEyeTags)
		c.Path = fmt.Sprintf(OneEyePathTemplate, clusterName)
		c.S3ObjectKeyFormat = OneEyeObjectKeyFormat
	}
	params, err := types.NewStructToStringMapper(secretLoader).StringsMap(c)
	if err != nil {
		return nil, err
	}

	delete(params, "oneeye_format")
	delete(params, "clustername")

	s3.Params = params

	if c.Buffer == nil {
		c.Buffer = &Buffer{}
	}
	if buffer, err := c.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		s3.SubDirectives = append(s3.SubDirectives, buffer)
	}
	if c.Format != nil {
		if format, err := c.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			s3.SubDirectives = append(s3.SubDirectives, format)
		}
	}
	if c.Compress != nil {
		if compress, err := types.NewFlatDirective(types.PluginMeta{Directive: "compress"},
			c.Compress, secretLoader); err != nil {
			return nil, err
		} else {
			s3.SubDirectives = append(s3.SubDirectives, compress)
		}
	}
	if err := c.validateAndSetCredentials(s3, secretLoader); err != nil {
		return nil, err
	}
	return s3, nil
}

func (c *S3OutputConfig) validateAndSetCredentials(s3 *types.OutputPlugin, secretLoader secret.SecretLoader) error {
	if c.AssumeRoleCredentials != nil {
		if directive, err := types.NewFlatDirective(types.PluginMeta{Directive: "assume_role_credentials"},
			c.AssumeRoleCredentials, secretLoader); err != nil {
			return err
		} else {
			s3.SubDirectives = append(s3.SubDirectives, directive)
		}
	}
	if c.InstanceProfileCredentials != nil {
		if c.AssumeRoleCredentials != nil {
			return errors.New("assume_role_credentials and instance_profile_credentials cannot be set simultaneously")
		}
		if directive, err := types.NewFlatDirective(types.PluginMeta{Directive: "instance_profile_credentials"},
			c.InstanceProfileCredentials, secretLoader); err != nil {
			return err
		} else {
			s3.SubDirectives = append(s3.SubDirectives, directive)
		}
	}
	if c.SharedCredentials != nil {
		if c.AssumeRoleCredentials != nil {
			return errors.New("assume_role_credentials and shared_credentials cannot be set simultaneously")
		}
		if c.InstanceProfileCredentials != nil {
			return errors.New("instance_profile_credentials and shared_credentials cannot be set simultaneously")
		}
		if directive, err := types.NewFlatDirective(types.PluginMeta{Directive: "shared_credentials"},
			c.SharedCredentials, secretLoader); err != nil {
			return err
		} else {
			s3.SubDirectives = append(s3.SubDirectives, directive)
		}
	}
	return nil
}
