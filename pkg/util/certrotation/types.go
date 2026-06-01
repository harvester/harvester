package certrotation

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	AnnotationRKE2CertInfo              = "harvesterhci.io/rke2-cert-info"
	AnnotationRKE2CertRotationNodeState = "harvesterhci.io/rke2-cert-rotation-node-state"

	AnnotationRKE2CertRotationState = "harvesterhci.io/rke2-cert-rotation-state"

	LabelRKE2CertAction       = "harvesterhci.io/rke2-cert-action"
	LabelRKE2CertCheckPurpose = "harvesterhci.io/rke2-cert-check-purpose"
	LabelRKE2CertNode         = "harvesterhci.io/rke2-cert-node"
	LabelRKE2CertGeneration   = "harvesterhci.io/rke2-cert-generation"
)

type CertCheckPurpose string

const (
	CertCheckPurposeSchedule CertCheckPurpose = "schedule"
	CertCheckPurposeVerify   CertCheckPurpose = "verify"
)

const (
	CertActionCheck  = "check"
	CertActionRotate = "rotate"
)

// Cluster-wide rotation phases. Stored in ClusterRotationState.Phase.
const (
	PhaseIdle           = "idle"
	PhaseCPRotation     = "cp-rotation"
	PhaseWorkerRotation = "worker-rotation"
	PhaseCompleted      = "completed"
	PhaseFailed         = "failed"
)

// Per-node rotation phases. Stored in NodeRotationStatus.Phase.
const (
	NodePhasePending   = "pending"
	NodePhaseRotating  = "rotating"
	NodePhaseVerifying = "verifying"
	NodePhaseCompleted = "completed"
	NodePhaseFailed    = "failed"
)

type ClusterRotationState struct {
	Generation  int64       `json:"generation"`
	Phase       string      `json:"phase"`
	CurrentNode string      `json:"currentNode,omitempty"`
	StartedAt   metav1.Time `json:"startedAt"`
	UpdatedAt   metav1.Time `json:"updatedAt"`
	LastError   string      `json:"lastError,omitempty"`
}

type NodeRotationState struct {
	Generation    int64       `json:"generation"`
	Phase         string      `json:"phase"`
	JobName       string      `json:"jobName,omitempty"`
	VerifyJobName string      `json:"verifyJobName,omitempty"`
	StartedAt     metav1.Time `json:"startedAt"`
	UpdatedAt     metav1.Time `json:"updatedAt"`
	Reason        string      `json:"reason,omitempty"`
}

type CertInfo struct {
	ClosestExpiryTime metav1.Time `json:"closestExpiryTime"`
	UpdatedAt         metav1.Time `json:"updatedAt"`
}

func IsRotationInProgress(phase string) bool {
	return phase == PhaseCPRotation || phase == PhaseWorkerRotation
}

func LoadClusterStateFromAnnotations(annotations map[string]string) (ClusterRotationState, error) {
	raw, ok := annotations[AnnotationRKE2CertRotationState]
	if !ok || raw == "" {
		return ClusterRotationState{Phase: PhaseIdle}, nil
	}
	var st ClusterRotationState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return ClusterRotationState{}, err
	}
	return st, nil
}

func MarshalClusterState(state ClusterRotationState) (string, error) {
	b, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func EqualState(a, b ClusterRotationState) bool {
	return a.Generation == b.Generation && a.Phase == b.Phase && a.CurrentNode == b.CurrentNode
}

func LoadNodeStateFromAnnotations(annotations map[string]string) (NodeRotationState, bool, error) {
	raw, ok := annotations[AnnotationRKE2CertRotationNodeState]
	if !ok || raw == "" {
		return NodeRotationState{}, false, nil
	}
	var st NodeRotationState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return NodeRotationState{}, false, err
	}
	return st, true, nil
}

func MarshalNodeState(state NodeRotationState) (string, error) {
	b, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func LoadCertInfoFromAnnotations(annotations map[string]string) (CertInfo, bool, error) {
	raw, ok := annotations[AnnotationRKE2CertInfo]
	if !ok || raw == "" {
		return CertInfo{}, false, nil
	}
	var info CertInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return CertInfo{}, false, err
	}
	return info, true, nil
}

func MarshalCertInfo(info CertInfo) (string, error) {
	b, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
