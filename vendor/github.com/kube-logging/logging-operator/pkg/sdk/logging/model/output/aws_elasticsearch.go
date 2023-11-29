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
	"github.com/cisco-open/operator-tools/pkg/utils"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Amazon Elasticsearch"
// +weight:"200"
type _hugoAwsElasticsearch interface{} //nolint:deadcode,unused

// +docName:"Amazon Elasticsearch output plugin for Fluentd"
//
//	More info at https://github.com/atomita/fluent-plugin-aws-elasticsearch-service
//
// ## Example output configurations
// {{< highlight yaml >}}
// spec:
//
//	awsElasticsearch:
//	  logstash_format: true
//	  include_tag_key: true
//	  tag_key: "@log_name"
//	  flush_interval: 1s
//	  endpoint:
//	    url: https://CLUSTER_ENDPOINT_URL
//	    region: eu-west-1
//	    access_key_id:
//	      value: aws-key
//	    secret_access_key:
//	      value: aws_secret
//
// {{</ highlight >}}
type _docAwsElasticsearch interface{} //nolint:deadcode,unused

// +name:"Amazon Elasticsearch"
// +url:"https://github.com/atomita/fluent-plugin-aws-elasticsearch-service"
// +version:"2.4.1"
// +description:"Fluent plugin for Amazon Elasticsearch"
// +status:"Testing"
type _metaAwsElasticsearch interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Amazon Elasticsearch"
// Send your logs to a Amazon Elasticsearch Service
type AwsElasticsearchOutputConfig struct {

	// flush_interval
	FlushInterval string `json:"flush_interval,omitempty"`

	// AWS Endpoint Credentials
	Endpoint *EndpointCredentials `json:"endpoint,omitempty"`

	// +docLink:"Format,../format/"
	Format *Format `json:"format,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	//  ElasticSearch
	*ElasticsearchOutput `json:",omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Endpoint Credentials"
// endpoint
type EndpointCredentials struct {
	// AWS region. It should be in form like us-east-1, us-west-2. Default nil, which means try to find from environment variable AWS_REGION.
	Region string `json:"region,omitempty"`

	// AWS connection url.
	Url string `json:"url,omitempty"`

	// AWS access key id. This parameter is required when your agent is not running on EC2 instance with an IAM Role.
	AccessKeyId *secret.Secret `json:"access_key_id,omitempty"`

	// AWS secret key. This parameter is required when your agent is not running on EC2 instance with an IAM Role.
	SecretAccessKey *secret.Secret `json:"secret_access_key,omitempty"`

	// Typically, you can use AssumeRole for cross-account access or federation.
	AssumeRoleArn *secret.Secret `json:"assume_role_arn,omitempty"`

	// Set with AWS_CONTAINER_CREDENTIALS_RELATIVE_URI environment variable value
	EcsContainerCredentialsRelativeUri *secret.Secret `json:"ecs_container_credentials_relative_uri,omitempty"`

	// AssumeRoleWithWebIdentity https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRoleWithWebIdentity.html
	AssumeRoleSessionName *secret.Secret `json:"assume_role_session_name,omitempty"`

	// AssumeRoleWithWebIdentity https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRoleWithWebIdentity.html
	AssumeRoleWebIdentityTokenFile *secret.Secret `json:"assume_role_web_identity_token_file,omitempty"`

	// By default, the AWS Security Token Service (AWS STS) is available as a global service, and all AWS STS requests go to a single endpoint at https://sts.amazonaws.com. AWS recommends using Regional AWS STS endpoints instead of the global endpoint to reduce latency, build in redundancy, and increase session token validity. https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_enable-regions.html
	StsCredentialsRegion *secret.Secret `json:"sts_credentials_region,omitempty"`
}

func (o *EndpointCredentials) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	return types.NewFlatDirective(types.PluginMeta{
		Directive: "endpoint",
	}, o, secretLoader)
}

func (e *AwsElasticsearchOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "aws-elasticsearch-service"
	awsElastic := &types.OutputPlugin{
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
		awsElastic.Params = params
	}
	if e.ElasticsearchOutput != nil {
		if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(e.ElasticsearchOutput); err != nil {
			return nil, err
		} else {
			awsElastic.Params = utils.MergeLabels(awsElastic.Params, params)
		}
	}

	if e.Endpoint != nil {
		if assumeRoleCredentials, err := e.Endpoint.ToDirective(secretLoader, id); err != nil {
			return nil, err
		} else {
			awsElastic.SubDirectives = append(awsElastic.SubDirectives, assumeRoleCredentials)
		}
	}
	if e.Buffer == nil {
		e.Buffer = &Buffer{}
	}

	if buffer, err := e.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		awsElastic.SubDirectives = append(awsElastic.SubDirectives, buffer)
	}
	if e.Format != nil {
		if format, err := e.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			awsElastic.SubDirectives = append(awsElastic.SubDirectives, format)
		}
	}
	return awsElastic, nil
}
