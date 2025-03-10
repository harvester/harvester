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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/output"
)

// +name:"SyslogNGOutputSpec"
// +weight:"200"
type _hugoSyslogNGOutputSpec interface{} //nolint:deadcode,unused

// +name:"SyslogNGOutputSpec"
// +version:"v1beta1"
// +description:"SyslogNGOutputSpec defines the desired state of SyslogNGOutput"
type _metaSyslogNGOutputSpec interface{} //nolint:deadcode,unused

// SyslogNGOutputSpec defines the desired state of SyslogNGOutput
type SyslogNGOutputSpec struct {
	LoggingRef      string                        `json:"loggingRef,omitempty"`
	Loggly          *output.Loggly                `json:"loggly,omitempty" syslog-ng:"dest-drv"`
	Syslog          *output.SyslogOutput          `json:"syslog,omitempty" syslog-ng:"dest-drv"`
	File            *output.FileOutput            `json:"file,omitempty" syslog-ng:"dest-drv"`
	MQTT            *output.MQTT                  `json:"mqtt,omitempty" syslog-ng:"dest-drv"`
	Redis           *output.RedisOutput           `json:"redis,omitempty" syslog-ng:"dest-drv"`
	MongoDB         *output.MongoDB               `json:"mongodb,omitempty" syslog-ng:"dest-drv"`
	SumologicHTTP   *output.SumologicHTTPOutput   `json:"sumologic-http,omitempty" syslog-ng:"dest-drv"`
	SumologicSyslog *output.SumologicSyslogOutput `json:"sumologic-syslog,omitempty" syslog-ng:"dest-drv"`
	HTTP            *output.HTTPOutput            `json:"http,omitempty" syslog-ng:"dest-drv"`
	Elasticsearch   *output.ElasticsearchOutput   `json:"elasticsearch,omitempty" syslog-ng:"dest-drv,name=elasticsearch-http"`
	LogScale        *output.LogScaleOutput        `json:"logscale,omitempty" syslog-ng:"dest-drv"`
	SplunkHEC       *output.SplunkHECOutput       `json:"splunk_hec_event,omitempty" syslog-ng:"dest-drv"`
	// Available in Logging operator version 4.4 and later.
	Loki *output.LokiOutput `json:"loki,omitempty" syslog-ng:"dest-drv"`
	// Available in Logging operator version 4.4 and later.
	S3 *output.S3Output `json:"s3,omitempty" syslog-ng:"dest-drv"`
	// Available in Logging operator version 4.5 and later.
	Openobserve *output.OpenobserveOutput `json:"openobserve,omitempty" syslog-ng:"dest-drv,name=openobserve-log"`
}

type SyslogNGOutputStatus OutputStatus

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=logging-all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Is the output active?"
// +kubebuilder:printcolumn:name="Problems",type="integer",JSONPath=".status.problemsCount",description="Number of problems"
// +kubebuilder:storageversion

// SyslogNGOutput is the Schema for the syslog-ng outputs API
type SyslogNGOutput struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SyslogNGOutputSpec   `json:"spec,omitempty"`
	Status SyslogNGOutputStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SyslogNGOutputList contains a list of SyslogNGOutput
type SyslogNGOutputList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyslogNGOutput `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SyslogNGOutput{}, &SyslogNGOutputList{})
}
