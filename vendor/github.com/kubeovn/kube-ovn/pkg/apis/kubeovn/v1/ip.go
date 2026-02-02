package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type IPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []IP `json:"items"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
type IP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec IPSpec `json:"spec"`
}

type IPSpec struct {
	PodName       string   `json:"podName"`
	Namespace     string   `json:"namespace"`
	Subnet        string   `json:"subnet"`
	AttachSubnets []string `json:"attachSubnets"`
	NodeName      string   `json:"nodeName"`
	IPAddress     string   `json:"ipAddress"`
	V4IPAddress   string   `json:"v4IpAddress"`
	V6IPAddress   string   `json:"v6IpAddress"`
	AttachIPs     []string `json:"attachIps"`
	MacAddress    string   `json:"macAddress"`
	AttachMacs    []string `json:"attachMacs"`
	ContainerID   string   `json:"containerID"`
	PodType       string   `json:"podType"`
}
