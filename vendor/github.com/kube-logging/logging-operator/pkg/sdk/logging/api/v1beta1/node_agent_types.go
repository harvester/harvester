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
	"github.com/cisco-open/operator-tools/pkg/types"
	"github.com/cisco-open/operator-tools/pkg/volume"
)

// +name:"NodeAgent"
// +weight:"200"
type _hugoNodeAgent interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true

type NodeAgent struct {
	//NodeAgent unique name.
	Name string `json:"name,omitempty"`
	// Specify the Logging-Operator nodeAgents profile. It can be linux or windows . (default:linux)
	Profile       string              `json:"profile,omitempty"`
	Metadata      types.MetaBase      `json:"metadata,omitempty"`
	FluentbitSpec *NodeAgentFluentbit `json:"nodeAgentFluentbit,omitempty"`
}

// +kubebuilder:object:generate=true

type NodeAgentFluentbit struct {
	Enabled                 *bool                        `json:"enabled,omitempty"`
	DaemonSetOverrides      *typeoverride.DaemonSet      `json:"daemonSet,omitempty"`
	ServiceAccountOverrides *typeoverride.ServiceAccount `json:"serviceAccount,omitempty"`
	TLS                     *FluentbitTLS                `json:"tls,omitempty"`
	TargetHost              string                       `json:"targetHost,omitempty"`
	TargetPort              int32                        `json:"targetPort,omitempty"`
	// Set the flush time in seconds.nanoseconds. The engine loop uses a Flush timeout to define when is required to flush the records ingested by input plugins through the defined output plugins. (default: 1)
	Flush int32 `json:"flush,omitempty"  plugin:"default:1"`
	// Set the grace time in seconds as Integer value. The engine loop uses a Grace timeout to define wait time on exit (default: 5)
	Grace int32 `json:"grace,omitempty" plugin:"default:5"`
	// Set the logging verbosity level. Allowed values are: error, warn, info, debug and trace. Values are accumulative, e.g: if 'debug' is set, it will include error, warning, info and debug.  Note that trace mode is only available if Fluent Bit was built with the WITH_TRACE option enabled. (default: info)
	LogLevel string `json:"logLevel,omitempty" plugin:"default:info"`
	// Set the coroutines stack size in bytes. The value must be greater than the page size of the running system. Don't set too small value (say 4096), or coroutine threads can overrun the stack buffer.
	//Do not change the default value of this parameter unless you know what you are doing. (default: 24576)
	CoroStackSize  int32                 `json:"coroStackSize,omitempty" plugin:"default:24576"`
	Metrics        *Metrics              `json:"metrics,omitempty"`
	MetricsService *typeoverride.Service `json:"metricsService,omitempty"`
	Security       *Security             `json:"security,omitempty"`
	// +docLink:"volume.KubernetesVolume,https://github.com/cisco-open/operator-tools/tree/master/docs/types"
	PositionDB              volume.KubernetesVolume `json:"positiondb,omitempty"`
	ContainersPath          string                  `json:"containersPath,omitempty"`
	VarLogsPath             string                  `json:"varLogsPath,omitempty"`
	ExtraVolumeMounts       []*VolumeMount          `json:"extraVolumeMounts,omitempty"`
	InputTail               InputTail               `json:"inputTail,omitempty"`
	FilterAws               *FilterAws              `json:"filterAws,omitempty"`
	FilterKubernetes        FilterKubernetes        `json:"filterKubernetes,omitempty"`
	DisableKubernetesFilter *bool                   `json:"disableKubernetesFilter,omitempty"`
	BufferStorage           BufferStorage           `json:"bufferStorage,omitempty"`
	// +docLink:"volume.KubernetesVolume,https://github.com/cisco-open/operator-tools/tree/master/docs/types"
	BufferStorageVolume  volume.KubernetesVolume `json:"bufferStorageVolume,omitempty"`
	CustomConfigSecret   string                  `json:"customConfigSecret,omitempty"`
	PodPriorityClassName string                  `json:"podPriorityClassName,omitempty"`
	LivenessDefaultCheck *bool                   `json:"livenessDefaultCheck,omitempty" plugin:"default:true"`
	Network              *FluentbitNetwork       `json:"network,omitempty"`
	ForwardOptions       *ForwardOptions         `json:"forwardOptions,omitempty"`
	EnableUpstream       *bool                   `json:"enableUpstream,omitempty"`
}
