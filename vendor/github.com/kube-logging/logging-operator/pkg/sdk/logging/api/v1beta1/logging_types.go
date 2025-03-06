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
	"errors"
	"fmt"

	util "github.com/cisco-open/operator-tools/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +name:"LoggingSpec"
// +weight:"200"
type _hugoLoggingSpec interface{} //nolint:deadcode,unused

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +name:"Logging"
// +version:"v1beta1"
// +description:"Logging system configuration"
type _metaLoggingSpec interface{} //nolint:deadcode,unused

// LoggingSpec defines the desired state of Logging
type LoggingSpec struct {
	// Reference to the logging system. Each of the `loggingRef`s can manage a fluentbit daemonset and a fluentd statefulset.
	LoggingRef string `json:"loggingRef,omitempty"`
	// Disable configuration check before applying new fluentd configuration.
	FlowConfigCheckDisabled bool `json:"flowConfigCheckDisabled,omitempty"`
	// Whether to skip invalid Flow and ClusterFlow resources
	SkipInvalidResources bool `json:"skipInvalidResources,omitempty"`
	// Override generated config. This is a *raw* configuration string for troubleshooting purposes.
	FlowConfigOverride string `json:"flowConfigOverride,omitempty"`
	// ConfigCheck settings that apply to both fluentd and syslog-ng
	ConfigCheck ConfigCheck `json:"configCheck,omitempty"`
	// FluentbitAgent daemonset configuration.
	// Deprecated, will be removed with next major version
	// Migrate to the standalone NodeAgent resource
	FluentbitSpec *FluentbitSpec `json:"fluentbit,omitempty"`
	// Fluentd statefulset configuration
	FluentdSpec *FluentdSpec `json:"fluentd,omitempty"`
	// Syslog-NG statefulset configuration
	SyslogNGSpec *SyslogNGSpec `json:"syslogNG,omitempty"`
	// Default flow for unmatched logs. This Flow configuration collects all logs that didn't matched any other Flow.
	DefaultFlowSpec *DefaultFlowSpec `json:"defaultFlow,omitempty"`
	// GlobalOutput name to flush ERROR events to
	ErrorOutputRef string `json:"errorOutputRef,omitempty"`
	// Global filters to apply on logs before any match or filter mechanism.
	GlobalFilters []Filter `json:"globalFilters,omitempty"`
	// Limit namespaces to watch Flow and Output custom resources.
	WatchNamespaces []string `json:"watchNamespaces,omitempty"`
	// WatchNamespaceSelector is a LabelSelector to find matching namespaces to watch as in WatchNamespaces
	WatchNamespaceSelector *metav1.LabelSelector `json:"watchNamespaceSelector,omitempty"`
	// Cluster domain name to be used when templating URLs to services (default: "cluster.local.").
	ClusterDomain *string `json:"clusterDomain,omitempty"`
	// Namespace for cluster wide configuration resources like ClusterFlow and ClusterOutput.
	// This should be a protected namespace from regular users.
	// Resources like fluentbit and fluentd will run in this namespace as well.
	ControlNamespace string `json:"controlNamespace"`
	// Allow configuration of cluster resources from any namespace. Mutually exclusive with ControlNamespace restriction of Cluster resources
	AllowClusterResourcesFromAllNamespaces bool `json:"allowClusterResourcesFromAllNamespaces,omitempty"`
	// InlineNodeAgent Configuration
	// Deprecated, will be removed with next major version
	NodeAgents []*InlineNodeAgent `json:"nodeAgents,omitempty"`
	// EnableRecreateWorkloadOnImmutableFieldChange enables the operator to recreate the
	// fluentbit daemonset and the fluentd statefulset (and possibly other resource in the future)
	// in case there is a change in an immutable field
	// that otherwise couldn't be managed with a simple update.
	EnableRecreateWorkloadOnImmutableFieldChange bool `json:"enableRecreateWorkloadOnImmutableFieldChange,omitempty"`
}

type ConfigCheckStrategy string

const (
	ConfigCheckStrategyDryRun  ConfigCheckStrategy = "DryRun"
	ConfigCheckStrategyTimeout ConfigCheckStrategy = "StartWithTimeout"
)

type ConfigCheck struct {
	// Select the config check strategy to use.
	// `DryRun`: Parse and validate configuration.
	// `StartWithTimeout`: Start with given configuration and exit after specified timeout.
	// Default: `DryRun`
	Strategy ConfigCheckStrategy `json:"strategy,omitempty"`

	// Configure timeout in seconds if strategy is StartWithTimeout
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// Labels to use for the configcheck pods on top of labels added by the operator by default. Default values can be overwritten.
	Labels map[string]string `json:"labels,omitempty"`
}

// LoggingStatus defines the observed state of Logging
type LoggingStatus struct {
	// Result of the config check. Under normal conditions there is a single item in the map with a bool value.
	ConfigCheckResults map[string]bool `json:"configCheckResults,omitempty"`
	// Available in Logging operator version 4.5 and later. Name of the matched detached fluentd configuration object.
	FluentdConfigName string `json:"fluentdConfigName,omitempty"`
	// Available in Logging operator version 4.5 and later. Name of the matched detached SyslogNG configuration object.
	SyslogNGConfigName string `json:"syslogNGConfigName,omitempty"`

	// Problems with the logging resource
	Problems []string `json:"problems,omitempty"`
	// Count of problems for printcolumn
	ProblemsCount int `json:"problemsCount,omitempty"`
	// List of namespaces that watchNamespaces + watchNamespaceSelector is resolving to.
	// Not set means all namespaces.
	WatchNamespaces []string `json:"watchNamespaces,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=loggings,scope=Cluster,categories=logging-all
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="LoggingRef",type="string",JSONPath=".spec.loggingRef",description="Logging reference"
// +kubebuilder:printcolumn:name="ControlNamespace",type="string",JSONPath=".spec.controlNamespace",description="Control namespace"
// +kubebuilder:printcolumn:name="WatchNamespaces",type="string",JSONPath=".status.watchNamespaces",description="Watched namespaces"
// +kubebuilder:printcolumn:name="Problems",type="integer",JSONPath=".status.problemsCount",description="Number of problems"

// Logging is the Schema for the loggings API
type Logging struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoggingSpec   `json:"spec,omitempty"`
	Status LoggingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LoggingList contains a list of Logging
type LoggingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Logging `json:"items"`
}

// +kubebuilder:object:generate=true

// DefaultFlowSpec is a Flow for logs that did not match any other Flow
type DefaultFlowSpec struct {
	Filters []Filter `json:"filters,omitempty"`
	// Deprecated
	OutputRefs           []string `json:"outputRefs,omitempty"`
	GlobalOutputRefs     []string `json:"globalOutputRefs,omitempty"`
	FlowLabel            string   `json:"flowLabel,omitempty"`
	IncludeLabelInRouter *bool    `json:"includeLabelInRouter,omitempty"`
}

const (
	DefaultFluentbitImageRepository               = "fluent/fluent-bit"
	DefaultFluentbitImageTag                      = "2.1.8"
	DefaultFluentbitBufferVolumeImageRepository   = "ghcr.io/kube-logging/node-exporter"
	DefaultFluentbitBufferVolumeImageTag          = "v0.7.1"
	DefaultFluentbitBufferStorageVolumeName       = "fluentbit-buffer"
	DefaultFluentbitConfigReloaderImageRepository = "ghcr.io/kube-logging/config-reloader"
	DefaultFluentbitConfigReloaderImageTag        = "v0.0.5"
	DefaultFluentdImageRepository                 = "ghcr.io/kube-logging/fluentd"
	DefaultFluentdImageTag                        = "v1.16-full"
	DefaultFluentdBufferStorageVolumeName         = "fluentd-buffer"
	DefaultFluentdDrainWatchImageRepository       = "ghcr.io/kube-logging/fluentd-drain-watch"
	DefaultFluentdDrainWatchImageTag              = "v0.2.1"
	DefaultFluentdDrainPauseImageRepository       = "k8s.gcr.io/pause"
	DefaultFluentdDrainPauseImageTag              = "3.2"
	DefaultFluentdVolumeModeImageRepository       = "busybox"
	DefaultFluentdVolumeModeImageTag              = "latest"
	DefaultFluentdConfigReloaderImageRepository   = "ghcr.io/kube-logging/config-reloader"
	DefaultFluentdConfigReloaderImageTag          = "v0.0.5"
	DefaultFluentdBufferVolumeImageRepository     = "ghcr.io/kube-logging/node-exporter"
	DefaultFluentdBufferVolumeImageTag            = "v0.7.1"
)

// SetDefaults fills empty attributes
func (l *Logging) SetDefaults() error {
	if l.Spec.ClusterDomain == nil {
		l.Spec.ClusterDomain = util.StringPointer("cluster.local.")
	}
	if !l.Spec.FlowConfigCheckDisabled && l.Status.ConfigCheckResults == nil {
		l.Status.ConfigCheckResults = make(map[string]bool)
	}
	if len(l.Status.FluentdConfigName) == 0 {
		if err := l.Spec.FluentdSpec.SetDefaults(); err != nil {
			return err
		}
	}
	if l.Spec.ConfigCheck.TimeoutSeconds == 0 {
		l.Spec.ConfigCheck.TimeoutSeconds = 10
	}
	if len(l.Status.SyslogNGConfigName) == 0 {
		l.Spec.SyslogNGSpec.SetDefaults()
	}

	return nil
}

func (logging *Logging) WatchAllNamespaces() bool {
	watchNamespaces := logging.Spec.WatchNamespaces
	nsLabelSelector := logging.Spec.WatchNamespaceSelector
	return len(watchNamespaces) == 0 && nsLabelSelector == nil
}

func FluentBitDefaults(fluentbitSpec *FluentbitSpec) error {
	if fluentbitSpec != nil { // nolint:nestif
		if fluentbitSpec.PosisionDBLegacy != nil {
			return errors.New("`position_db` field is deprecated, use `positiondb`")
		}
		if fluentbitSpec.Parser != "" {
			return errors.New("`parser` field is deprecated, use `inputTail.Parser`")
		}
		if fluentbitSpec.Image.Repository == "" {
			fluentbitSpec.Image.Repository = DefaultFluentbitImageRepository
		}
		if fluentbitSpec.Image.Tag == "" {
			fluentbitSpec.Image.Tag = DefaultFluentbitImageTag
		}
		if fluentbitSpec.Image.PullPolicy == "" {
			fluentbitSpec.Image.PullPolicy = "IfNotPresent"
		}
		if fluentbitSpec.Flush == 0 {
			fluentbitSpec.Flush = 1
		}
		if fluentbitSpec.Grace == 0 {
			fluentbitSpec.Grace = 5
		}
		if fluentbitSpec.LogLevel == "" {
			fluentbitSpec.LogLevel = "info"
		}
		if fluentbitSpec.CoroStackSize == 0 {
			fluentbitSpec.CoroStackSize = 24576
		}
		if fluentbitSpec.Resources.Limits == nil {
			fluentbitSpec.Resources.Limits = v1.ResourceList{
				v1.ResourceMemory: resource.MustParse("100M"),
				v1.ResourceCPU:    resource.MustParse("200m"),
			}
		}
		if fluentbitSpec.Resources.Requests == nil {
			fluentbitSpec.Resources.Requests = v1.ResourceList{
				v1.ResourceMemory: resource.MustParse("50M"),
				v1.ResourceCPU:    resource.MustParse("100m"),
			}
		}
		if fluentbitSpec.InputTail.Path == "" {
			fluentbitSpec.InputTail.Path = "/var/log/containers/*.log"
		}
		if fluentbitSpec.InputTail.RefreshInterval == "" {
			fluentbitSpec.InputTail.RefreshInterval = "5"
		}
		if fluentbitSpec.InputTail.SkipLongLines == "" {
			fluentbitSpec.InputTail.SkipLongLines = "On"
		}
		if fluentbitSpec.InputTail.DB == nil {
			fluentbitSpec.InputTail.DB = util.StringPointer("/tail-db/tail-containers-state.db")
		}
		if fluentbitSpec.InputTail.DBLocking == nil {
			fluentbitSpec.InputTail.DBLocking = util.BoolPointer(true)
		}
		if fluentbitSpec.InputTail.MemBufLimit == "" {
			fluentbitSpec.InputTail.MemBufLimit = "5MB"
		}
		if fluentbitSpec.InputTail.Tag == "" {
			fluentbitSpec.InputTail.Tag = "kubernetes.*"
		}
		if fluentbitSpec.Annotations == nil {
			fluentbitSpec.Annotations = make(map[string]string)
		}
		if fluentbitSpec.Security == nil {
			fluentbitSpec.Security = &Security{}
		}
		if fluentbitSpec.Security.RoleBasedAccessControlCreate == nil {
			fluentbitSpec.Security.RoleBasedAccessControlCreate = util.BoolPointer(true)
		}
		if fluentbitSpec.BufferVolumeImage.Repository == "" {
			fluentbitSpec.BufferVolumeImage.Repository = DefaultFluentbitBufferVolumeImageRepository
		}
		if fluentbitSpec.BufferVolumeImage.Tag == "" {
			fluentbitSpec.BufferVolumeImage.Tag = DefaultFluentbitBufferVolumeImageTag
		}
		if fluentbitSpec.BufferVolumeImage.PullPolicy == "" {
			fluentbitSpec.BufferVolumeImage.PullPolicy = "IfNotPresent"
		}
		if fluentbitSpec.BufferVolumeResources.Limits == nil {
			fluentbitSpec.BufferVolumeResources.Limits = v1.ResourceList{
				v1.ResourceMemory: resource.MustParse("10M"),
				v1.ResourceCPU:    resource.MustParse("50m"),
			}
		}
		if fluentbitSpec.BufferVolumeResources.Requests == nil {
			fluentbitSpec.BufferVolumeResources.Requests = v1.ResourceList{
				v1.ResourceMemory: resource.MustParse("10M"),
				v1.ResourceCPU:    resource.MustParse("1m"),
			}
		}
		if fluentbitSpec.Security.SecurityContext == nil {
			fluentbitSpec.Security.SecurityContext = &v1.SecurityContext{}
		}
		if fluentbitSpec.Security.PodSecurityContext == nil {
			fluentbitSpec.Security.PodSecurityContext = &v1.PodSecurityContext{}
		}
		if fluentbitSpec.Metrics != nil {
			if fluentbitSpec.Metrics.Path == "" {
				fluentbitSpec.Metrics.Path = "/api/v1/metrics/prometheus"
			}
			if fluentbitSpec.Metrics.Port == 0 {
				fluentbitSpec.Metrics.Port = 2020
			}
			if fluentbitSpec.Metrics.Timeout == "" {
				fluentbitSpec.Metrics.Timeout = "5s"
			}
			if fluentbitSpec.Metrics.Interval == "" {
				fluentbitSpec.Metrics.Interval = "15s"
			}
			if fluentbitSpec.Metrics.PrometheusAnnotations {
				fluentbitSpec.Annotations["prometheus.io/scrape"] = "true"
				fluentbitSpec.Annotations["prometheus.io/path"] = fluentbitSpec.Metrics.Path
				fluentbitSpec.Annotations["prometheus.io/port"] = fmt.Sprintf("%d", fluentbitSpec.Metrics.Port)
			}
		} else if fluentbitSpec.LivenessDefaultCheck || fluentbitSpec.ConfigHotReload != nil {
			fluentbitSpec.Metrics = &Metrics{
				Port: 2020,
				Path: "/",
			}
		}
		if fluentbitSpec.LivenessProbe == nil {
			if fluentbitSpec.LivenessDefaultCheck {
				fluentbitSpec.LivenessProbe = &v1.Probe{
					ProbeHandler: v1.ProbeHandler{
						HTTPGet: &v1.HTTPGetAction{
							Path: fluentbitSpec.Metrics.Path,
							Port: intstr.IntOrString{
								IntVal: fluentbitSpec.Metrics.Port,
							},
						}},
					InitialDelaySeconds: 10,
					TimeoutSeconds:      0,
					PeriodSeconds:       10,
					SuccessThreshold:    0,
					FailureThreshold:    3,
				}
			}
		}

		if fluentbitSpec.MountPath == "" {
			fluentbitSpec.MountPath = "/var/lib/docker/containers"
		}
		if fluentbitSpec.BufferStorage.StoragePath == "" {
			fluentbitSpec.BufferStorage.StoragePath = "/buffers"
		}
		if fluentbitSpec.FilterAws != nil {
			if fluentbitSpec.FilterAws.ImdsVersion == "" {
				fluentbitSpec.FilterAws.ImdsVersion = "v2"
			}
			if fluentbitSpec.FilterAws.AZ == nil {
				fluentbitSpec.FilterAws.AZ = util.BoolPointer(true)
			}
			if fluentbitSpec.FilterAws.Ec2InstanceID == nil {
				fluentbitSpec.FilterAws.Ec2InstanceID = util.BoolPointer(true)
			}
			if fluentbitSpec.FilterAws.Ec2InstanceType == nil {
				fluentbitSpec.FilterAws.Ec2InstanceType = util.BoolPointer(false)
			}
			if fluentbitSpec.FilterAws.PrivateIP == nil {
				fluentbitSpec.FilterAws.PrivateIP = util.BoolPointer(false)
			}
			if fluentbitSpec.FilterAws.AmiID == nil {
				fluentbitSpec.FilterAws.AmiID = util.BoolPointer(false)
			}
			if fluentbitSpec.FilterAws.AccountID == nil {
				fluentbitSpec.FilterAws.AccountID = util.BoolPointer(false)
			}
			if fluentbitSpec.FilterAws.Hostname == nil {
				fluentbitSpec.FilterAws.Hostname = util.BoolPointer(false)
			}
			if fluentbitSpec.FilterAws.VpcID == nil {
				fluentbitSpec.FilterAws.VpcID = util.BoolPointer(false)
			}
		}
		if len(fluentbitSpec.FilterKubernetes.UseKubelet) == 0 {
			fluentbitSpec.FilterKubernetes.UseKubelet = "Off"
		}
		if fluentbitSpec.FilterKubernetes.UseKubelet == "On" {
			fluentbitSpec.DNSPolicy = "ClusterFirstWithHostNet"
			fluentbitSpec.HostNetwork = true
		}
		if fluentbitSpec.ForwardOptions == nil {
			fluentbitSpec.ForwardOptions = &ForwardOptions{}
		}
		if fluentbitSpec.ForwardOptions.RetryLimit == "" {
			fluentbitSpec.ForwardOptions.RetryLimit = "False"
		}
		if fluentbitSpec.TLS == nil {
			fluentbitSpec.TLS = &FluentbitTLS{}
		}
		if fluentbitSpec.TLS.Enabled == nil {
			fluentbitSpec.TLS.Enabled = util.BoolPointer(false)
		}
		if fluentbitSpec.ConfigHotReload != nil {
			if fluentbitSpec.ConfigHotReload.Image.Repository == "" {
				fluentbitSpec.ConfigHotReload.Image.Repository = DefaultFluentbitConfigReloaderImageRepository
			}
			if fluentbitSpec.ConfigHotReload.Image.Tag == "" {
				fluentbitSpec.ConfigHotReload.Image.Tag = DefaultFluentbitConfigReloaderImageTag
			}
			if fluentbitSpec.ConfigHotReload.Image.PullPolicy == "" {
				fluentbitSpec.ConfigHotReload.Image.PullPolicy = "IfNotPresent"
			}
		}
	}
	return nil
}

// SetDefaultsOnCopy makes a deep copy of the instance and sets defaults on the copy
func (l *Logging) SetDefaultsOnCopy() (*Logging, error) {
	if l == nil {
		return nil, nil
	}

	copy := l.DeepCopy()
	if err := copy.SetDefaults(); err != nil {
		return nil, err
	}
	return copy, nil
}

// QualifiedName is the "logging-resource" name combined
func (l *Logging) QualifiedName(name string) string {
	return fmt.Sprintf("%s-%s", l.Name, name)
}

// ClusterDomainAsSuffix formats the cluster domain as a suffix, e.g.:
// .Spec.ClusterDomain == "", returns ""
// .Spec.ClusterDomain == "cluster.local.", returns ".cluster.local."
func (l *Logging) ClusterDomainAsSuffix() string {
	if l.Spec.ClusterDomain == nil || *l.Spec.ClusterDomain == "" {
		return ""
	}
	return fmt.Sprintf(".%s", *l.Spec.ClusterDomain)
}

func init() {
	SchemeBuilder.Register(&Logging{}, &LoggingList{})
}

func persistentVolumeModePointer(mode v1.PersistentVolumeMode) *v1.PersistentVolumeMode {
	return &mode
}

// FluentdObjectMeta creates an objectMeta for resource fluentd
func (l *Logging) FluentdObjectMeta(name, component string, f FluentdSpec, fc *FluentdConfig) metav1.ObjectMeta {
	ownerReference := metav1.OwnerReference{
		APIVersion: l.APIVersion,
		Kind:       l.Kind,
		Name:       l.Name,
		UID:        l.UID,
		Controller: util.BoolPointer(true),
	}

	if fc != nil {
		ownerReference = metav1.OwnerReference{
			APIVersion: fc.APIVersion,
			Kind:       fc.Kind,
			Name:       fc.Name,
			UID:        fc.UID,
			Controller: util.BoolPointer(true),
		}
	}
	o := metav1.ObjectMeta{
		Name:            l.QualifiedName(name),
		Namespace:       l.Spec.ControlNamespace,
		Labels:          l.GetFluentdLabels(component, f),
		OwnerReferences: []metav1.OwnerReference{ownerReference},
	}
	return o
}

func (l *Logging) GetFluentdLabels(component string, f FluentdSpec) map[string]string {
	return util.MergeLabels(
		f.Labels,
		map[string]string{
			"app.kubernetes.io/name":      "fluentd",
			"app.kubernetes.io/component": component,
		},
		GenerateLoggingRefLabels(l.ObjectMeta.GetName()),
	)
}

// SyslogNGObjectMeta creates an objectMeta for resource syslog-ng
func (l *Logging) SyslogNGObjectMeta(name, component string, sc *SyslogNGConfig) metav1.ObjectMeta {
	ownerReference := metav1.OwnerReference{
		APIVersion: l.APIVersion,
		Kind:       l.Kind,
		Name:       l.Name,
		UID:        l.UID,
		Controller: util.BoolPointer(true),
	}
	if sc != nil {
		ownerReference = metav1.OwnerReference{
			APIVersion: sc.APIVersion,
			Kind:       sc.Kind,
			Name:       sc.Name,
			UID:        sc.UID,
			Controller: util.BoolPointer(true),
		}
	}
	o := metav1.ObjectMeta{
		Name:            l.QualifiedName(name),
		Namespace:       l.Spec.ControlNamespace,
		Labels:          l.GetSyslogNGLabels(component),
		OwnerReferences: []metav1.OwnerReference{ownerReference},
	}
	return o
}

func (l *Logging) GetSyslogNGLabels(component string) map[string]string {
	return util.MergeLabels(
		map[string]string{
			"app.kubernetes.io/name":      "syslog-ng",
			"app.kubernetes.io/component": component,
		},
		GenerateLoggingRefLabels(l.ObjectMeta.GetName()),
	)
}

func GenerateLoggingRefLabels(loggingRef string) map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": loggingRef}
}

func (l *Logging) AreMultipleAggregatorsSet() bool {
	return (l.Spec.SyslogNGSpec != nil || len(l.Status.SyslogNGConfigName) != 0) &&
		(l.Spec.FluentdSpec != nil || len(l.Status.FluentdConfigName) != 0)
}
