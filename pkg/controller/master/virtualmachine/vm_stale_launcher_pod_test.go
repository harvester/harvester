package virtualmachine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	staleTestNS     = "default"
	staleTestVM     = "vm1"
	staleTestVMIUID = types.UID("vmi-uid-1")
)

func staleTestVMObj(volumes []kubevirtv1.Volume, mutate func(*kubevirtv1.VirtualMachine)) *kubevirtv1.VirtualMachine {
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Namespace: staleTestNS, Name: staleTestVM, Annotations: map[string]string{}},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{Volumes: volumes},
			},
		},
		Status: kubevirtv1.VirtualMachineStatus{PrintableStatus: kubevirtv1.VirtualMachineStatusRunning},
	}
	if mutate != nil {
		mutate(vm)
	}
	return vm
}

func staleTestVMI() *kubevirtv1.VirtualMachineInstance {
	return &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Namespace: staleTestNS, Name: staleTestVM, UID: staleTestVMIUID},
		Status:     kubevirtv1.VirtualMachineInstanceStatus{Phase: kubevirtv1.Running},
	}
}

func pvcVolume(name, claim string) kubevirtv1.Volume {
	return kubevirtv1.Volume{
		Name: name,
		VolumeSource: kubevirtv1.VolumeSource{
			PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim},
			},
		},
	}
}

func launcherPod(name string, uid types.UID, phase corev1.PodPhase, claims ...string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: staleTestNS,
			Name:      name,
			Labels: map[string]string{
				kubevirtv1.AppLabel:       virtLauncherAppLabelValue,
				kubevirtv1.CreatedByLabel: string(uid),
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: kubevirtv1.VirtualMachineInstanceGroupVersionKind.GroupVersion().String(),
				Kind:       kubevirtv1.VirtualMachineInstanceGroupVersionKind.Kind,
				Name:       staleTestVM,
				UID:        uid,
			}},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
	for _, c := range claims {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: c,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: c},
			},
		})
	}
	return pod
}

func newStaleTestController(t *testing.T, pvcs []*corev1.PersistentVolumeClaim, objs ...runtime.Object) (*VMController, *fake.Clientset) {
	clientset := fake.NewSimpleClientset()
	for _, obj := range objs {
		require.NoError(t, clientset.Tracker().Add(obj))
	}
	return &VMController{
		podClient: fakeclients.PodClient(clientset.CoreV1().Pods),
		podCache:  fakeclients.PodCache(clientset.CoreV1().Pods),
		vmCache:   fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		vmiCache:  fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
		pvcCache:  newFakePVCCache(pvcs...),
	}, clientset
}

func podExists(t *testing.T, clientset *fake.Clientset, name string) bool {
	_, err := clientset.CoreV1().Pods(staleTestNS).Get(context.TODO(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return true
}

func TestCleanupStaleLauncherPodsOnVMChange(t *testing.T) {
	backendPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: staleTestNS,
			Name:      "persistent-state-for-vm1-abcde",
			Labels:    map[string]string{util.LabelKubeVirtPersistentState: staleTestVM},
		},
	}

	otherVMBackendPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: staleTestNS,
			Name:      "persistent-state-for-vm2-fghij",
			Labels:    map[string]string{util.LabelKubeVirtPersistentState: "vm2"},
		},
	}

	tests := []struct {
		name        string
		vm          *kubevirtv1.VirtualMachine
		noVMI       bool
		pods        []*corev1.Pod
		pvcs        []*corev1.PersistentVolumeClaim
		wantDeleted []string
		wantKept    []string
	}{
		{
			name: "storage migration source pod is deleted",
			vm:   staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "new-pvc")}, nil),
			pods: []*corev1.Pod{
				launcherPod("source", staleTestVMIUID, corev1.PodSucceeded, "old-pvc"),
				launcherPod("target", staleTestVMIUID, corev1.PodRunning, "new-pvc"),
			},
			wantDeleted: []string{"source"},
			wantKept:    []string{"target"},
		},
		{
			name: "failed pod that references a PVC no longer in the spec is deleted",
			vm:   staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "old-pvc")}, nil),
			pods: []*corev1.Pod{
				launcherPod("failed-target", staleTestVMIUID, corev1.PodFailed, "new-pvc"),
				launcherPod("active", staleTestVMIUID, corev1.PodRunning, "old-pvc"),
			},
			wantDeleted: []string{"failed-target"},
			wantKept:    []string{"active"},
		},
		{
			name: "compute live migration source pod with the same PVCs is kept",
			vm:   staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "pvc")}, nil),
			pods: []*corev1.Pod{
				launcherPod("source", staleTestVMIUID, corev1.PodSucceeded, "pvc"),
			},
			wantKept: []string{"source"},
		},
		{
			name: "running pod with a PVC no longer in the spec is kept (restart pending)",
			vm:   staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "new-pvc")}, nil),
			pods: []*corev1.Pod{
				launcherPod("active", staleTestVMIUID, corev1.PodRunning, "old-pvc"),
			},
			wantKept: []string{"active"},
		},
		{
			name: "nothing is deleted while the VolumesChange condition is set",
			vm: staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "new-pvc")}, func(vm *kubevirtv1.VirtualMachine) {
				vm.Status.Conditions = []kubevirtv1.VirtualMachineCondition{{
					Type:   kubevirtv1.VirtualMachineConditionType(kubevirtv1.VirtualMachineInstanceVolumesChange),
					Status: corev1.ConditionTrue,
				}}
			}),
			pods: []*corev1.Pod{
				launcherPod("source", staleTestVMIUID, corev1.PodSucceeded, "old-pvc"),
			},
			wantKept: []string{"source"},
		},
		{
			name: "nothing is deleted while a storage migration is waiting to start",
			vm: staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "new-pvc")}, func(vm *kubevirtv1.VirtualMachine) {
				vm.Annotations[util.AnnotationWaitingStorageMigration] = "true"
			}),
			pods: []*corev1.Pod{
				launcherPod("source", staleTestVMIUID, corev1.PodSucceeded, "old-pvc"),
			},
			wantKept: []string{"source"},
		},
		{
			name: "backend-storage PVC does not make a pod stale",
			vm:   staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "pvc")}, nil),
			pods: []*corev1.Pod{
				launcherPod("source", staleTestVMIUID, corev1.PodSucceeded, "pvc", backendPVC.Name),
			},
			pvcs:     []*corev1.PersistentVolumeClaim{backendPVC},
			wantKept: []string{"source"},
		},
		{
			name: "backend-storage PVC of another VM makes a pod stale",
			vm:   staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "pvc")}, nil),
			pods: []*corev1.Pod{
				launcherPod("source", staleTestVMIUID, corev1.PodSucceeded, "pvc", otherVMBackendPVC.Name),
			},
			pvcs:        []*corev1.PersistentVolumeClaim{otherVMBackendPVC},
			wantDeleted: []string{"source"},
		},
		{
			name: "DataVolume-backed disks count as in use",
			vm: staleTestVMObj([]kubevirtv1.Volume{{
				Name:         "data",
				VolumeSource: kubevirtv1.VolumeSource{DataVolume: &kubevirtv1.DataVolumeSource{Name: "dv-pvc"}},
			}}, nil),
			pods: []*corev1.Pod{
				launcherPod("source", staleTestVMIUID, corev1.PodSucceeded, "dv-pvc"),
			},
			wantKept: []string{"source"},
		},
		{
			name: "pods of another VMI are ignored",
			vm:   staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "new-pvc")}, nil),
			pods: []*corev1.Pod{
				launcherPod("other", types.UID("other-uid"), corev1.PodSucceeded, "old-pvc"),
			},
			wantKept: []string{"other"},
		},
		{
			name:  "no VMI means nothing to do",
			vm:    staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "new-pvc")}, nil),
			noVMI: true,
			pods: []*corev1.Pod{
				launcherPod("source", staleTestVMIUID, corev1.PodSucceeded, "old-pvc"),
			},
			wantKept: []string{"source"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			objs := []runtime.Object{tc.vm}
			if !tc.noVMI {
				objs = append(objs, staleTestVMI())
			}
			for _, p := range tc.pods {
				objs = append(objs, p)
			}
			ctrl, clientset := newStaleTestController(t, tc.pvcs, objs...)

			_, err := ctrl.CleanupStaleLauncherPodsOnVMChange("", tc.vm)
			require.NoError(t, err)

			for _, name := range tc.wantDeleted {
				assert.False(t, podExists(t, clientset, name), "pod %s should be deleted", name)
			}
			for _, name := range tc.wantKept {
				assert.True(t, podExists(t, clientset, name), "pod %s should be kept", name)
			}
		})
	}
}

func TestCleanupStaleLauncherPodsOnPodChange(t *testing.T) {
	vm := staleTestVMObj([]kubevirtv1.Volume{pvcVolume("data", "new-pvc")}, nil)

	t.Run("terminated launcher pod triggers cleanup", func(t *testing.T) {
		source := launcherPod("source", staleTestVMIUID, corev1.PodSucceeded, "old-pvc")
		ctrl, clientset := newStaleTestController(t, nil, vm, staleTestVMI(), source)

		_, err := ctrl.CleanupStaleLauncherPodsOnPodChange("", source)
		require.NoError(t, err)
		assert.False(t, podExists(t, clientset, "source"))
	})

	t.Run("non-launcher pod is ignored", func(t *testing.T) {
		other := launcherPod("job", staleTestVMIUID, corev1.PodSucceeded, "old-pvc")
		other.Labels[kubevirtv1.AppLabel] = "something-else"
		ctrl, clientset := newStaleTestController(t, nil, vm, staleTestVMI(), other)

		_, err := ctrl.CleanupStaleLauncherPodsOnPodChange("", other)
		require.NoError(t, err)
		assert.True(t, podExists(t, clientset, "job"))
	})

	t.Run("pod whose VM is gone is ignored", func(t *testing.T) {
		source := launcherPod("source", staleTestVMIUID, corev1.PodSucceeded, "old-pvc")
		ctrl, clientset := newStaleTestController(t, nil, staleTestVMI(), source)

		_, err := ctrl.CleanupStaleLauncherPodsOnPodChange("", source)
		require.NoError(t, err)
		assert.True(t, podExists(t, clientset, "source"))
	})
}
