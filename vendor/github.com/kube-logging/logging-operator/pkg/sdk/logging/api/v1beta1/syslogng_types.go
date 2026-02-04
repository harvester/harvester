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
	"github.com/cisco-open/operator-tools/pkg/typeoverride"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/filter"
)

// +name:"SyslogNGSpec"
// +weight:"200"
type _hugoSyslogNGSpec interface{} //nolint:deadcode,unused

// +name:"SyslogNGSpec"
// +version:"v1beta1"
// +description:"SyslogNGSpec defines the desired state of SyslogNG"
type _metaSyslogNGSpec interface{} //nolint:deadcode,unused

const (
	defaultSyslogngImageRepository           = "ghcr.io/axoflow/axosyslog"
	defaultSyslogngImageTag                  = "4.11.0"
	defaultPrometheusExporterImageRepository = "ghcr.io/axoflow/axosyslog-metrics-exporter"
	defaultPrometheusExporterImageTag        = "0.0.9"
	defaultConfigReloaderImageRepository     = "ghcr.io/kube-logging/logging-operator/syslog-ng-reloader"
	defaultConfigReloaderImageTag            = "latest"
	defaultBufferVolumeImageRepository       = "ghcr.io/kube-logging/logging-operator/node-exporter"
	defaultBufferVolumeImageTag              = "latest"
)

// +kubebuilder:object:generate=true

// SyslogNGSpec defines the desired state of SyslogNG
type SyslogNGSpec struct {
	SyslogNGImage                       *BasicImageSpec              `json:"syslogNGImage,omitempty"`
	ConfigReloadImage                   *BasicImageSpec              `json:"configReloadImage,omitempty"`
	MetricsExporterImage                *BasicImageSpec              `json:"metricsExporterImage,omitempty"`
	BufferVolumeMetricsImage            *BasicImageSpec              `json:"bufferVolumeMetricsImage,omitempty"`
	TLS                                 SyslogNGTLS                  `json:"tls,omitempty"`
	ReadinessDefaultCheck               ReadinessDefaultCheck        `json:"readinessDefaultCheck,omitempty"`
	SkipRBACCreate                      bool                         `json:"skipRBACCreate,omitempty"`
	StatefulSetOverrides                *typeoverride.StatefulSet    `json:"statefulSet,omitempty"`
	ServiceOverrides                    *typeoverride.Service        `json:"service,omitempty"`
	ServiceAccountOverrides             *typeoverride.ServiceAccount `json:"serviceAccount,omitempty"`
	ConfigCheckPodOverrides             *typeoverride.PodSpec        `json:"configCheckPod,omitempty"`
	Metrics                             *Metrics                     `json:"metrics,omitempty"`
	MetricsServiceOverrides             *typeoverride.Service        `json:"metricsService,omitempty"`
	BufferVolumeMetrics                 *BufferMetrics               `json:"bufferVolumeMetrics,omitempty"`
	BufferVolumeMetricsServiceOverrides *typeoverride.Service        `json:"bufferVolumeMetricsService,omitempty"`
	BufferVolumeMetricsResources        corev1.ResourceRequirements  `json:"bufferVolumeMetricsResources,omitempty"`
	BufferVolumeMetricsLivenessProbe    *corev1.Probe                `json:"bufferVolumeMetricsLivenessProbe,omitempty"`
	GlobalOptions                       *GlobalOptions               `json:"globalOptions,omitempty"`
	JSONKeyPrefix                       string                       `json:"jsonKeyPrefix,omitempty"`
	JSONKeyDelimiter                    string                       `json:"jsonKeyDelim,omitempty"`
	// Available in Logging operator version 4.5 and later.
	// Parses date automatically from the timestamp registered by the container runtime.
	// Note: `jsonKeyPrefix` and `jsonKeyDelim` are respected.
	SourceDateParser *SourceDateParser `json:"sourceDateParser,omitempty"`
	// Available in Logging operator version 4.5 and later.
	// Set the maximum number of connections for the source. For details, see [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-routing-filters/concepts-flow-control/configuring-flow-control/).
	MaxConnections int `json:"maxConnections,omitempty"`
	LogIWSize      int `json:"logIWSize,omitempty"`
	// Available in Logging operator version 4.5 and later.
	// Create [custom log metrics for sources and outputs]({{< relref "/docs/examples/custom-syslog-ng-metrics.md" >}}).
	SourceMetrics []filter.MetricsProbe `json:"sourceMetrics,omitempty"`
	// Overrides the default logging level configCheck setup.
	// This field is not used directly, just copied over the field in the logging resource if defined.
	ConfigCheck *ConfigCheck `json:"configCheck,omitempty"`
}

//
/*
Available in Logging operator version 4.5 and later.

Parses date automatically from the timestamp registered by the container runtime.
Note: `jsonKeyPrefix` and `jsonKeyDelim` are respected.
It is disabled by default, but if enabled, then the default settings parse the timestamp written by the container runtime and parsed by Fluent Bit using the `cri` or the `docker` parser.
*/
type SourceDateParser struct {
	// Default: "%FT%T.%f%z"
	Format *string `json:"format,omitempty"`
	// Default(depending on JSONKeyPrefix): "${json.time}"
	Template *string `json:"template,omitempty"`
}

// +kubebuilder:object:generate=true

// SyslogNGTLS defines the TLS configs
type SyslogNGTLS struct {
	Enabled    bool   `json:"enabled"`
	SecretName string `json:"secretName,omitempty"`
	SharedKey  string `json:"sharedKey,omitempty"`
}

type GlobalOptions struct {
	// Deprecated. Use stats/level from 4.1+
	StatsLevel *int `json:"stats_level,omitempty"`
	// Deprecated. Use stats/freq from 4.1+
	StatsFreq *int `json:"stats_freq,omitempty"`
	// See the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-global-options/reference-options/#global-option-stats).
	Stats *Stats `json:"stats,omitempty"`
	// See the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-global-options/reference-options/#global-options-log-level).
	LogLevel *string `json:"log_level,omitempty"`
}

type Stats struct {
	Level *int `json:"level,omitempty"`
	Freq  *int `json:"freq,omitempty"`
}

func (s *SyslogNGSpec) SetDefaults() {
	if s != nil {
		// if s.MaxConnections == 0 {
		// 	max connections is now configured dynamically if not set
		// }
		if s.Metrics != nil {
			if s.Metrics.Path == "" {
				s.Metrics.Path = "/metrics"
			}
			if s.Metrics.Port == 0 {
				s.Metrics.Port = 9577
			}
			if s.Metrics.Timeout == "" {
				s.Metrics.Timeout = "5s"
			}
			if s.Metrics.Interval == "" {
				s.Metrics.Interval = "15s"
			}
		}
		if s.SyslogNGImage == nil {
			s.SyslogNGImage = &BasicImageSpec{
				Repository: defaultSyslogngImageRepository,
				Tag:        defaultSyslogngImageTag,
			}
		}
		if s.ConfigReloadImage == nil {
			s.ConfigReloadImage = &BasicImageSpec{}
		}
		if s.ConfigReloadImage.Repository == "" {
			s.ConfigReloadImage.Repository = defaultConfigReloaderImageRepository
		}
		if s.ConfigReloadImage.Tag == "" {
			if Version == "" {
				s.ConfigReloadImage.Tag = defaultConfigReloaderImageTag
			} else {
				s.ConfigReloadImage.Tag = Version
			}
		}
		if s.MetricsExporterImage == nil {
			s.MetricsExporterImage = &BasicImageSpec{
				Repository: defaultPrometheusExporterImageRepository,
				Tag:        defaultPrometheusExporterImageTag,
			}
		}
		if s.BufferVolumeMetricsImage == nil {
			s.BufferVolumeMetricsImage = &BasicImageSpec{}
		}
		if s.BufferVolumeMetricsImage.Repository == "" {
			s.BufferVolumeMetricsImage.Repository = defaultBufferVolumeImageRepository
		}
		if s.BufferVolumeMetricsImage.Tag == "" {
			if Version == "" {
				s.BufferVolumeMetricsImage.Tag = defaultBufferVolumeImageTag
			} else {
				s.BufferVolumeMetricsImage.Tag = Version
			}
		}
		if s.BufferVolumeMetricsResources.Limits == nil {
			s.BufferVolumeMetricsResources.Limits = corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("100M"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			}
		}
		if s.BufferVolumeMetricsResources.Requests == nil {
			s.BufferVolumeMetricsResources.Requests = corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("20M"),
				corev1.ResourceCPU:    resource.MustParse("2m"),
			}
		}
		if s.BufferVolumeMetricsLivenessProbe == nil {
			s.BufferVolumeMetricsLivenessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Port:   intstr.FromString("buffer-metrics"),
						Scheme: corev1.URISchemeHTTP,
					},
				},
				InitialDelaySeconds: 600,
				TimeoutSeconds:      5,
				PeriodSeconds:       30,
				SuccessThreshold:    1,
			}
		}
	}
}
