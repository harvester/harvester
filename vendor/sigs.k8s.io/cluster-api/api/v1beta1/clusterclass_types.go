/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ClusterClassKind represents the Kind of ClusterClass.
const ClusterClassKind = "ClusterClass"

// ClusterClass VariablesReady condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// ClusterClassVariablesReadyV1Beta2Condition is true if the ClusterClass variables, including both inline and external
	// variables, have been successfully reconciled and thus ready to be used to default and validate variables on Clusters using
	// this ClusterClass.
	ClusterClassVariablesReadyV1Beta2Condition = "VariablesReady"

	// ClusterClassVariablesReadyV1Beta2Reason surfaces that the variables are ready.
	ClusterClassVariablesReadyV1Beta2Reason = "VariablesReady"

	// ClusterClassVariablesReadyVariableDiscoveryFailedV1Beta2Reason surfaces that variable discovery failed.
	ClusterClassVariablesReadyVariableDiscoveryFailedV1Beta2Reason = "VariableDiscoveryFailed"
)

// ClusterClass RefVersionsUpToDate condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// ClusterClassRefVersionsUpToDateV1Beta2Condition documents if the references in the ClusterClass are
	// up-to-date (i.e. they are using the latest apiVersion of the current Cluster API contract from
	// the corresponding CRD).
	ClusterClassRefVersionsUpToDateV1Beta2Condition = "RefVersionsUpToDate"

	// ClusterClassRefVersionsUpToDateV1Beta2Reason surfaces that the references in the ClusterClass are
	// up-to-date (i.e. they are using the latest apiVersion of the current Cluster API contract from
	// the corresponding CRD).
	ClusterClassRefVersionsUpToDateV1Beta2Reason = "RefVersionsUpToDate"

	// ClusterClassRefVersionsNotUpToDateV1Beta2Reason surfaces that the references in the ClusterClass are not
	// up-to-date (i.e. they are not using the latest apiVersion of the current Cluster API contract from
	// the corresponding CRD).
	ClusterClassRefVersionsNotUpToDateV1Beta2Reason = "RefVersionsNotUpToDate"

	// ClusterClassRefVersionsUpToDateInternalErrorV1Beta2Reason surfaces that an unexpected error occurred when validating
	// if the references are up-to-date.
	ClusterClassRefVersionsUpToDateInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clusterclasses,shortName=cc,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of ClusterClass"

// ClusterClass is a template which can be used to create managed topologies.
type ClusterClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterClassSpec   `json:"spec,omitempty"`
	Status ClusterClassStatus `json:"status,omitempty"`
}

// ClusterClassSpec describes the desired state of the ClusterClass.
type ClusterClassSpec struct {
	// infrastructure is a reference to a provider-specific template that holds
	// the details for provisioning infrastructure specific cluster
	// for the underlying provider.
	// The underlying provider is responsible for the implementation
	// of the template to an infrastructure cluster.
	// +optional
	Infrastructure LocalObjectTemplate `json:"infrastructure,omitempty"`

	// controlPlane is a reference to a local struct that holds the details
	// for provisioning the Control Plane for the Cluster.
	// +optional
	ControlPlane ControlPlaneClass `json:"controlPlane,omitempty"`

	// workers describes the worker nodes for the cluster.
	// It is a collection of node types which can be used to create
	// the worker nodes of the cluster.
	// +optional
	Workers WorkersClass `json:"workers,omitempty"`

	// variables defines the variables which can be configured
	// in the Cluster topology and are then used in patches.
	// +optional
	Variables []ClusterClassVariable `json:"variables,omitempty"`

	// patches defines the patches which are applied to customize
	// referenced templates of a ClusterClass.
	// Note: Patches will be applied in the order of the array.
	// +optional
	Patches []ClusterClassPatch `json:"patches,omitempty"`
}

// ControlPlaneClass defines the class for the control plane.
type ControlPlaneClass struct {
	// metadata is the metadata applied to the ControlPlane and the Machines of the ControlPlane
	// if the ControlPlaneTemplate referenced is machine based. If not, it is applied only to the
	// ControlPlane.
	// At runtime this metadata is merged with the corresponding metadata from the topology.
	//
	// This field is supported if and only if the control plane provider template
	// referenced is Machine based.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// LocalObjectTemplate contains the reference to the control plane provider.
	LocalObjectTemplate `json:",inline"`

	// machineInfrastructure defines the metadata and infrastructure information
	// for control plane machines.
	//
	// This field is supported if and only if the control plane provider template
	// referenced above is Machine based and supports setting replicas.
	//
	// +optional
	MachineInfrastructure *LocalObjectTemplate `json:"machineInfrastructure,omitempty"`

	// machineHealthCheck defines a MachineHealthCheck for this ControlPlaneClass.
	// This field is supported if and only if the ControlPlane provider template
	// referenced above is Machine based and supports setting replicas.
	// +optional
	MachineHealthCheck *MachineHealthCheckClass `json:"machineHealthCheck,omitempty"`

	// namingStrategy allows changing the naming pattern used when creating the control plane provider object.
	// +optional
	NamingStrategy *ControlPlaneClassNamingStrategy `json:"namingStrategy,omitempty"`

	// nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// NOTE: This value can be overridden while defining a Cluster.Topology.
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// NOTE: This value can be overridden while defining a Cluster.Topology.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// NOTE: This value can be overridden while defining a Cluster.Topology.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`
}

// ControlPlaneClassNamingStrategy defines the naming strategy for control plane objects.
type ControlPlaneClassNamingStrategy struct {
	// template defines the template to use for generating the name of the ControlPlane object.
	// If not defined, it will fallback to `{{ .cluster.name }}-{{ .random }}`.
	// If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will
	// get concatenated with a random suffix of length 5.
	// The templating mechanism provides the following arguments:
	// * `.cluster.name`: The name of the cluster object.
	// * `.random`: A random alphanumeric string, without vowels, of length 5.
	// +optional
	Template *string `json:"template,omitempty"`
}

// WorkersClass is a collection of deployment classes.
type WorkersClass struct {
	// machineDeployments is a list of machine deployment classes that can be used to create
	// a set of worker nodes.
	// +optional
	// +listType=map
	// +listMapKey=class
	MachineDeployments []MachineDeploymentClass `json:"machineDeployments,omitempty"`

	// machinePools is a list of machine pool classes that can be used to create
	// a set of worker nodes.
	// +optional
	// +listType=map
	// +listMapKey=class
	MachinePools []MachinePoolClass `json:"machinePools,omitempty"`
}

// MachineDeploymentClass serves as a template to define a set of worker nodes of the cluster
// provisioned using the `ClusterClass`.
type MachineDeploymentClass struct {
	// class denotes a type of worker node present in the cluster,
	// this name MUST be unique within a ClusterClass and can be referenced
	// in the Cluster to create a managed MachineDeployment.
	Class string `json:"class"`

	// template is a local struct containing a collection of templates for creation of
	// MachineDeployment objects representing a set of worker nodes.
	Template MachineDeploymentClassTemplate `json:"template"`

	// machineHealthCheck defines a MachineHealthCheck for this MachineDeploymentClass.
	// +optional
	MachineHealthCheck *MachineHealthCheckClass `json:"machineHealthCheck,omitempty"`

	// failureDomain is the failure domain the machines will be created in.
	// Must match a key in the FailureDomains map stored on the cluster object.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	// +optional
	FailureDomain *string `json:"failureDomain,omitempty"`

	// namingStrategy allows changing the naming pattern used when creating the MachineDeployment.
	// +optional
	NamingStrategy *MachineDeploymentClassNamingStrategy `json:"namingStrategy,omitempty"`

	// nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`

	// Minimum number of seconds for which a newly created machine should
	// be ready.
	// Defaults to 0 (machine will be considered available as soon as it
	// is ready)
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// The deployment strategy to use to replace existing machines with
	// new ones.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	Strategy *MachineDeploymentStrategy `json:"strategy,omitempty"`
}

// MachineDeploymentClassTemplate defines how a MachineDeployment generated from a MachineDeploymentClass
// should look like.
type MachineDeploymentClassTemplate struct {
	// metadata is the metadata applied to the MachineDeployment and the machines of the MachineDeployment.
	// At runtime this metadata is merged with the corresponding metadata from the topology.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// bootstrap contains the bootstrap template reference to be used
	// for the creation of worker Machines.
	Bootstrap LocalObjectTemplate `json:"bootstrap"`

	// infrastructure contains the infrastructure template reference to be used
	// for the creation of worker Machines.
	Infrastructure LocalObjectTemplate `json:"infrastructure"`
}

// MachineDeploymentClassNamingStrategy defines the naming strategy for machine deployment objects.
type MachineDeploymentClassNamingStrategy struct {
	// template defines the template to use for generating the name of the MachineDeployment object.
	// If not defined, it will fallback to `{{ .cluster.name }}-{{ .machineDeployment.topologyName }}-{{ .random }}`.
	// If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will
	// get concatenated with a random suffix of length 5.
	// The templating mechanism provides the following arguments:
	// * `.cluster.name`: The name of the cluster object.
	// * `.random`: A random alphanumeric string, without vowels, of length 5.
	// * `.machineDeployment.topologyName`: The name of the MachineDeployment topology (Cluster.spec.topology.workers.machineDeployments[].name).
	// +optional
	Template *string `json:"template,omitempty"`
}

// MachineHealthCheckClass defines a MachineHealthCheck for a group of Machines.
type MachineHealthCheckClass struct {
	// unhealthyConditions contains a list of the conditions that determine
	// whether a node is considered unhealthy. The conditions are combined in a
	// logical OR, i.e. if any of the conditions is met, the node is unhealthy.
	//
	// +optional
	UnhealthyConditions []UnhealthyCondition `json:"unhealthyConditions,omitempty"`

	// Any further remediation is only allowed if at most "MaxUnhealthy" machines selected by
	// "selector" are not healthy.
	// +optional
	MaxUnhealthy *intstr.IntOrString `json:"maxUnhealthy,omitempty"`

	// Any further remediation is only allowed if the number of machines selected by "selector" as not healthy
	// is within the range of "UnhealthyRange". Takes precedence over MaxUnhealthy.
	// Eg. "[3-5]" - This means that remediation will be allowed only when:
	// (a) there are at least 3 unhealthy machines (and)
	// (b) there are at most 5 unhealthy machines
	// +optional
	// +kubebuilder:validation:Pattern=^\[[0-9]+-[0-9]+\]$
	UnhealthyRange *string `json:"unhealthyRange,omitempty"`

	// nodeStartupTimeout allows to set the maximum time for MachineHealthCheck
	// to consider a Machine unhealthy if a corresponding Node isn't associated
	// through a `Spec.ProviderID` field.
	//
	// The duration set in this field is compared to the greatest of:
	// - Cluster's infrastructure ready condition timestamp (if and when available)
	// - Control Plane's initialized condition timestamp (if and when available)
	// - Machine's infrastructure ready condition timestamp (if and when available)
	// - Machine's metadata creation timestamp
	//
	// Defaults to 10 minutes.
	// If you wish to disable this feature, set the value explicitly to 0.
	// +optional
	NodeStartupTimeout *metav1.Duration `json:"nodeStartupTimeout,omitempty"`

	// remediationTemplate is a reference to a remediation template
	// provided by an infrastructure provider.
	//
	// This field is completely optional, when filled, the MachineHealthCheck controller
	// creates a new object from the template referenced and hands off remediation of the machine to
	// a controller that lives outside of Cluster API.
	// +optional
	RemediationTemplate *corev1.ObjectReference `json:"remediationTemplate,omitempty"`
}

// MachinePoolClass serves as a template to define a pool of worker nodes of the cluster
// provisioned using `ClusterClass`.
type MachinePoolClass struct {
	// class denotes a type of machine pool present in the cluster,
	// this name MUST be unique within a ClusterClass and can be referenced
	// in the Cluster to create a managed MachinePool.
	Class string `json:"class"`

	// template is a local struct containing a collection of templates for creation of
	// MachinePools objects representing a pool of worker nodes.
	Template MachinePoolClassTemplate `json:"template"`

	// failureDomains is the list of failure domains the MachinePool should be attached to.
	// Must match a key in the FailureDomains map stored on the cluster object.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass.
	// +optional
	FailureDomains []string `json:"failureDomains,omitempty"`

	// namingStrategy allows changing the naming pattern used when creating the MachinePool.
	// +optional
	NamingStrategy *MachinePoolClassNamingStrategy `json:"namingStrategy,omitempty"`

	// nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass.
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine Pool is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`

	// Minimum number of seconds for which a newly created machine pool should
	// be ready.
	// Defaults to 0 (machine will be considered available as soon as it
	// is ready)
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass.
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`
}

// MachinePoolClassTemplate defines how a MachinePool generated from a MachinePoolClass
// should look like.
type MachinePoolClassTemplate struct {
	// metadata is the metadata applied to the MachinePool.
	// At runtime this metadata is merged with the corresponding metadata from the topology.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// bootstrap contains the bootstrap template reference to be used
	// for the creation of the Machines in the MachinePool.
	Bootstrap LocalObjectTemplate `json:"bootstrap"`

	// infrastructure contains the infrastructure template reference to be used
	// for the creation of the MachinePool.
	Infrastructure LocalObjectTemplate `json:"infrastructure"`
}

// MachinePoolClassNamingStrategy defines the naming strategy for machine pool objects.
type MachinePoolClassNamingStrategy struct {
	// template defines the template to use for generating the name of the MachinePool object.
	// If not defined, it will fallback to `{{ .cluster.name }}-{{ .machinePool.topologyName }}-{{ .random }}`.
	// If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will
	// get concatenated with a random suffix of length 5.
	// The templating mechanism provides the following arguments:
	// * `.cluster.name`: The name of the cluster object.
	// * `.random`: A random alphanumeric string, without vowels, of length 5.
	// * `.machinePool.topologyName`: The name of the MachinePool topology (Cluster.spec.topology.workers.machinePools[].name).
	// +optional
	Template *string `json:"template,omitempty"`
}

// IsZero returns true if none of the values of MachineHealthCheckClass are defined.
func (m MachineHealthCheckClass) IsZero() bool {
	return reflect.ValueOf(m).IsZero()
}

// ClusterClassVariable defines a variable which can
// be configured in the Cluster topology and used in patches.
type ClusterClassVariable struct {
	// name of the variable.
	Name string `json:"name"`

	// required specifies if the variable is required.
	// Note: this applies to the variable as a whole and thus the
	// top-level object defined in the schema. If nested fields are
	// required, this will be specified inside the schema.
	Required bool `json:"required"`

	// metadata is the metadata of a variable.
	// It can be used to add additional data for higher level tools to
	// a ClusterClassVariable.
	//
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please use XMetadata in JSONSchemaProps instead.
	//
	// +optional
	Metadata ClusterClassVariableMetadata `json:"metadata,omitempty"`

	// schema defines the schema of the variable.
	Schema VariableSchema `json:"schema"`
}

// ClusterClassVariableMetadata is the metadata of a variable.
// It can be used to add additional data for higher level tools to
// a ClusterClassVariable.
//
// Deprecated: This struct is deprecated and is going to be removed in the next apiVersion.
type ClusterClassVariableMetadata struct {
	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) variables.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations is an unstructured key value map that can be used to store and
	// retrieve arbitrary metadata.
	// They are not queryable.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// VariableSchema defines the schema of a variable.
type VariableSchema struct {
	// openAPIV3Schema defines the schema of a variable via OpenAPI v3
	// schema. The schema is a subset of the schema used in
	// Kubernetes CRDs.
	OpenAPIV3Schema JSONSchemaProps `json:"openAPIV3Schema"`
}

// Adapted from https://github.com/kubernetes/apiextensions-apiserver/blob/v0.28.5/pkg/apis/apiextensions/v1/types_jsonschema.go#L40

// JSONSchemaProps is a JSON-Schema following Specification Draft 4 (http://json-schema.org/).
// This struct has been initially copied from apiextensionsv1.JSONSchemaProps, but all fields
// which are not supported in CAPI have been removed.
type JSONSchemaProps struct {
	// description is a human-readable description of this variable.
	Description string `json:"description,omitempty"`

	// example is an example for this variable.
	Example *apiextensionsv1.JSON `json:"example,omitempty"`

	// type is the type of the variable.
	// Valid values are: object, array, string, integer, number or boolean.
	// +optional
	Type string `json:"type,omitempty"`

	// properties specifies fields of an object.
	// NOTE: Can only be set if type is object.
	// NOTE: Properties is mutually exclusive with AdditionalProperties.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Properties map[string]JSONSchemaProps `json:"properties,omitempty"`

	// additionalProperties specifies the schema of values in a map (keys are always strings).
	// NOTE: Can only be set if type is object.
	// NOTE: AdditionalProperties is mutually exclusive with Properties.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AdditionalProperties *JSONSchemaProps `json:"additionalProperties,omitempty"`

	// maxProperties is the maximum amount of entries in a map or properties in an object.
	// NOTE: Can only be set if type is object.
	// +optional
	MaxProperties *int64 `json:"maxProperties,omitempty"`

	// minProperties is the minimum amount of entries in a map or properties in an object.
	// NOTE: Can only be set if type is object.
	// +optional
	MinProperties *int64 `json:"minProperties,omitempty"`

	// required specifies which fields of an object are required.
	// NOTE: Can only be set if type is object.
	// +optional
	Required []string `json:"required,omitempty"`

	// items specifies fields of an array.
	// NOTE: Can only be set if type is array.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Items *JSONSchemaProps `json:"items,omitempty"`

	// maxItems is the max length of an array variable.
	// NOTE: Can only be set if type is array.
	// +optional
	MaxItems *int64 `json:"maxItems,omitempty"`

	// minItems is the min length of an array variable.
	// NOTE: Can only be set if type is array.
	// +optional
	MinItems *int64 `json:"minItems,omitempty"`

	// uniqueItems specifies if items in an array must be unique.
	// NOTE: Can only be set if type is array.
	// +optional
	UniqueItems bool `json:"uniqueItems,omitempty"`

	// format is an OpenAPI v3 format string. Unknown formats are ignored.
	// For a list of supported formats please see: (of the k8s.io/apiextensions-apiserver version we're currently using)
	// https://github.com/kubernetes/apiextensions-apiserver/blob/master/pkg/apiserver/validation/formats.go
	// NOTE: Can only be set if type is string.
	// +optional
	Format string `json:"format,omitempty"`

	// maxLength is the max length of a string variable.
	// NOTE: Can only be set if type is string.
	// +optional
	MaxLength *int64 `json:"maxLength,omitempty"`

	// minLength is the min length of a string variable.
	// NOTE: Can only be set if type is string.
	// +optional
	MinLength *int64 `json:"minLength,omitempty"`

	// pattern is the regex which a string variable must match.
	// NOTE: Can only be set if type is string.
	// +optional
	Pattern string `json:"pattern,omitempty"`

	// maximum is the maximum of an integer or number variable.
	// If ExclusiveMaximum is false, the variable is valid if it is lower than, or equal to, the value of Maximum.
	// If ExclusiveMaximum is true, the variable is valid if it is strictly lower than the value of Maximum.
	// NOTE: Can only be set if type is integer or number.
	// +optional
	Maximum *int64 `json:"maximum,omitempty"`

	// exclusiveMaximum specifies if the Maximum is exclusive.
	// NOTE: Can only be set if type is integer or number.
	// +optional
	ExclusiveMaximum bool `json:"exclusiveMaximum,omitempty"`

	// minimum is the minimum of an integer or number variable.
	// If ExclusiveMinimum is false, the variable is valid if it is greater than, or equal to, the value of Minimum.
	// If ExclusiveMinimum is true, the variable is valid if it is strictly greater than the value of Minimum.
	// NOTE: Can only be set if type is integer or number.
	// +optional
	Minimum *int64 `json:"minimum,omitempty"`

	// exclusiveMinimum specifies if the Minimum is exclusive.
	// NOTE: Can only be set if type is integer or number.
	// +optional
	ExclusiveMinimum bool `json:"exclusiveMinimum,omitempty"`

	// x-kubernetes-preserve-unknown-fields allows setting fields in a variable object
	// which are not defined in the variable schema. This affects fields recursively,
	// except if nested properties or additionalProperties are specified in the schema.
	// +optional
	XPreserveUnknownFields bool `json:"x-kubernetes-preserve-unknown-fields,omitempty"`

	// enum is the list of valid values of the variable.
	// NOTE: Can be set for all types.
	// +optional
	Enum []apiextensionsv1.JSON `json:"enum,omitempty"`

	// default is the default value of the variable.
	// NOTE: Can be set for all types.
	// +optional
	Default *apiextensionsv1.JSON `json:"default,omitempty"`

	// x-kubernetes-validations describes a list of validation rules written in the CEL expression language.
	// +optional
	// +listType=map
	// +listMapKey=rule
	XValidations []ValidationRule `json:"x-kubernetes-validations,omitempty"`

	// x-metadata is the metadata of a variable or a nested field within a variable.
	// It can be used to add additional data for higher level tools.
	// +optional
	XMetadata *VariableSchemaMetadata `json:"x-metadata,omitempty"`

	// x-kubernetes-int-or-string specifies that this value is
	// either an integer or a string. If this is true, an empty
	// type is allowed and type as child of anyOf is permitted
	// if following one of the following patterns:
	//
	// 1) anyOf:
	//    - type: integer
	//    - type: string
	// 2) allOf:
	//    - anyOf:
	//      - type: integer
	//      - type: string
	//    - ... zero or more
	// +optional
	XIntOrString bool `json:"x-kubernetes-int-or-string,omitempty"`

	// allOf specifies that the variable must validate against all of the subschemas in the array.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AllOf []JSONSchemaProps `json:"allOf,omitempty"`

	// oneOf specifies that the variable must validate against exactly one of the subschemas in the array.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	OneOf []JSONSchemaProps `json:"oneOf,omitempty"`

	// anyOf specifies that the variable must validate against one or more of the subschemas in the array.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AnyOf []JSONSchemaProps `json:"anyOf,omitempty"`

	// not specifies that the variable must not validate against the subschema.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Not *JSONSchemaProps `json:"not,omitempty"`
}

// VariableSchemaMetadata is the metadata of a variable or a nested field within a variable.
// It can be used to add additional data for higher level tools.
type VariableSchemaMetadata struct {
	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) variables.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations is an unstructured key value map that can be used to store and
	// retrieve arbitrary metadata.
	// They are not queryable.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ValidationRule describes a validation rule written in the CEL expression language.
type ValidationRule struct {
	// rule represents the expression which will be evaluated by CEL.
	// ref: https://github.com/google/cel-spec
	// The Rule is scoped to the location of the x-kubernetes-validations extension in the schema.
	// The `self` variable in the CEL expression is bound to the scoped value.
	// If the Rule is scoped to an object with properties, the accessible properties of the object are field selectable
	// via `self.field` and field presence can be checked via `has(self.field)`.
	// If the Rule is scoped to an object with additionalProperties (i.e. a map) the value of the map
	// are accessible via `self[mapKey]`, map containment can be checked via `mapKey in self` and all entries of the map
	// are accessible via CEL macros and functions such as `self.all(...)`.
	// If the Rule is scoped to an array, the elements of the array are accessible via `self[i]` and also by macros and
	// functions.
	// If the Rule is scoped to a scalar, `self` is bound to the scalar value.
	// Examples:
	// - Rule scoped to a map of objects: {"rule": "self.components['Widget'].priority < 10"}
	// - Rule scoped to a list of integers: {"rule": "self.values.all(value, value >= 0 && value < 100)"}
	// - Rule scoped to a string value: {"rule": "self.startsWith('kube')"}
	//
	// Unknown data preserved in custom resources via x-kubernetes-preserve-unknown-fields is not accessible in CEL
	// expressions. This includes:
	// - Unknown field values that are preserved by object schemas with x-kubernetes-preserve-unknown-fields.
	// - Object properties where the property schema is of an "unknown type". An "unknown type" is recursively defined as:
	//   - A schema with no type and x-kubernetes-preserve-unknown-fields set to true
	//   - An array where the items schema is of an "unknown type"
	//   - An object where the additionalProperties schema is of an "unknown type"
	//
	// Only property names of the form `[a-zA-Z_.-/][a-zA-Z0-9_.-/]*` are accessible.
	// Accessible property names are escaped according to the following rules when accessed in the expression:
	// - '__' escapes to '__underscores__'
	// - '.' escapes to '__dot__'
	// - '-' escapes to '__dash__'
	// - '/' escapes to '__slash__'
	// - Property names that exactly match a CEL RESERVED keyword escape to '__{keyword}__'. The keywords are:
	//	  "true", "false", "null", "in", "as", "break", "const", "continue", "else", "for", "function", "if",
	//	  "import", "let", "loop", "package", "namespace", "return".
	// Examples:
	//   - Rule accessing a property named "namespace": {"rule": "self.__namespace__ > 0"}
	//   - Rule accessing a property named "x-prop": {"rule": "self.x__dash__prop > 0"}
	//   - Rule accessing a property named "redact__d": {"rule": "self.redact__underscores__d > 0"}
	//
	//
	// If `rule` makes use of the `oldSelf` variable it is implicitly a
	// `transition rule`.
	//
	// By default, the `oldSelf` variable is the same type as `self`.
	//
	// Transition rules by default are applied only on UPDATE requests and are
	// skipped if an old value could not be found.
	//
	// +required
	Rule string `json:"rule"`
	// message represents the message displayed when validation fails. The message is required if the Rule contains
	// line breaks. The message must not contain line breaks.
	// If unset, the message is "failed rule: {Rule}".
	// e.g. "must be a URL with the host matching spec.host"
	// +optional
	Message string `json:"message,omitempty"`
	// messageExpression declares a CEL expression that evaluates to the validation failure message that is returned when this rule fails.
	// Since messageExpression is used as a failure message, it must evaluate to a string.
	// If both message and messageExpression are present on a rule, then messageExpression will be used if validation
	// fails. If messageExpression results in a runtime error, the validation failure message is produced
	// as if the messageExpression field were unset. If messageExpression evaluates to an empty string, a string with only spaces, or a string
	// that contains line breaks, then the validation failure message will also be produced as if the messageExpression field were unset.
	// messageExpression has access to all the same variables as the rule; the only difference is the return type.
	// Example:
	// "x must be less than max ("+string(self.max)+")"
	// +optional
	MessageExpression string `json:"messageExpression,omitempty"`
	// reason provides a machine-readable validation failure reason that is returned to the caller when a request fails this validation rule.
	// The currently supported reasons are: "FieldValueInvalid", "FieldValueForbidden", "FieldValueRequired", "FieldValueDuplicate".
	// If not set, default to use "FieldValueInvalid".
	// All future added reasons must be accepted by clients when reading this value and unknown reasons should be treated as FieldValueInvalid.
	// +optional
	// +kubebuilder:validation:Enum=FieldValueInvalid;FieldValueForbidden;FieldValueRequired;FieldValueDuplicate
	// +kubebuilder:default=FieldValueInvalid
	// +default=ref(sigs.k8s.io/cluster-api/api/v1beta1.FieldValueInvalid)
	Reason FieldValueErrorReason `json:"reason,omitempty"`
	// fieldPath represents the field path returned when the validation fails.
	// It must be a relative JSON path (i.e. with array notation) scoped to the location of this x-kubernetes-validations extension in the schema and refer to an existing field.
	// e.g. when validation checks if a specific attribute `foo` under a map `testMap`, the fieldPath could be set to `.testMap.foo`
	// If the validation checks two lists must have unique attributes, the fieldPath could be set to either of the list: e.g. `.testList`
	// It does not support list numeric index.
	// It supports child operation to refer to an existing field currently. Refer to [JSONPath support in Kubernetes](https://kubernetes.io/docs/reference/kubectl/jsonpath/) for more info.
	// Numeric index of array is not supported.
	// For field name which contains special characters, use `['specialName']` to refer the field name.
	// e.g. for attribute `foo.34$` appears in a list `testList`, the fieldPath could be set to `.testList['foo.34$']`
	// +optional
	FieldPath string `json:"fieldPath,omitempty"`
}

// FieldValueErrorReason is a machine-readable value providing more detail about why a field failed the validation.
type FieldValueErrorReason string

const (
	// FieldValueRequired is used to report required values that are not
	// provided (e.g. empty strings, null values, or empty arrays).
	FieldValueRequired FieldValueErrorReason = "FieldValueRequired"
	// FieldValueDuplicate is used to report collisions of values that must be
	// unique (e.g. unique IDs).
	FieldValueDuplicate FieldValueErrorReason = "FieldValueDuplicate"
	// FieldValueInvalid is used to report malformed values (e.g. failed regex
	// match, too long, out of bounds).
	FieldValueInvalid FieldValueErrorReason = "FieldValueInvalid"
	// FieldValueForbidden is used to report valid (as per formatting rules)
	// values which would be accepted under some conditions, but which are not
	// permitted by the current conditions (such as security policy).
	FieldValueForbidden FieldValueErrorReason = "FieldValueForbidden"
)

// ClusterClassPatch defines a patch which is applied to customize the referenced templates.
type ClusterClassPatch struct {
	// name of the patch.
	Name string `json:"name"`

	// description is a human-readable description of this patch.
	Description string `json:"description,omitempty"`

	// enabledIf is a Go template to be used to calculate if a patch should be enabled.
	// It can reference variables defined in .spec.variables and builtin variables.
	// The patch will be enabled if the template evaluates to `true`, otherwise it will
	// be disabled.
	// If EnabledIf is not set, the patch will be enabled per default.
	// +optional
	EnabledIf *string `json:"enabledIf,omitempty"`

	// definitions define inline patches.
	// Note: Patches will be applied in the order of the array.
	// Note: Exactly one of Definitions or External must be set.
	// +optional
	Definitions []PatchDefinition `json:"definitions,omitempty"`

	// external defines an external patch.
	// Note: Exactly one of Definitions or External must be set.
	// +optional
	External *ExternalPatchDefinition `json:"external,omitempty"`
}

// PatchDefinition defines a patch which is applied to customize the referenced templates.
type PatchDefinition struct {
	// selector defines on which templates the patch should be applied.
	Selector PatchSelector `json:"selector"`

	// jsonPatches defines the patches which should be applied on the templates
	// matching the selector.
	// Note: Patches will be applied in the order of the array.
	JSONPatches []JSONPatch `json:"jsonPatches"`
}

// PatchSelector defines on which templates the patch should be applied.
// Note: Matching on APIVersion and Kind is mandatory, to enforce that the patches are
// written for the correct version. The version of the references in the ClusterClass may
// be automatically updated during reconciliation if there is a newer version for the same contract.
// Note: The results of selection based on the individual fields are ANDed.
type PatchSelector struct {
	// apiVersion filters templates by apiVersion.
	APIVersion string `json:"apiVersion"`

	// kind filters templates by kind.
	Kind string `json:"kind"`

	// matchResources selects templates based on where they are referenced.
	MatchResources PatchSelectorMatch `json:"matchResources"`
}

// PatchSelectorMatch selects templates based on where they are referenced.
// Note: The selector must match at least one template.
// Note: The results of selection based on the individual fields are ORed.
type PatchSelectorMatch struct {
	// controlPlane selects templates referenced in .spec.ControlPlane.
	// Note: this will match the controlPlane and also the controlPlane
	// machineInfrastructure (depending on the kind and apiVersion).
	// +optional
	ControlPlane bool `json:"controlPlane,omitempty"`

	// infrastructureCluster selects templates referenced in .spec.infrastructure.
	// +optional
	InfrastructureCluster bool `json:"infrastructureCluster,omitempty"`

	// machineDeploymentClass selects templates referenced in specific MachineDeploymentClasses in
	// .spec.workers.machineDeployments.
	// +optional
	MachineDeploymentClass *PatchSelectorMatchMachineDeploymentClass `json:"machineDeploymentClass,omitempty"`

	// machinePoolClass selects templates referenced in specific MachinePoolClasses in
	// .spec.workers.machinePools.
	// +optional
	MachinePoolClass *PatchSelectorMatchMachinePoolClass `json:"machinePoolClass,omitempty"`
}

// PatchSelectorMatchMachineDeploymentClass selects templates referenced
// in specific MachineDeploymentClasses in .spec.workers.machineDeployments.
type PatchSelectorMatchMachineDeploymentClass struct {
	// names selects templates by class names.
	// +optional
	Names []string `json:"names,omitempty"`
}

// PatchSelectorMatchMachinePoolClass selects templates referenced
// in specific MachinePoolClasses in .spec.workers.machinePools.
type PatchSelectorMatchMachinePoolClass struct {
	// names selects templates by class names.
	// +optional
	Names []string `json:"names,omitempty"`
}

// JSONPatch defines a JSON patch.
type JSONPatch struct {
	// op defines the operation of the patch.
	// Note: Only `add`, `replace` and `remove` are supported.
	Op string `json:"op"`

	// path defines the path of the patch.
	// Note: Only the spec of a template can be patched, thus the path has to start with /spec/.
	// Note: For now the only allowed array modifications are `append` and `prepend`, i.e.:
	// * for op: `add`: only index 0 (prepend) and - (append) are allowed
	// * for op: `replace` or `remove`: no indexes are allowed
	Path string `json:"path"`

	// value defines the value of the patch.
	// Note: Either Value or ValueFrom is required for add and replace
	// operations. Only one of them is allowed to be set at the same time.
	// Note: We have to use apiextensionsv1.JSON instead of our JSON type,
	// because controller-tools has a hard-coded schema for apiextensionsv1.JSON
	// which cannot be produced by another type (unset type field).
	// Ref: https://github.com/kubernetes-sigs/controller-tools/blob/d0e03a142d0ecdd5491593e941ee1d6b5d91dba6/pkg/crd/known_types.go#L106-L111
	// +optional
	Value *apiextensionsv1.JSON `json:"value,omitempty"`

	// valueFrom defines the value of the patch.
	// Note: Either Value or ValueFrom is required for add and replace
	// operations. Only one of them is allowed to be set at the same time.
	// +optional
	ValueFrom *JSONPatchValue `json:"valueFrom,omitempty"`
}

// JSONPatchValue defines the value of a patch.
// Note: Only one of the fields is allowed to be set at the same time.
type JSONPatchValue struct {
	// variable is the variable to be used as value.
	// Variable can be one of the variables defined in .spec.variables or a builtin variable.
	// +optional
	Variable *string `json:"variable,omitempty"`

	// template is the Go template to be used to calculate the value.
	// A template can reference variables defined in .spec.variables and builtin variables.
	// Note: The template must evaluate to a valid YAML or JSON value.
	// +optional
	Template *string `json:"template,omitempty"`
}

// ExternalPatchDefinition defines an external patch.
// Note: At least one of GenerateExtension or ValidateExtension must be set.
type ExternalPatchDefinition struct {
	// generateExtension references an extension which is called to generate patches.
	// +optional
	GenerateExtension *string `json:"generateExtension,omitempty"`

	// validateExtension references an extension which is called to validate the topology.
	// +optional
	ValidateExtension *string `json:"validateExtension,omitempty"`

	// discoverVariablesExtension references an extension which is called to discover variables.
	// +optional
	DiscoverVariablesExtension *string `json:"discoverVariablesExtension,omitempty"`

	// settings defines key value pairs to be passed to the extensions.
	// Values defined here take precedence over the values defined in the
	// corresponding ExtensionConfig.
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
}

// LocalObjectTemplate defines a template for a topology Class.
type LocalObjectTemplate struct {
	// ref is a required reference to a custom resource
	// offered by a provider.
	Ref *corev1.ObjectReference `json:"ref"`
}

// ANCHOR: ClusterClassStatus

// ClusterClassStatus defines the observed state of the ClusterClass.
type ClusterClassStatus struct {
	// variables is a list of ClusterClassStatusVariable that are defined for the ClusterClass.
	// +optional
	Variables []ClusterClassStatusVariable `json:"variables,omitempty"`

	// conditions defines current observed state of the ClusterClass.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in ClusterClass's status with the V1Beta2 version.
	// +optional
	V1Beta2 *ClusterClassV1Beta2Status `json:"v1beta2,omitempty"`
}

// ClusterClassV1Beta2Status groups all the fields that will be added or modified in ClusterClass with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ClusterClassV1Beta2Status struct {
	// conditions represents the observations of a ClusterClass's current state.
	// Known condition types are VariablesReady, RefVersionsUpToDate, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ClusterClassStatusVariable defines a variable which appears in the status of a ClusterClass.
type ClusterClassStatusVariable struct {
	// name is the name of the variable.
	Name string `json:"name"`

	// definitionsConflict specifies whether or not there are conflicting definitions for a single variable name.
	// +optional
	DefinitionsConflict bool `json:"definitionsConflict"`

	// definitions is a list of definitions for a variable.
	Definitions []ClusterClassStatusVariableDefinition `json:"definitions"`
}

// ClusterClassStatusVariableDefinition defines a variable which appears in the status of a ClusterClass.
type ClusterClassStatusVariableDefinition struct {
	// from specifies the origin of the variable definition.
	// This will be `inline` for variables defined in the ClusterClass or the name of a patch defined in the ClusterClass
	// for variables discovered from a DiscoverVariables runtime extensions.
	From string `json:"from"`

	// required specifies if the variable is required.
	// Note: this applies to the variable as a whole and thus the
	// top-level object defined in the schema. If nested fields are
	// required, this will be specified inside the schema.
	Required bool `json:"required"`

	// metadata is the metadata of a variable.
	// It can be used to add additional data for higher level tools to
	// a ClusterClassVariable.
	//
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion.
	//
	// +optional
	Metadata ClusterClassVariableMetadata `json:"metadata,omitempty"`

	// schema defines the schema of the variable.
	Schema VariableSchema `json:"schema"`
}

// GetConditions returns the set of conditions for this object.
func (c *ClusterClass) GetConditions() Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *ClusterClass) SetConditions(conditions Conditions) {
	c.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (c *ClusterClass) GetV1Beta2Conditions() []metav1.Condition {
	if c.Status.V1Beta2 == nil {
		return nil
	}
	return c.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (c *ClusterClass) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if c.Status.V1Beta2 == nil {
		c.Status.V1Beta2 = &ClusterClassV1Beta2Status{}
	}
	c.Status.V1Beta2.Conditions = conditions
}

// ANCHOR_END: ClusterClassStatus

// +kubebuilder:object:root=true

// ClusterClassList contains a list of Cluster.
type ClusterClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterClass `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ClusterClass{}, &ClusterClassList{})
}
