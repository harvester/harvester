package kubernetes

import (
	corev1 "k8s.io/api/core/v1"
)

// IsContainerCompleted returns true if the container is terminated with exit code 0.
func IsContainerCompleted(status *corev1.ContainerStatus) bool {
	return status.State.Terminated != nil && status.State.Terminated.ExitCode == 0
}

// IsContainerInitializing returns true if the container is waiting for initialization.
func IsContainerInitializing(status *corev1.ContainerStatus) bool {
	return status.State.Waiting != nil && status.State.Waiting.Reason == "PodInitializing"
}

// IsContainerReady returns true if the container is ready.
func IsContainerReady(status *corev1.ContainerStatus) bool {
	return status.Ready
}

// IsContainerRestarted returns true if the container is terminated and restarted at least once.
func IsContainerRestarted(status *corev1.ContainerStatus) bool {
	return status.State.Terminated != nil && status.RestartCount > 0
}

// IsContainerWaitingCrashLoopBackOff returns true if the container is waiting for crash loop back off.
func IsContainerWaitingCrashLoopBackOff(status *corev1.ContainerStatus) bool {
	return status.State.Waiting != nil && status.State.Waiting.Reason == "CrashLoopBackOff"
}

// IsPodContainerInState checks if a container is in a desired state.
// The function searches through the container statuses of the pod of a pod (including init containers)
// to determine if a specific container is in the desired state of the conditionFunc.
func IsPodContainerInState(pod *corev1.Pod, containerName string, conditionFunc func(*corev1.ContainerStatus) bool) bool {
	containerStatuses := pod.DeepCopy().Status.InitContainerStatuses
	containerStatuses = append(containerStatuses, pod.DeepCopy().Status.ContainerStatuses...)

	// During the initialization phase of a pod, the statuses of init containers
	// may not be populated immediately. If this function is called during this
	// phase and the target container is an init container, return false to indicate
	// that the container is not yet the desired state.
	isChecked := false

	for _, status := range containerStatuses {
		if status.Name != containerName {
			continue
		}

		if !conditionFunc(&status) {
			return false
		}

		isChecked = true
	}
	return isChecked
}
