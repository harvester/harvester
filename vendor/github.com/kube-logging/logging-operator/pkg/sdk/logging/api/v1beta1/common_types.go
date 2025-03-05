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
	v1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
)

// +name:"Common"
// +weight:"200"
type _hugoCommon interface{} //nolint:deadcode,unused

// +name:"Common"
// +version:"v1beta1"
// +description:"ImageSpec Metrics Security"
type _metaCommon interface{} //nolint:deadcode,unused

const (
	HostPath = "/opt/logging-operator/%s/%s"
)

// ImageSpec struct hold information about image specification
type ImageSpec struct {
	Repository       string                        `json:"repository,omitempty"`
	Tag              string                        `json:"tag,omitempty"`
	PullPolicy       string                        `json:"pullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

func (s ImageSpec) RepositoryWithTag() string {
	return RepositoryWithTag(s.Repository, s.Tag)
}

func RepositoryWithTag(repository, tag string) string {
	res := repository
	if tag != "" {
		res += ":" + tag
	}
	return res
}

// Metrics defines the service monitor endpoints
type Metrics struct {
	Interval              string               `json:"interval,omitempty"`
	Timeout               string               `json:"timeout,omitempty"`
	Port                  int32                `json:"port,omitempty"`
	Path                  string               `json:"path,omitempty"`
	ServiceMonitor        bool                 `json:"serviceMonitor,omitempty"`
	ServiceMonitorConfig  ServiceMonitorConfig `json:"serviceMonitorConfig,omitempty"`
	PrometheusAnnotations bool                 `json:"prometheusAnnotations,omitempty"`
	PrometheusRules       bool                 `json:"prometheusRules,omitempty"`
}

// BufferMetrics defines the service monitor endpoints
type BufferMetrics struct {
	Metrics   `json:",inline"`
	MountName string `json:"mount_name,omitempty"`
}

// ServiceMonitorConfig defines the ServiceMonitor properties
type ServiceMonitorConfig struct {
	AdditionalLabels   map[string]string   `json:"additionalLabels,omitempty"`
	HonorLabels        bool                `json:"honorLabels,omitempty"`
	Relabelings        []*v1.RelabelConfig `json:"relabelings,omitempty"`
	MetricsRelabelings []*v1.RelabelConfig `json:"metricRelabelings,omitempty"`
	Scheme             string              `json:"scheme,omitempty"`
	TLSConfig          *v1.TLSConfig       `json:"tlsConfig,omitempty"`
}

// Security defines Fluentd, FluentbitAgent deployment security properties
type Security struct {
	ServiceAccount               string `json:"serviceAccount,omitempty"`
	RoleBasedAccessControlCreate *bool  `json:"roleBasedAccessControlCreate,omitempty"`
	// Warning: this is not supported anymore and does nothing
	PodSecurityPolicyCreate bool                       `json:"podSecurityPolicyCreate,omitempty"`
	SecurityContext         *corev1.SecurityContext    `json:"securityContext,omitempty"`
	PodSecurityContext      *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
}

// ReadinessDefaultCheck Enable default readiness checks
type ReadinessDefaultCheck struct {
	// Enable default Readiness check it'll fail if the buffer volume free space exceeds the `readinessDefaultThreshold` percentage (90%).
	BufferFreeSpace          bool  `json:"bufferFreeSpace,omitempty"`
	BufferFreeSpaceThreshold int32 `json:"bufferFreeSpaceThreshold,omitempty"`
	BufferFileNumber         bool  `json:"bufferFileNumber,omitempty"`
	BufferFileNumberMax      int32 `json:"bufferFileNumberMax,omitempty"`
	InitialDelaySeconds      int32 `json:"initialDelaySeconds,omitempty"`
	TimeoutSeconds           int32 `json:"timeoutSeconds,omitempty"`
	PeriodSeconds            int32 `json:"periodSeconds,omitempty"`
	SuccessThreshold         int32 `json:"successThreshold,omitempty"`
	FailureThreshold         int32 `json:"failureThreshold,omitempty"`
}
