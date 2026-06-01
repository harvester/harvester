package setting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/util/certrotation"
)

func nodeWithLabels(name string, labels map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
	}
}

func certInfoAnnotations(t *testing.T, info certrotation.CertInfo) map[string]string {
	t.Helper()
	raw, err := certrotation.MarshalCertInfo(info)
	require.NoError(t, err)
	return map[string]string{certrotation.AnnotationRKE2CertInfo: raw}
}

func nodeStatusAnnotations(t *testing.T, status certrotation.NodeRotationState) map[string]string {
	t.Helper()
	raw, err := certrotation.MarshalNodeState(status)
	require.NoError(t, err)
	return map[string]string{certrotation.AnnotationRKE2CertRotationNodeState: raw}
}

func TestRoleOfNode(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   nodeRole
	}{
		{
			name:   "no role labels -> worker",
			labels: nil,
			want:   roleWorker,
		},
		{
			name:   "control-plane=true -> management",
			labels: map[string]string{"node-role.kubernetes.io/control-plane": "true"},
			want:   roleManagement,
		},
		{
			name:   "etcd=true (witness) -> management",
			labels: map[string]string{"node-role.kubernetes.io/etcd": "true"},
			want:   roleManagement,
		},
		{
			name: "control-plane + etcd both -> management (CP ordering)",
			labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "true",
				"node-role.kubernetes.io/etcd":          "true",
			},
			want: roleManagement,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := nodeWithLabels("n", tt.labels)
			assert.Equal(t, tt.want, roleOfNode(n))
		})
	}

	t.Run("nil node -> worker (defensive)", func(t *testing.T) {
		assert.Equal(t, roleWorker, roleOfNode(nil))
	})
}

func TestNodesForRole_FiltersAndSorts(t *testing.T) {
	cp1 := nodeWithLabels("cp-zzz", map[string]string{"node-role.kubernetes.io/control-plane": "true"})
	cp2 := nodeWithLabels("cp-aaa", map[string]string{"node-role.kubernetes.io/control-plane": "true"})
	w1 := nodeWithLabels("worker-2", nil)
	w2 := nodeWithLabels("worker-1", nil)
	witness := nodeWithLabels("witness", map[string]string{"node-role.kubernetes.io/etcd": "true"})
	deleting := nodeWithLabels("cp-deleting", map[string]string{"node-role.kubernetes.io/control-plane": "true"})
	now := metav1.Now()
	deleting.DeletionTimestamp = &now

	all := []*corev1.Node{cp1, cp2, w1, w2, witness, deleting, nil}

	mgmt := nodesForRole(all, roleManagement)
	require.Len(t, mgmt, 3)
	// Sorted by name; deleting and nil are excluded.
	assert.Equal(t, "cp-aaa", mgmt[0].Name)
	assert.Equal(t, "cp-zzz", mgmt[1].Name)
	assert.Equal(t, "witness", mgmt[2].Name)

	workers := nodesForRole(all, roleWorker)
	require.Len(t, workers, 2)
	assert.Equal(t, "worker-1", workers[0].Name)
	assert.Equal(t, "worker-2", workers[1].Name)
}

func TestEarliestExpiry(t *testing.T) {
	mkTime := func(s string) time.Time {
		result, err := time.Parse(time.RFC3339, s)
		require.Nil(t, err)
		return result
	}
	mk := func(name, expiry string) *corev1.Node {
		n := nodeWithLabels(name, nil)
		if expiry != "" {
			n.Annotations = certInfoAnnotations(t, certrotation.CertInfo{
				ClosestExpiryTime: metav1.Time{Time: mkTime(expiry)},
				UpdatedAt:         metav1.Time{Time: mkTime("2026-05-11T10:00:00Z")},
			})
		}
		return n
	}

	t.Run("no nodes with annotation -> zero", func(t *testing.T) {
		ts, name := earliestExpiry([]*corev1.Node{mk("a", ""), mk("b", "")})
		assert.True(t, ts.IsZero())
		assert.Empty(t, name)
	})

	t.Run("malformed annotation skipped", func(t *testing.T) {
		bad := nodeWithLabels("bad", nil)
		bad.Annotations = map[string]string{certrotation.AnnotationRKE2CertInfo: "{not json"}
		good := mk("good", "2026-12-01T00:00:00Z")
		ts, name := earliestExpiry([]*corev1.Node{bad, good})
		assert.Equal(t, "good", name)
		assert.Equal(t, "2026-12-01T00:00:00Z", ts.UTC().Format(time.RFC3339))
	})

	t.Run("returns earliest across nodes", func(t *testing.T) {
		ts, name := earliestExpiry([]*corev1.Node{
			mk("late", "2027-01-01T00:00:00Z"),
			mk("early", "2026-06-01T00:00:00Z"),
			mk("middle", "2026-09-01T00:00:00Z"),
		})
		assert.Equal(t, "early", name)
		assert.Equal(t, "2026-06-01T00:00:00Z", ts.UTC().Format(time.RFC3339))
	})

	t.Run("nil node entries skipped", func(t *testing.T) {
		ts, name := earliestExpiry([]*corev1.Node{nil, mk("only", "2027-01-01T00:00:00Z"), nil})
		assert.Equal(t, "only", name)
		assert.False(t, ts.IsZero())
	})
}

func TestPickNextNodeForRotation(t *testing.T) {
	const gen = int64(5)

	completed := nodeWithLabels("done", nil)
	completed.Annotations = nodeStatusAnnotations(t, certrotation.NodeRotationState{
		Generation: gen, Phase: certrotation.NodePhaseCompleted,
	})

	rotating := nodeWithLabels("rotating", nil)
	rotating.Annotations = nodeStatusAnnotations(t, certrotation.NodeRotationState{
		Generation: gen, Phase: certrotation.NodePhaseRotating,
	})

	staleGen := nodeWithLabels("stale", nil)
	staleGen.Annotations = nodeStatusAnnotations(t, certrotation.NodeRotationState{
		Generation: gen - 1, Phase: certrotation.NodePhaseCompleted,
	})

	pristine := nodeWithLabels("pristine", nil)

	tests := []struct {
		name  string
		nodes []*corev1.Node
		want  string // name of the picked node, or "" for nil
	}{
		{
			name:  "empty list -> nil",
			nodes: nil,
			want:  "",
		},
		{
			name:  "all completed at current gen -> nil",
			nodes: []*corev1.Node{completed, completed},
			want:  "",
		},
		{
			name:  "in-flight at current gen -> picked",
			nodes: []*corev1.Node{completed, rotating, completed},
			want:  "rotating",
		},
		{
			name:  "older generation -> picked",
			nodes: []*corev1.Node{completed, staleGen},
			want:  "stale",
		},
		{
			name:  "no annotation -> picked",
			nodes: []*corev1.Node{completed, pristine},
			want:  "pristine",
		},
		{
			name:  "preserves input order",
			nodes: []*corev1.Node{rotating, pristine},
			want:  "rotating",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickNextNodeForRotation(tt.nodes, gen)
			if tt.want == "" {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.Name)
		})
	}
}
