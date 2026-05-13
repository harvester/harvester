package node

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/certrotation"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const testNamespace = "harvester-system"

func newCertCheckHandlerWithJobs(jobs ...*batchv1.Job) *certCheckNodeHandler {
	objs := make([]runtime.Object, 0, len(jobs))
	for _, j := range jobs {
		objs = append(objs, j)
	}
	clientset := fake.NewSimpleClientset(objs...)
	return &certCheckNodeHandler{
		jobCache:  fakeclients.JobCache(clientset.BatchV1().Jobs),
		jobClient: fakeclients.JobClient(clientset.BatchV1().Jobs),
		namespace: testNamespace,
	}
}

func certInfoAnnotation(t *testing.T, info certrotation.CertInfo) map[string]string {
	t.Helper()
	raw, err := certrotation.MarshalCertInfo(info)
	require.NoError(t, err)
	return map[string]string{certrotation.AnnotationRKE2CertInfo: raw}
}

func TestDueAt(t *testing.T) {
	h := &certCheckNodeHandler{}
	logger := logrus.NewEntry(logrus.New())
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name             string
		annotations      map[string]string
		expectDue        bool
		expectUntilCheck func(t *testing.T, until time.Duration)
	}{
		{
			name:        "missing annotation -> due now",
			annotations: nil,
			expectUntilCheck: func(t *testing.T, until time.Duration) {
				assert.Equal(t, until, time.Duration(0))
			},
		},
		{
			name:        "empty annotation value -> due now",
			annotations: map[string]string{certrotation.AnnotationRKE2CertInfo: ""},
			expectUntilCheck: func(t *testing.T, until time.Duration) {
				assert.Equal(t, until, time.Duration(0))
			},
		},
		{
			name:        "malformed JSON -> due now",
			annotations: map[string]string{certrotation.AnnotationRKE2CertInfo: "{not json"},
			expectUntilCheck: func(t *testing.T, until time.Duration) {
				assert.Equal(t, until, time.Duration(0))
			},
		},
		{
			name: "checked just now -> not due",
			annotations: certInfoAnnotation(t, certrotation.CertInfo{
				ClosestExpiryTime: metav1.Time{Time: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
				UpdatedAt:         metav1.Time{Time: now},
			}),
			expectUntilCheck: func(t *testing.T, until time.Duration) {
				assert.GreaterOrEqual(t, until, certCheckInterval-time.Second)
				assert.LessOrEqual(t, until, certCheckInterval+certCheckJitterMax*time.Second)
			},
		},
		{
			name: "checked >24h+jitter ago -> due",
			annotations: certInfoAnnotation(t, certrotation.CertInfo{
				ClosestExpiryTime: metav1.Time{Time: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
				UpdatedAt:         metav1.Time{Time: now.Add(-26 * time.Hour)},
			}),
			expectUntilCheck: func(t *testing.T, until time.Duration) {
				assert.Equal(t, until, time.Duration(0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
				Name:        "n",
				UID:         types.UID("stable-uid-for-deterministic-jitter"),
				Annotations: tt.annotations,
			}}
			until := h.untilNextCheck(logger, n, now)
			tt.expectUntilCheck(t, until)
		})
	}
}

func TestRunningCheckJobsForNode(t *testing.T) {
	mkJob := func(name, nodeName string, complete, failed, deleted bool) *batchv1.Job {
		j := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
				Labels: map[string]string{
					certrotation.LabelRKE2CertAction:       certrotation.CertActionCheck,
					certrotation.LabelRKE2CertNode:         nodeName,
					certrotation.LabelRKE2CertCheckPurpose: string(certrotation.CertCheckPurposeSchedule),
				},
			},
		}
		if complete {
			j.Status.Conditions = append(j.Status.Conditions, batchv1.JobCondition{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
			})
		}
		if failed {
			j.Status.Conditions = append(j.Status.Conditions, batchv1.JobCondition{
				Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
			})
		}
		if deleted {
			now := metav1.Now()
			j.DeletionTimestamp = &now
		}
		return j
	}

	tests := []struct {
		name     string
		jobs     []*batchv1.Job
		nodeName string
		want     []string
	}{
		{
			name:     "no jobs",
			jobs:     nil,
			nodeName: "n1",
			want:     nil,
		},
		{
			name:     "running job for n1 returned",
			jobs:     []*batchv1.Job{mkJob("j1", "n1", false, false, false)},
			nodeName: "n1",
			want:     []string{"j1"},
		},
		{
			name:     "completed job excluded",
			jobs:     []*batchv1.Job{mkJob("j1", "n1", true, false, false)},
			nodeName: "n1",
			want:     nil,
		},
		{
			name:     "failed job excluded",
			jobs:     []*batchv1.Job{mkJob("j1", "n1", false, true, false)},
			nodeName: "n1",
			want:     nil,
		},
		{
			name:     "deleting job excluded",
			jobs:     []*batchv1.Job{mkJob("j1", "n1", false, false, true)},
			nodeName: "n1",
			want:     nil,
		},
		{
			name:     "job for other node excluded",
			jobs:     []*batchv1.Job{mkJob("j1", "n2", false, false, false)},
			nodeName: "n1",
			want:     nil,
		},
		{
			name: "mixed: returns only running for n1",
			jobs: []*batchv1.Job{
				mkJob("done", "n1", true, false, false),
				mkJob("running", "n1", false, false, false),
				mkJob("other", "n2", false, false, false),
			},
			nodeName: "n1",
			want:     []string{"running"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCertCheckHandlerWithJobs(tt.jobs...)
			got, err := h.runningCheckJobsForNode(tt.nodeName)
			require.NoError(t, err)
			gotNames := make([]string, 0, len(got))
			for _, j := range got {
				gotNames = append(gotNames, j.Name)
			}
			assert.ElementsMatch(t, tt.want, gotNames)
		})
	}
}

func TestRunningCheckJobsForNode_SharedDedupPool(t *testing.T) {
	mkInflight := func(name, nodeName, purpose string) *batchv1.Job {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
				Labels: map[string]string{
					certrotation.LabelRKE2CertAction:       certrotation.CertActionCheck,
					certrotation.LabelRKE2CertNode:         nodeName,
					certrotation.LabelRKE2CertCheckPurpose: purpose,
				},
			},
		}
	}

	tests := []struct {
		name string
		jobs []*batchv1.Job
		want []string
	}{
		{
			name: "schedule in flight -> returned",
			jobs: []*batchv1.Job{mkInflight("sched", "n1", string(certrotation.CertCheckPurposeSchedule))},
			want: []string{"sched"},
		},
		{
			name: "verify in flight -> returned (shared pool)",
			jobs: []*batchv1.Job{mkInflight("verify", "n1", string(certrotation.CertCheckPurposeVerify))},
			want: []string{"verify"},
		},
		{
			name: "both in flight -> both returned",
			jobs: []*batchv1.Job{
				mkInflight("sched", "n1", string(certrotation.CertCheckPurposeSchedule)),
				mkInflight("verify", "n1", string(certrotation.CertCheckPurposeVerify)),
			},
			want: []string{"sched", "verify"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCertCheckHandlerWithJobs(tt.jobs...)
			got, err := h.runningCheckJobsForNode("n1")
			require.NoError(t, err)
			gotNames := make([]string, 0, len(got))
			for _, j := range got {
				gotNames = append(gotNames, j.Name)
			}
			assert.ElementsMatch(t, tt.want, gotNames)
		})
	}
}

func TestOnJobChanged_IgnoresVerifyPurpose(t *testing.T) {
	mkVerify := func(complete bool) *batchv1.Job {
		j := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "verify-job",
				Namespace: testNamespace,
				Labels: map[string]string{
					certrotation.LabelRKE2CertAction:       certrotation.CertActionCheck,
					certrotation.LabelRKE2CertNode:         "n1",
					certrotation.LabelRKE2CertCheckPurpose: string(certrotation.CertCheckPurposeVerify),
				},
			},
		}
		if complete {
			j.Status.Conditions = append(j.Status.Conditions, batchv1.JobCondition{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
			})
		}
		return j
	}

	h := newCertCheckHandlerWithJobs()
	// nodeController is not wired in this fixture; if OnJobChanged tried
	// to call h.nodeController.Enqueue for a verify Job, it would NPE.
	// The test passing without a panic is the assertion: the early
	// return for verify-purpose Jobs short-circuits before any Enqueue.
	for _, complete := range []bool{false, true} {
		_, err := h.OnJobChanged("verify-job", mkVerify(complete))
		assert.NoError(t, err, "OnJobChanged must not error on verify-purpose Jobs (complete=%v)", complete)
	}
}
