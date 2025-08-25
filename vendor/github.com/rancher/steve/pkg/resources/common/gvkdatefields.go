package common

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var DateFieldsByGVKBuiltins = map[schema.GroupVersionKind][]string{
	{Group: "", Version: "v1", Kind: "ConfigMap"}:             {"Age"},
	{Group: "", Version: "v1", Kind: "Endpoints"}:             {"Age"},
	{Group: "", Version: "v1", Kind: "Event"}:                 {"Last Seen", "First Seen"},
	{Group: "", Version: "v1", Kind: "Namespace"}:             {"Age"},
	{Group: "", Version: "v1", Kind: "Node"}:                  {"Age"},
	{Group: "", Version: "v1", Kind: "PersistentVolume"}:      {"Age"},
	{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}: {"Age"},
	{Group: "", Version: "v1", Kind: "Pod"}:                   {"Age"},
	{Group: "", Version: "v1", Kind: "ReplicationController"}: {"Age"},
	{Group: "", Version: "v1", Kind: "ResourceQuota"}:         {"Age"},
	{Group: "", Version: "v1", Kind: "Secret"}:                {"Age"},
	{Group: "", Version: "v1", Kind: "Service"}:               {"Age"},
	{Group: "", Version: "v1", Kind: "ServiceAccount"}:        {"Age"},

	{Group: "admissionregistration.k8s.io", Version: "v1alpha2", Kind: "MutatingAdmissionPolicy"}:          {"Age"},
	{Group: "admissionregistration.k8s.io", Version: "v1alpha2", Kind: "MutatingAdmissionPolicyBinding"}:   {"Age"},
	{Group: "admissionregistration.k8s.io", Version: "v1alpha2", Kind: "ValidatingAdmissionPolicy"}:        {"Age"},
	{Group: "admissionregistration.k8s.io", Version: "v1alpha2", Kind: "ValidatingAdmissionPolicyBinding"}: {"Age"},
	{Group: "admissionregistration.k8s.io", Version: "v1beta1", Kind: "MutatingWebhookConfiguration"}:      {"Age"},
	{Group: "admissionregistration.k8s.io", Version: "v1beta1", Kind: "ValidatingWebhookConfiguration"}:    {"Age"},

	{Group: "apiserverinternal.k8s.io", Version: "v1alpha1", Kind: "StorageVersion"}: {"Age"},

	{Group: "apps", Version: "v1", Kind: "DaemonSet"}:               {"Age"},
	{Group: "apps", Version: "v1", Kind: "Deployment"}:              {"Age"},
	{Group: "apps", Version: "v1", Kind: "ReplicaSet"}:              {"Age"},
	{Group: "apps", Version: "v1", Kind: "StatefulSet"}:             {"Age"},
	{Group: "apps", Version: "v1beta1", Kind: "ControllerRevision"}: {"Age"},

	{Group: "autoscaling", Version: "v1", Kind: "Scale"}:                        {"Age"},
	{Group: "autoscaling", Version: "v2beta1", Kind: "HorizontalPodAutoscaler"}: {"Age"},

	{Group: "batch", Version: "v1", Kind: "Job"}:          {"Duration", "Age"},
	{Group: "batch", Version: "v1beta1", Kind: "CronJob"}: {"Last Schedule", "Age"},

	{Group: "certificates.k8s.io", Version: "v1beta1", Kind: "CertificateSigningRequest"}: {"Age"},

	{Group: "coordination.k8s.io", Version: "v1", Kind: "Lease"}:                {"Age"},
	{Group: "coordination.k8s.io", Version: "v1alpha2", Kind: "LeaseCandidate"}: {"Age"},

	{Group: "discovery.k8s.io", Version: "v1beta1", Kind: "EndpointSlice"}: {"Age"},

	{Group: "flowcontrol.k8s.io", Version: "v1", Kind: "FlowSchema"}:                 {"Age"},
	{Group: "flowcontrol.k8s.io", Version: "v1", Kind: "PriorityLevelConfiguration"}: {"Age"},

	{Group: "networking.k8s.io", Version: "v1beta1", Kind: "Ingress"}:       {"Age"},
	{Group: "networking.k8s.io", Version: "v1beta1", Kind: "IngressClass"}:  {"Age"},
	{Group: "networking.k8s.io", Version: "v1beta1", Kind: "IPAddress"}:     {"Age"},
	{Group: "networking.k8s.io", Version: "v1beta1", Kind: "NetworkPolicy"}: {"Age"},
	{Group: "networking.k8s.io", Version: "v1beta1", Kind: "ServiceCIDR"}:   {"Age"},

	{Group: "node.k8s.io", Version: "v1", Kind: "RuntimeClass"}: {"Age"},

	{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"}: {"Age"},

	{Group: "rbac.authorization.k8s.io", Version: "v1beta1", Kind: "ClusterRoleBinding"}: {"Age"},
	{Group: "rbac.authorization.k8s.io", Version: "v1beta1", Kind: "RoleBinding"}:        {"Age"},

	{Group: "resource.k8s.io", Version: "v1beta1", Kind: "DeviceClass"}:           {"Age"},
	{Group: "resource.k8s.io", Version: "v1beta1", Kind: "ResourceClaim"}:         {"Age"},
	{Group: "resource.k8s.io", Version: "v1beta1", Kind: "ResourceClaimTemplate"}: {"Age"},
	{Group: "resource.k8s.io", Version: "v1beta1", Kind: "ResourceSlice"}:         {"Age"},

	{Group: "scheduling.k8s.io", Version: "v1", Kind: "PriorityClass"}: {"Age"},

	{Group: "storage.k8s.io", Version: "v1", Kind: "CSIDriver"}:                  {"Age"},
	{Group: "storage.k8s.io", Version: "v1", Kind: "CSINode"}:                    {"Age"},
	{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"}:               {"Age"},
	{Group: "storage.k8s.io", Version: "v1", Kind: "VolumeAttachment"}:           {"Age"},
	{Group: "storage.k8s.io", Version: "v1beta1", Kind: "VolumeAttributesClass"}: {"Age"},
}
