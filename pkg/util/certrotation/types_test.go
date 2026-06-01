package certrotation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsRotationInProgress(t *testing.T) {
	tests := []struct {
		name  string
		phase string
		want  bool
	}{
		{"empty string is not in progress", "", false},
		{"idle is not in progress", PhaseIdle, false},
		{"completed is not in progress", PhaseCompleted, false},
		{"failed is not in progress", PhaseFailed, false},
		{"cp-rotation is in progress", PhaseCPRotation, true},
		{"worker-rotation is in progress", PhaseWorkerRotation, true},
		{"garbage value is not in progress", "garbage", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsRotationInProgress(tt.phase))
		})
	}
}

func TestLoadClusterStateFromAnnotations(t *testing.T) {
	now := metav1.NewTime(time.Now().UTC().Truncate(time.Second))

	t.Run("nil annotations map yields PhaseIdle with nil error", func(t *testing.T) {
		st, err := LoadClusterStateFromAnnotations(nil)
		require.NoError(t, err)
		assert.Equal(t, PhaseIdle, st.Phase)
		assert.Equal(t, int64(0), st.Generation)
	})

	t.Run("missing annotation yields PhaseIdle with nil error", func(t *testing.T) {
		st, err := LoadClusterStateFromAnnotations(map[string]string{
			"unrelated": "value",
		})
		require.NoError(t, err)
		assert.Equal(t, PhaseIdle, st.Phase)
	})

	t.Run("empty-string annotation yields PhaseIdle with nil error", func(t *testing.T) {
		st, err := LoadClusterStateFromAnnotations(map[string]string{
			AnnotationRKE2CertRotationState: "",
		})
		require.NoError(t, err)
		assert.Equal(t, PhaseIdle, st.Phase)
	})

	t.Run("valid JSON parses correctly", func(t *testing.T) {
		want := ClusterRotationState{
			Generation:  3,
			Phase:       PhaseCPRotation,
			CurrentNode: "node-1",
			StartedAt:   now,
			UpdatedAt:   now,
		}
		raw, err := json.Marshal(want)
		require.NoError(t, err)

		got, err := LoadClusterStateFromAnnotations(map[string]string{
			AnnotationRKE2CertRotationState: string(raw),
		})
		require.NoError(t, err)
		assert.Equal(t, want.Generation, got.Generation)
		assert.Equal(t, want.Phase, got.Phase)
		assert.Equal(t, want.CurrentNode, got.CurrentNode)
		require.NotNil(t, got.StartedAt)
		assert.True(t, want.StartedAt.Equal(&got.StartedAt))
		require.NotNil(t, got.UpdatedAt)
		assert.True(t, want.UpdatedAt.Equal(&got.UpdatedAt))
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, err := LoadClusterStateFromAnnotations(map[string]string{
			AnnotationRKE2CertRotationState: "{not valid json",
		})
		assert.Error(t, err)
	})
}

func TestMarshalClusterState_RoundTrip(t *testing.T) {
	now := metav1.NewTime(time.Now().UTC().Truncate(time.Second))
	original := ClusterRotationState{
		Generation:  7,
		Phase:       PhaseWorkerRotation,
		CurrentNode: "worker-2",
		StartedAt:   now,
		UpdatedAt:   now,
		LastError:   "",
	}

	raw, err := MarshalClusterState(original)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	got, err := LoadClusterStateFromAnnotations(map[string]string{
		AnnotationRKE2CertRotationState: raw,
	})
	require.NoError(t, err)
	assert.Equal(t, original.Generation, got.Generation)
	assert.Equal(t, original.Phase, got.Phase)
	assert.Equal(t, original.CurrentNode, got.CurrentNode)
	assert.Equal(t, original.LastError, got.LastError)
	require.NotNil(t, got.StartedAt)
	assert.True(t, original.StartedAt.Equal(&got.StartedAt))
	require.NotNil(t, got.UpdatedAt)
	assert.True(t, original.UpdatedAt.Equal(&got.UpdatedAt))
}

func TestMarshalClusterState_OmitsEmptyOptionalFields(t *testing.T) {
	// Sanity check that omitempty tags work, so terminal phases produce a
	// compact annotation value.
	state := ClusterRotationState{
		Generation: 1,
		Phase:      PhaseIdle,
	}
	raw, err := MarshalClusterState(state)
	require.NoError(t, err)
	assert.NotContains(t, raw, "currentNode")
	assert.NotContains(t, raw, "lastError")
}

func TestLoadNodeStatusFromAnnotations(t *testing.T) {
	now := metav1.NewTime(time.Now().UTC().Truncate(time.Second))

	t.Run("nil annotations map", func(t *testing.T) {
		st, ok, err := LoadNodeStateFromAnnotations(nil)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, NodeRotationState{}, st)
	})

	t.Run("missing annotation", func(t *testing.T) {
		st, ok, err := LoadNodeStateFromAnnotations(map[string]string{
			"other": "thing",
		})
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, NodeRotationState{}, st)
	})

	t.Run("empty-string annotation", func(t *testing.T) {
		st, ok, err := LoadNodeStateFromAnnotations(map[string]string{
			AnnotationRKE2CertRotationNodeState: "",
		})
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, NodeRotationState{}, st)
	})

	t.Run("valid JSON", func(t *testing.T) {
		want := NodeRotationState{
			Generation: 5,
			Phase:      NodePhaseVerifying,
			JobName:    "rke2-cert-rotate-node-1-5",
			StartedAt:  now,
			UpdatedAt:  now,
		}
		raw, err := json.Marshal(want)
		require.NoError(t, err)

		got, ok, err := LoadNodeStateFromAnnotations(map[string]string{
			AnnotationRKE2CertRotationNodeState: string(raw),
		})
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, want.Generation, got.Generation)
		assert.Equal(t, want.Phase, got.Phase)
		assert.Equal(t, want.JobName, got.JobName)
		require.NotNil(t, got.StartedAt)
		assert.True(t, want.StartedAt.Equal(&got.StartedAt))
		require.NotNil(t, got.UpdatedAt)
		assert.True(t, want.UpdatedAt.Equal(&got.UpdatedAt))
	})

	t.Run("malformed JSON", func(t *testing.T) {
		_, ok, err := LoadNodeStateFromAnnotations(map[string]string{
			AnnotationRKE2CertRotationNodeState: "}}}",
		})
		assert.Error(t, err)
		assert.False(t, ok)
	})
}

func TestMarshalNodeStatus_RoundTrip(t *testing.T) {
	now := metav1.NewTime(time.Now().UTC().Truncate(time.Second))
	original := NodeRotationState{
		Generation: 2,
		Phase:      NodePhaseFailed,
		JobName:    "rke2-cert-rotate-node-2-2",
		StartedAt:  now,
		UpdatedAt:  now,
		Reason:     "rke2-server failed to come back ready within 5m",
	}

	raw, err := MarshalNodeState(original)
	require.NoError(t, err)

	got, ok, err := LoadNodeStateFromAnnotations(map[string]string{
		AnnotationRKE2CertRotationNodeState: raw,
	})
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, original.Generation, got.Generation)
	assert.Equal(t, original.Phase, got.Phase)
	assert.Equal(t, original.JobName, got.JobName)
	assert.Equal(t, original.Reason, got.Reason)
	require.NotNil(t, got.StartedAt)
	assert.True(t, original.StartedAt.Equal(&got.StartedAt))
	require.NotNil(t, got.UpdatedAt)
	assert.True(t, original.UpdatedAt.Equal(&got.UpdatedAt))
}

func TestLoadCertInfoFromAnnotations(t *testing.T) {
	t.Run("nil annotations map", func(t *testing.T) {
		info, ok, err := LoadCertInfoFromAnnotations(nil)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, CertInfo{}, info)
	})

	t.Run("missing annotation", func(t *testing.T) {
		info, ok, err := LoadCertInfoFromAnnotations(map[string]string{
			"other": "thing",
		})
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, CertInfo{}, info)
	})

	t.Run("valid JSON matches helper-script field names", func(t *testing.T) {
		// This payload mirrors what rke2-cert-check.sh produces. If the
		// helper-script schema ever changes, this test will fail and remind
		// the developer to update both sides in lockstep.
		raw := `{"closestExpiryTime":"2027-05-07T09:38:50Z","updatedAt":"2026-05-11T10:02:14Z"}`
		info, ok, err := LoadCertInfoFromAnnotations(map[string]string{
			AnnotationRKE2CertInfo: raw,
		})
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "2027-05-07T09:38:50Z", info.ClosestExpiryTime.UTC().Format(time.RFC3339))
		assert.Equal(t, "2026-05-11T10:02:14Z", info.UpdatedAt.UTC().Format(time.RFC3339))
	})

	t.Run("malformed JSON", func(t *testing.T) {
		_, ok, err := LoadCertInfoFromAnnotations(map[string]string{
			AnnotationRKE2CertInfo: "not json",
		})
		assert.Error(t, err)
		assert.False(t, ok)
	})
}
