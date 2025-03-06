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

	"github.com/cisco-open/operator-tools/pkg/typeoverride"
	util "github.com/cisco-open/operator-tools/pkg/utils"
	"github.com/cisco-open/operator-tools/pkg/volume"
	"github.com/spf13/cast"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/input"
)

// +name:"FluentdSpec"
// +weight:"200"
type _hugoFluentdSpec interface{} //nolint:deadcode,unused

// +name:"FluentdSpec"
// +version:"v1beta1"
// +description:"FluentdSpec defines the desired state of Fluentd"
type _metaFluentdSpec interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true

// FluentdSpec defines the desired state of Fluentd
type FluentdSpec struct {
	StatefulSetAnnotations map[string]string `json:"statefulsetAnnotations,omitempty"`
	Annotations            map[string]string `json:"annotations,omitempty"`
	ConfigCheckAnnotations map[string]string `json:"configCheckAnnotations,omitempty"`
	Labels                 map[string]string `json:"labels,omitempty"`
	EnvVars                []corev1.EnvVar   `json:"envVars,omitempty"`
	TLS                    FluentdTLS        `json:"tls,omitempty"`
	Image                  ImageSpec         `json:"image,omitempty"`
	DisablePvc             bool              `json:"disablePvc,omitempty"`
	// BufferStorageVolume is by default configured as PVC using FluentdPvcSpec
	// +docLink:"volume.KubernetesVolume,https://github.com/cisco-open/operator-tools/tree/master/docs/types"
	BufferStorageVolume volume.KubernetesVolume `json:"bufferStorageVolume,omitempty"`
	ExtraVolumes        []ExtraVolume           `json:"extraVolumes,omitempty"`
	// Deprecated, use bufferStorageVolume
	FluentdPvcSpec          *volume.KubernetesVolume    `json:"fluentdPvcSpec,omitempty"`
	VolumeMountChmod        bool                        `json:"volumeMountChmod,omitempty"`
	VolumeModImage          ImageSpec                   `json:"volumeModImage,omitempty"`
	ConfigReloaderImage     ImageSpec                   `json:"configReloaderImage,omitempty"`
	Resources               corev1.ResourceRequirements `json:"resources,omitempty"`
	ConfigCheckResources    corev1.ResourceRequirements `json:"configCheckResources,omitempty"`
	ConfigReloaderResources corev1.ResourceRequirements `json:"configReloaderResources,omitempty"`
	LivenessProbe           *corev1.Probe               `json:"livenessProbe,omitempty"`
	LivenessDefaultCheck    bool                        `json:"livenessDefaultCheck,omitempty"`
	ReadinessProbe          *corev1.Probe               `json:"readinessProbe,omitempty"`
	ReadinessDefaultCheck   ReadinessDefaultCheck       `json:"readinessDefaultCheck,omitempty"`
	// Fluentd port inside the container (24240 by default).
	// The headless service port is controlled by this field as well.
	// Note that the default ClusterIP service port is always 24240, regardless of this field.
	Port                      int32                             `json:"port,omitempty"`
	Tolerations               []corev1.Toleration               `json:"tolerations,omitempty"`
	NodeSelector              map[string]string                 `json:"nodeSelector,omitempty"`
	Affinity                  *corev1.Affinity                  `json:"affinity,omitempty"`
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	Metrics                   *Metrics                          `json:"metrics,omitempty"`
	BufferVolumeMetrics       *Metrics                          `json:"bufferVolumeMetrics,omitempty"`
	BufferVolumeImage         ImageSpec                         `json:"bufferVolumeImage,omitempty"`
	BufferVolumeArgs          []string                          `json:"bufferVolumeArgs,omitempty"`
	BufferVolumeResources     corev1.ResourceRequirements       `json:"bufferVolumeResources,omitempty"`
	Security                  *Security                         `json:"security,omitempty"`
	Scaling                   *FluentdScaling                   `json:"scaling,omitempty"`
	Workers                   int32                             `json:"workers,omitempty"`
	RootDir                   string                            `json:"rootDir,omitempty"`
	// +kubebuilder:validation:enum=fatal,error,warn,info,debug,trace
	LogLevel string `json:"logLevel,omitempty"`
	// Ignore same log lines
	// +docLink:"more info, https://docs.fluentd.org/deployment/logging#ignore_same_log_interval"
	IgnoreSameLogInterval string `json:"ignoreSameLogInterval,omitempty"`
	// Ignore repeated log lines
	// +docLink:"more info, https://docs.fluentd.org/deployment/logging#ignore_repeated_log_interval"
	IgnoreRepeatedLogInterval string `json:"ignoreRepeatedLogInterval,omitempty"`
	// Allows Time object in buffer's MessagePack serde
	// +docLink:"more info, https://docs.fluentd.org/deployment/system-config#enable_msgpack_time_support"
	EnableMsgpackTimeSupport bool   `json:"enableMsgpackTimeSupport,omitempty"`
	PodPriorityClassName     string `json:"podPriorityClassName,omitempty"`
	// +kubebuilder:validation:enum=stdout,null
	FluentLogDestination string `json:"fluentLogDestination,omitempty"`
	// FluentOutLogrotate sends fluent's stdout to file and rotates it
	FluentOutLogrotate      *FluentOutLogrotate          `json:"fluentOutLogrotate,omitempty"`
	ForwardInputConfig      *input.ForwardInputConfig    `json:"forwardInputConfig,omitempty"`
	ServiceAccountOverrides *typeoverride.ServiceAccount `json:"serviceAccount,omitempty"`
	DNSPolicy               corev1.DNSPolicy             `json:"dnsPolicy,omitempty"`
	DNSConfig               *corev1.PodDNSConfig         `json:"dnsConfig,omitempty"`
	ExtraArgs               []string                     `json:"extraArgs,omitempty"`
	CompressConfigFile      bool                         `json:"compressConfigFile,omitempty"`
	Pdb                     *PdbInput                    `json:"pdb,omitempty"`
	// Available in Logging operator version 4.5 and later.
	// Configure sidecar container in Fluentd pods, for example: [https://github.com/kube-logging/logging-operator/config/samples/logging_logging_fluentd_sidecars.yaml](https://github.com/kube-logging/logging-operator/config/samples/logging_logging_fluentd_sidecars.yaml).
	SidecarContainers []corev1.Container `json:"sidecarContainers,omitempty"`
}

// +kubebuilder:object:generate=true

type FluentOutLogrotate struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path,omitempty"`
	Age     string `json:"age,omitempty"`
	Size    string `json:"size,omitempty"`
}

// GetFluentdMetricsPath returns the right Fluentd metrics endpoint
// depending on the number of workers and the user configuration
func (f *FluentdSpec) GetFluentdMetricsPath() string {
	if f.Metrics.Path == "" {
		if f.Workers > 1 {
			return "/aggregated_metrics"
		}
		return "/metrics"
	}
	return f.Metrics.Path
}

func (f *FluentdSpec) SetDefaults() error {
	if f != nil { // nolint:nestif
		if f.FluentdPvcSpec != nil {
			return errors.New("`fluentdPvcSpec` field is deprecated, use: `bufferStorageVolume`")
		}
		if f.Image.Repository == "" {
			f.Image.Repository = DefaultFluentdImageRepository
		}
		if f.Image.Tag == "" {
			f.Image.Tag = DefaultFluentdImageTag
		}
		if f.Image.PullPolicy == "" {
			f.Image.PullPolicy = "IfNotPresent"
		}
		if f.Annotations == nil {
			f.Annotations = make(map[string]string)
		}
		if f.Security == nil {
			f.Security = &Security{}
		}
		if f.Security.RoleBasedAccessControlCreate == nil {
			f.Security.RoleBasedAccessControlCreate = util.BoolPointer(true)
		}
		if f.Security.SecurityContext == nil {
			f.Security.SecurityContext = &corev1.SecurityContext{}
		}
		if f.Security.PodSecurityContext == nil {
			f.Security.PodSecurityContext = &corev1.PodSecurityContext{}
		}
		if f.Security.PodSecurityContext.FSGroup == nil {
			f.Security.PodSecurityContext.FSGroup = util.IntPointer64(101)
		}
		if f.Workers <= 0 {
			f.Workers = 1
		}
		if f.Metrics != nil {
			if f.Metrics.Port == 0 {
				f.Metrics.Port = 24231
			}
			if f.Metrics.Timeout == "" {
				f.Metrics.Timeout = "5s"
			}
			if f.Metrics.Interval == "" {
				f.Metrics.Interval = "15s"
			}
			if f.Metrics.PrometheusAnnotations {
				f.Annotations["prometheus.io/scrape"] = "true"
				f.Annotations["prometheus.io/path"] = f.GetFluentdMetricsPath()
				f.Annotations["prometheus.io/port"] = fmt.Sprintf("%d", f.Metrics.Port)
			}
		}
		if f.LogLevel == "" {
			f.LogLevel = "info"
		}
		if !f.DisablePvc {
			if f.BufferStorageVolume.PersistentVolumeClaim == nil {
				f.BufferStorageVolume.PersistentVolumeClaim = &volume.PersistentVolumeClaim{
					PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{},
				}
			}
			if f.BufferStorageVolume.PersistentVolumeClaim.PersistentVolumeClaimSpec.AccessModes == nil {
				f.BufferStorageVolume.PersistentVolumeClaim.PersistentVolumeClaimSpec.AccessModes = []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				}
			}
			if f.BufferStorageVolume.PersistentVolumeClaim.PersistentVolumeClaimSpec.Resources.Requests == nil {
				f.BufferStorageVolume.PersistentVolumeClaim.PersistentVolumeClaimSpec.Resources.Requests = map[corev1.ResourceName]resource.Quantity{
					"storage": resource.MustParse("20Gi"),
				}
			}
			if f.BufferStorageVolume.PersistentVolumeClaim.PersistentVolumeClaimSpec.VolumeMode == nil {
				f.BufferStorageVolume.PersistentVolumeClaim.PersistentVolumeClaimSpec.VolumeMode = persistentVolumeModePointer(corev1.PersistentVolumeFilesystem)
			}
			if f.BufferStorageVolume.PersistentVolumeClaim.PersistentVolumeSource.ClaimName == "" {
				f.BufferStorageVolume.PersistentVolumeClaim.PersistentVolumeSource.ClaimName = DefaultFluentdBufferStorageVolumeName
			}
		}
		if f.VolumeModImage.Repository == "" {
			f.VolumeModImage.Repository = DefaultFluentdVolumeModeImageRepository
		}
		if f.VolumeModImage.Tag == "" {
			f.VolumeModImage.Tag = DefaultFluentdVolumeModeImageTag
		}
		if f.VolumeModImage.PullPolicy == "" {
			f.VolumeModImage.PullPolicy = "IfNotPresent"
		}
		if f.ConfigReloaderImage.Repository == "" {
			f.ConfigReloaderImage.Repository = DefaultFluentdConfigReloaderImageRepository
		}
		if f.ConfigReloaderImage.Tag == "" {
			f.ConfigReloaderImage.Tag = DefaultFluentdConfigReloaderImageTag
		}
		if f.ConfigReloaderImage.PullPolicy == "" {
			f.ConfigReloaderImage.PullPolicy = "IfNotPresent"
		}
		if f.BufferVolumeImage.Repository == "" {
			f.BufferVolumeImage.Repository = DefaultFluentdBufferVolumeImageRepository
		}
		if f.BufferVolumeImage.Tag == "" {
			f.BufferVolumeImage.Tag = DefaultFluentdBufferVolumeImageTag
		}
		if f.BufferVolumeImage.PullPolicy == "" {
			f.BufferVolumeImage.PullPolicy = "IfNotPresent"
		}
		if f.BufferVolumeResources.Limits == nil {
			f.BufferVolumeResources.Limits = corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("10M"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			}
		}
		if f.BufferVolumeResources.Requests == nil {
			f.BufferVolumeResources.Requests = corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("10M"),
				corev1.ResourceCPU:    resource.MustParse("1m"),
			}
		}
		if f.Resources.Limits == nil {
			f.Resources.Limits = corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("400M"),
				corev1.ResourceCPU:    resource.MustParse("1000m"),
			}
		}
		if f.Resources.Requests == nil {
			f.Resources.Requests = corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("100M"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			}
		}
		if f.Port == 0 {
			f.Port = 24240
		}
		if f.Scaling == nil {
			f.Scaling = new(FluentdScaling)
		}
		if f.Scaling.PodManagementPolicy == "" {
			f.Scaling.PodManagementPolicy = "OrderedReady"
		}
		if f.Scaling.Drain.Image.Repository == "" {
			f.Scaling.Drain.Image.Repository = DefaultFluentdDrainWatchImageRepository
		}
		if f.Scaling.Drain.Image.Tag == "" {
			f.Scaling.Drain.Image.Tag = DefaultFluentdDrainWatchImageTag
		}
		if f.Scaling.Drain.Image.PullPolicy == "" {
			f.Scaling.Drain.Image.PullPolicy = "IfNotPresent"
		}
		if f.Scaling.Drain.PauseImage.Repository == "" {
			f.Scaling.Drain.PauseImage.Repository = DefaultFluentdDrainPauseImageRepository
		}
		if f.Scaling.Drain.PauseImage.Tag == "" {
			f.Scaling.Drain.PauseImage.Tag = DefaultFluentdDrainPauseImageTag
		}
		if f.Scaling.Drain.PauseImage.PullPolicy == "" {
			f.Scaling.Drain.PauseImage.PullPolicy = "IfNotPresent"
		}
		if f.Scaling.Drain.Resources == nil {
			f.Scaling.Drain.Resources = &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("50M"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("20m"),
				},
			}
		}
		if f.Scaling.Drain.SecurityContext == nil {
			f.Scaling.Drain.SecurityContext = f.Security.SecurityContext.DeepCopy()
		}
		if f.FluentLogDestination == "" {
			f.FluentLogDestination = "null"
		}
		if f.FluentOutLogrotate == nil {
			f.FluentOutLogrotate = &FluentOutLogrotate{
				Enabled: false,
			}
		}
		if _, ok := f.Annotations["fluentbit.io/exclude"]; !ok {
			f.Annotations["fluentbit.io/exclude"] = "true"
		}
		if f.FluentOutLogrotate.Path == "" {
			f.FluentOutLogrotate.Path = "/fluentd/log/out"
		}
		if f.FluentOutLogrotate.Age == "" {
			f.FluentOutLogrotate.Age = "10"
		}
		if f.FluentOutLogrotate.Size == "" {
			f.FluentOutLogrotate.Size = cast.ToString(1024 * 1024 * 10)
		}
		if f.LivenessProbe == nil {
			if f.LivenessDefaultCheck {
				f.LivenessProbe = &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						Exec: &corev1.ExecAction{Command: []string{"/bin/healthy.sh"}},
					},
					InitialDelaySeconds: 600,
					TimeoutSeconds:      0,
					PeriodSeconds:       60,
					SuccessThreshold:    0,
					FailureThreshold:    0,
				}
			}
		}
		if f.ReadinessDefaultCheck.BufferFreeSpace {
			if f.ReadinessDefaultCheck.BufferFreeSpaceThreshold == 0 {
				f.ReadinessDefaultCheck.BufferFreeSpaceThreshold = 90
			}
		}

		if f.ReadinessDefaultCheck.BufferFileNumber {
			if f.ReadinessDefaultCheck.BufferFileNumberMax == 0 {
				f.ReadinessDefaultCheck.BufferFileNumberMax = 5000
			}
		}
		if f.ReadinessDefaultCheck.InitialDelaySeconds == 0 {
			f.ReadinessDefaultCheck.InitialDelaySeconds = 5
		}
		if f.ReadinessDefaultCheck.TimeoutSeconds == 0 {
			f.ReadinessDefaultCheck.TimeoutSeconds = 3
		}
		if f.ReadinessDefaultCheck.PeriodSeconds == 0 {
			f.ReadinessDefaultCheck.PeriodSeconds = 30
		}
		if f.ReadinessDefaultCheck.SuccessThreshold == 0 {
			f.ReadinessDefaultCheck.SuccessThreshold = 3
		}
		if f.ReadinessDefaultCheck.FailureThreshold == 0 {
			f.ReadinessDefaultCheck.FailureThreshold = 1
		}
		for i := range f.ExtraVolumes {
			e := &f.ExtraVolumes[i]
			if e.ContainerName == "" {
				e.ContainerName = "fluentd"
			}
			if e.VolumeName == "" {
				e.VolumeName = fmt.Sprintf("extravolume-%d", i)
			}
			if e.Path == "" {
				e.Path = "/tmp"
			}
			if e.Volume == nil {
				e.Volume = &volume.KubernetesVolume{}
			}
		}
	}
	return nil
}

// +kubebuilder:object:generate=true

// ExtraVolume defines the fluentd extra volumes
type ExtraVolume struct {
	VolumeName    string                   `json:"volumeName,omitempty"`
	Path          string                   `json:"path,omitempty"`
	ContainerName string                   `json:"containerName,omitempty"`
	Volume        *volume.KubernetesVolume `json:"volume,omitempty"`
}

func (e *ExtraVolume) GetVolume() (corev1.Volume, error) {
	return e.Volume.GetVolume(e.VolumeName)
}

func (e *ExtraVolume) ApplyVolumeForPodSpec(spec *corev1.PodSpec) error {
	return e.Volume.ApplyVolumeForPodSpec(e.VolumeName, e.ContainerName, e.Path, spec)
}

// +kubebuilder:object:generate=true

// FluentdScaling enables configuring the scaling behaviour of the fluentd statefulset
type FluentdScaling struct {
	Replicas            int                `json:"replicas,omitempty"`
	PodManagementPolicy string             `json:"podManagementPolicy,omitempty"`
	Drain               FluentdDrainConfig `json:"drain,omitempty"`
}

// +kubebuilder:object:generate=true

// FluentdTLS defines the TLS configs
type FluentdTLS struct {
	Enabled    bool   `json:"enabled"`
	SecretName string `json:"secretName,omitempty"`
	SharedKey  string `json:"sharedKey,omitempty"`
}

// +kubebuilder:object:generate=true

// FluentdDrainConfig enables configuring the drain behavior when scaling down the fluentd statefulset
type FluentdDrainConfig struct {
	// Should buffers on persistent volumes left after scaling down the statefulset be drained
	Enabled bool `json:"enabled,omitempty"`
	// Annotations to use for the drain watch sidecar
	Annotations map[string]string `json:"annotations,omitempty"`
	// Labels to use for the drain watch sidecar on top of labels added by the operator by default. Default values can be overwritten.
	Labels map[string]string `json:"labels,omitempty"`
	// Should persistent volume claims be deleted after draining is done
	DeleteVolume bool      `json:"deleteVolume,omitempty"`
	Image        ImageSpec `json:"image,omitempty"`
	// Container image to use for the fluentd placeholder pod
	PauseImage ImageSpec `json:"pauseImage,omitempty"`
	// Available in Logging operator version 4.4 and later. Configurable resource requirements for the drainer sidecar container. Default 20m cpu request, 20M memory limit
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Available in Logging operator version 4.4 and later. Configurable security context, uses fluentd pods' security context by default
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

// +kubebuilder:object:generate=true

type PdbInput struct {
	MinAvailable               *intstr.IntOrString                      `json:"minAvailable,omitempty"`
	MaxUnavailable             *intstr.IntOrString                      `json:"maxUnavailable,omitempty"`
	UnhealthyPodEvictionPolicy *policyv1.UnhealthyPodEvictionPolicyType `json:"unhealthyPodEvictionPolicy,omitempty"`
}
