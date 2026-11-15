package datamover

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func handoffFixture(t *testing.T) (Volumes, *metav1.PartialObjectMetadata, *corev1.PersistentVolumeClaim, *fake.Clientset) {
	t.Helper()
	parent := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "tenant", UID: "restore-uid"}}
	stage := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "stage", Namespace: joblifecycle.Namespace, UID: "stage-uid"},
		Spec:   corev1.PersistentVolumeClaimSpec{VolumeName: "pv", StorageClassName: ptr.To("lvm"), VolumeMode: ptr.To(corev1.PersistentVolumeBlock), AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	joblifecycle.Track(&stage.ObjectMeta, parent, RestoreKind)
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv", UID: "pv-uid"},
		Spec: corev1.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			ClaimRef:               &corev1.ObjectReference{Name: stage.Name, Namespace: stage.Namespace, UID: stage.UID},
			PersistentVolumeSource: corev1.PersistentVolumeSource{CSI: &corev1.CSIPersistentVolumeSource{Driver: "lvm.driver", VolumeHandle: "data"}},
			VolumeMode:             stage.Spec.VolumeMode,
			NodeAffinity:           &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: []string{"node-a"}}}}}}},
		}, Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "transfer", Namespace: joblifecycle.Namespace, UID: "job-uid", Finalizers: []string{joblifecycle.CompletionFinalizer}}, Status: batchv1.JobStatus{Succeeded: 1}}
	joblifecycle.Track(&job.ObjectMeta, parent, RestoreKind)
	client := fake.NewSimpleClientset(stage, pv, job)
	client.PrependReactor("create", "persistentvolumeclaims", func(action clienttesting.Action) (bool, runtime.Object, error) {
		action.(clienttesting.CreateAction).GetObject().(*corev1.PersistentVolumeClaim).UID = "destination-uid"
		return false, nil, nil
	})
	destination := stage.DeepCopy()
	destination.ObjectMeta = metav1.ObjectMeta{Name: "destination", Namespace: parent.Namespace}
	destination.Spec.VolumeName = ""
	destination.Status = corev1.PersistentVolumeClaimStatus{}
	return Volumes{Client: client}, parent, destination, client
}

func TestHandoffSurvivesEveryReconcile(t *testing.T) {
	v, parent, destination, client := handoffFixture(t)
	ctx := context.Background()
	original, err := client.CoreV1().PersistentVolumes().Get(ctx, "pv", metav1.GetOptions{})
	require.NoError(t, err)
	require.ErrorIs(t, v.BeginHandoff(parent, "stage", "transfer", destination), ErrPending)
	ready := false
	for i := 0; i < 12 && !ready; i++ {
		// No in-memory state survives between calls.
		found, done, err := (Volumes{Client: client}).ResumeHandoff(parent, "stage", destination)
		require.True(t, found)
		if err != nil {
			require.ErrorIs(t, err, ErrPending)
		}
		ready = done
		pv, err := client.CoreV1().PersistentVolumes().Get(ctx, "pv", metav1.GetOptions{})
		require.NoError(t, err)
		require.NotNil(t, pv.Spec.ClaimRef)
		require.Equal(t, original.Spec.NodeAffinity, pv.Spec.NodeAffinity)
		require.Equal(t, original.Spec.CSI, pv.Spec.CSI)
		require.Equal(t, original.Spec.VolumeMode, pv.Spec.VolumeMode)
		if pv.Spec.ClaimRef.Namespace != parent.Namespace {
			continue
		}
		claim, err := client.CoreV1().PersistentVolumeClaims(parent.Namespace).Get(ctx, destination.Name, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, claim.UID, pv.Spec.ClaimRef.UID)
		claim.Status.Phase = corev1.ClaimBound
		_, err = client.CoreV1().PersistentVolumeClaims(parent.Namespace).UpdateStatus(ctx, claim, metav1.UpdateOptions{})
		require.NoError(t, err)
	}
	require.True(t, ready)
	pv, err := client.CoreV1().PersistentVolumes().Get(ctx, "pv", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, corev1.PersistentVolumeReclaimDelete, pv.Spec.PersistentVolumeReclaimPolicy)
	require.Equal(t, complete, pv.Annotations[TransferState])
	require.NoError(t, v.Cleanup(parent), "parent deletion preserves the destination")
	_, err = client.CoreV1().PersistentVolumes().Get(ctx, "pv", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestHandoffWaitsForConsumersAndDetachment(t *testing.T) {
	v, parent, destination, client := handoffFixture(t)
	ctx := context.Background()
	require.ErrorIs(t, v.BeginHandoff(parent, "stage", "transfer", destination), ErrPending)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "consumer", Namespace: joblifecycle.Namespace, Labels: map[string]string{batchv1.JobNameLabel: "transfer"}}}
	_, err := client.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, _, err = v.ResumeHandoff(parent, "stage", destination)
		require.ErrorIs(t, err, ErrPending)
	}
	_, err = client.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace).Get(ctx, "stage", metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, client.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}))
	attachment := &storagev1.VolumeAttachment{ObjectMeta: metav1.ObjectMeta{Name: "attachment"}, Spec: storagev1.VolumeAttachmentSpec{Source: storagev1.VolumeAttachmentSource{PersistentVolumeName: ptr.To("pv")}}}
	_, err = client.StorageV1().VolumeAttachments().Create(ctx, attachment, metav1.CreateOptions{})
	require.NoError(t, err)
	_, _, err = v.ResumeHandoff(parent, "stage", destination)
	require.ErrorIs(t, err, ErrPending)
	_, err = client.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace).Get(ctx, "stage", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestHandoffRejectsForeignDestination(t *testing.T) {
	v, parent, destination, client := handoffFixture(t)
	require.ErrorIs(t, v.BeginHandoff(parent, "stage", "transfer", destination), ErrPending)
	_, err := client.CoreV1().PersistentVolumeClaims(parent.Namespace).Create(context.Background(), destination, metav1.CreateOptions{})
	require.NoError(t, err)
	_, _, err = v.ResumeHandoff(parent, "stage", destination)
	require.ErrorContains(t, err, "does not belong")
	for _, action := range client.Actions() {
		require.NotEqual(t, "delete", action.GetVerb())
	}
}

func TestCancelBeforeHandoffReclaimsOnlyStaging(t *testing.T) {
	v, parent, destination, client := handoffFixture(t)
	require.ErrorIs(t, v.BeginHandoff(parent, "stage", "transfer", destination), ErrPending)
	require.NoError(t, StopJobs(client, parent, RestoreKind))
	for i := 0; i < 8; i++ {
		err := v.Cleanup(parent)
		if err == nil {
			break
		}
		require.ErrorIs(t, err, ErrPending)
	}
	volumes, err := client.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, volumes.Items)
}

func TestDeleteVolumeWaitsForPolicy(t *testing.T) {
	v, parent, destination, client := handoffFixture(t)
	require.ErrorIs(t, v.BeginHandoff(parent, "stage", "transfer", destination), ErrPending)
	volumes := client.CoreV1().PersistentVolumes()
	pv, err := volumes.Get(context.Background(), "pv", metav1.GetOptions{})
	require.NoError(t, err)
	client.ClearActions()

	require.ErrorIs(t, v.deleteVolume(pv), ErrPending)
	for _, action := range client.Actions() {
		require.NotEqual(t, "delete", action.GetVerb(), "policy update must precede deletion")
	}
	pv, err = volumes.Get(context.Background(), pv.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, corev1.PersistentVolumeReclaimDelete, pv.Spec.PersistentVolumeReclaimPolicy)
	client.ClearActions()

	require.ErrorIs(t, v.deleteVolume(pv), ErrPending)
	actions := client.Actions()
	require.Len(t, actions, 1)
	deletion, ok := actions[0].(clienttesting.DeleteAction)
	require.True(t, ok)
	require.Equal(t, pv.UID, *deletion.GetDeleteOptions().Preconditions.UID)
	_, err = volumes.Get(context.Background(), pv.Name, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
}

func TestHandoffRetriesConflicts(t *testing.T) {
	for _, step := range []string{"checkpoint", "reservation", "reclaim policy"} {
		t.Run(step, func(t *testing.T) {
			v, parent, destination, client := handoffFixture(t)
			ctx := context.Background()
			conflicted := false
			client.PrependReactor("update", "persistentvolumes", func(action clienttesting.Action) (bool, runtime.Object, error) {
				pv := action.(clienttesting.UpdateAction).GetObject().(*corev1.PersistentVolume)
				currentStep := "checkpoint"
				if pv.Spec.ClaimRef.Namespace == destination.Namespace {
					currentStep = "reservation"
				}
				if pv.Annotations[TransferState] == complete {
					currentStep = "reclaim policy"
				}
				if !conflicted && currentStep == step {
					conflicted = true
					return true, nil, apierrors.NewConflict(corev1.Resource("persistentvolumes"), pv.Name, fmt.Errorf("concurrent update"))
				}
				return false, nil, nil
			})
			ready := false
			for i := 0; i < 20 && !ready; i++ {
				found, done, err := (Volumes{Client: client}).ResumeHandoff(parent, "stage", destination)
				if !found {
					err = v.BeginHandoff(parent, "stage", "transfer", destination)
				}
				if err != nil {
					require.True(t, Retryable(err), "%v", err)
				}
				ready = done
				pv, err := client.CoreV1().PersistentVolumes().Get(ctx, "pv", metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, pv.Spec.ClaimRef, "never expose an unreserved PV")
				if pv.Spec.ClaimRef.Namespace != destination.Namespace {
					continue
				}
				pvc, err := client.CoreV1().PersistentVolumeClaims(destination.Namespace).Get(ctx, destination.Name, metav1.GetOptions{})
				require.NoError(t, err)
				pvc.Status.Phase = corev1.ClaimBound
				_, err = client.CoreV1().PersistentVolumeClaims(destination.Namespace).UpdateStatus(ctx, pvc, metav1.UpdateOptions{})
				require.NoError(t, err)
			}
			require.True(t, conflicted)
			require.True(t, ready)
		})
	}
}

func TestCancelAfterAssignmentPreservesData(t *testing.T) {
	v, parent, destination, client := handoffFixture(t)
	ctx := context.Background()
	require.ErrorIs(t, v.BeginHandoff(parent, "stage", "transfer", destination), ErrPending)
	for i := 0; i < 8; i++ {
		_, _, err := v.ResumeHandoff(parent, "stage", destination)
		require.ErrorIs(t, err, ErrPending)
		pv, err := client.CoreV1().PersistentVolumes().Get(ctx, "pv", metav1.GetOptions{})
		require.NoError(t, err)
		if pv.Spec.ClaimRef.Namespace != destination.Namespace {
			continue
		}
		// Cancel before the binder reports the destination as Bound.
		require.NoError(t, v.Cleanup(parent))
		pvc, err := client.CoreV1().PersistentVolumeClaims(destination.Namespace).Get(ctx, destination.Name, metav1.GetOptions{})
		require.NoError(t, err)
		current, err := client.CoreV1().PersistentVolumes().Get(ctx, "pv", metav1.GetOptions{})
		require.NoError(t, err)
		require.True(t, claimMatches(current.Spec.ClaimRef, pvc))
		require.Equal(t, corev1.PersistentVolumeReclaimDelete, current.Spec.PersistentVolumeReclaimPolicy)
		return
	}
	t.Fatal("PV was never assigned to the destination")
}

func TestHandoffWaitsForOtherConsumers(t *testing.T) {
	v, parent, destination, client := handoffFixture(t)
	ctx := context.Background()
	require.ErrorIs(t, v.BeginHandoff(parent, "stage", "transfer", destination), ErrPending)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "other-consumer", Namespace: joblifecycle.Namespace},
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "stage"},
		}}}},
	}
	_, err := client.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, _, err = v.ResumeHandoff(parent, "stage", destination)
		require.ErrorIs(t, err, ErrPending)
	}
	_, err = client.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace).Get(ctx, "stage", metav1.GetOptions{})
	require.NoError(t, err)
	_, err = client.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	require.NoError(t, err)
}
