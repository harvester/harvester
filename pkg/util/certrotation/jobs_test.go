package certrotation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

func baseSpec(node *corev1.Node) CheckJobSpec {
	return CheckJobSpec{
		Node:                node,
		Namespace:           "harvester-system",
		Image:               "registry.example.com/harvester/general:v1",
		HelperConfigMapName: "harvester-helpers",
		Purpose:             CertCheckPurposeSchedule,
		Generation:          nil,
	}
}

func TestBuildCheckJob_SchedulePurposeBaseline(t *testing.T) {
	node := &corev1.Node{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
		ObjectMeta: metav1.ObjectMeta{Name: "node-1", UID: types.UID("uid-1")},
	}
	job := BuildCheckJob(baseSpec(node))

	// Job-level labels MUST always include the check-node and purpose
	// labels so dedup queries work regardless of dispatcher.
	assert.Equal(t, "node-1", job.Labels[LabelRKE2CertNode])
	assert.Equal(t, string(CertCheckPurposeSchedule), job.Labels[LabelRKE2CertCheckPurpose])
	// Pod template labels must mirror Job labels so apiserver Job
	// controller can match pods to Jobs without ambiguity.
	assert.Equal(t, job.Labels, job.Spec.Template.Labels)

	// Pinned to the node by hostname affinity.
	require.NotNil(t, job.Spec.Template.Spec.Affinity)
	require.NotNil(t, job.Spec.Template.Spec.Affinity.NodeAffinity)
	terms := job.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	require.NotNil(t, terms)
	require.Len(t, terms.NodeSelectorTerms, 1)
	require.Len(t, terms.NodeSelectorTerms[0].MatchExpressions, 1)
	expr := terms.NodeSelectorTerms[0].MatchExpressions[0]
	assert.Equal(t, corev1.LabelHostname, expr.Key)
	assert.Equal(t, []string{"node-1"}, expr.Values)

	// Privileged container with host-root + helpers volumes.
	require.Len(t, job.Spec.Template.Spec.Containers, 1)
	c := job.Spec.Template.Spec.Containers[0]
	require.NotNil(t, c.SecurityContext)
	require.NotNil(t, c.SecurityContext.Privileged)
	assert.True(t, *c.SecurityContext.Privileged)
	assert.Contains(t, c.Args, "/harvester-helpers/rke2-cert-check.sh")
	assert.Contains(t, c.Args, "node-1")
	assert.Equal(t, "harvester", job.Spec.Template.Spec.ServiceAccountName)

	// Owner reference points to the node so deleting the node cleans up
	// the Job.
	require.Len(t, job.OwnerReferences, 1)
	assert.Equal(t, "Node", job.OwnerReferences[0].Kind)
	assert.Equal(t, "node-1", job.OwnerReferences[0].Name)
	assert.Equal(t, types.UID("uid-1"), job.OwnerReferences[0].UID)

	// Generation label is NOT set for schedule-purpose Jobs by default.
	_, hasGen := job.Labels[LabelRKE2CertGeneration]
	assert.False(t, hasGen, "schedule-purpose Jobs should not carry a generation label by default")

	assert.NotNil(t, job.Spec.BackoffLimit)
	assert.NotNil(t, job.Spec.ActiveDeadlineSeconds)
	assert.Nil(t, job.Spec.TTLSecondsAfterFinished)
}

func TestBuildCheckJob_VerifyPurposeWithGenerationLabel(t *testing.T) {
	node := &corev1.Node{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
		ObjectMeta: metav1.ObjectMeta{Name: "node-2", UID: types.UID("uid-2")},
	}
	spec := baseSpec(node)
	spec.Purpose = CertCheckPurposeVerify
	spec.Generation = ptr.To(int64(7))

	job := BuildCheckJob(spec)

	assert.Equal(t, "node-2", job.Labels[LabelRKE2CertNode])
	assert.Equal(t, string(CertCheckPurposeVerify), job.Labels[LabelRKE2CertCheckPurpose])
	assert.Equal(t, "7", job.Labels[LabelRKE2CertGeneration])
	// Pod template inherits the same labels so kubectl describe-on-pod
	// shows the full provenance of the Job.
	assert.Equal(t, "7", job.Spec.Template.Labels[LabelRKE2CertGeneration])
	assert.Equal(t, string(CertCheckPurposeVerify), job.Spec.Template.Labels[LabelRKE2CertCheckPurpose])

	assert.NotNil(t, job.Spec.BackoffLimit)
	assert.NotNil(t, job.Spec.ActiveDeadlineSeconds)
	assert.Nil(t, job.Spec.TTLSecondsAfterFinished)
}

func TestGenerationLabelValue(t *testing.T) {
	assert.Equal(t, "0", GenerationLabelValue(0))
	assert.Equal(t, "1", GenerationLabelValue(1))
	assert.Equal(t, "12345", GenerationLabelValue(12345))
}
