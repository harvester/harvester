// Package jobprogress reads the latest progress percentage emitted by the
// `harvester io-mode` subcommand from a Job's pod logs. The io-mode binary
// writes a structured line each second to stderr of the form:
//
//	IO_PROGRESS mode=READ bytes=12345 total=67890 percent=12.34
//
// This helper finds the most recent such line in the latest pod for a Job and
// returns the integer percent (0-100). It is best-effort: callers should fall
// back to the previously-stored progress if the helper returns an error.
package jobprogress

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

const (
	// jobNameLabel is the standard label batch/v1 Jobs apply to their pods.
	// Both "job-name" (legacy) and "batch.kubernetes.io/job-name" (>=1.27)
	// are emitted; the legacy form is still supported by all recent versions.
	jobNameLabel = "job-name"

	// We only look at the tail of the pod log: io-mode emits one progress
	// line per second, so 50 lines is ~50 s of history — enough to recover
	// the latest reading without pulling the whole transcript.
	logTailLines = int64(50)
)

var progressLineRE = regexp.MustCompile(`IO_PROGRESS\s+mode=\S+\s+bytes=\d+\s+total=\d+\s+percent=([0-9.]+)`)

// FetchRestoreProgress caps log-derived progress at 99. Completing the
// byte stream does not prove the restore command exited successfully; only the
// restore Job's Succeeded status may advance volume progress to 100.
func FetchRestoreProgress(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace, jobName string,
) (int, error) {
	pct, err := FetchProgressPercent(ctx, clientset, namespace, jobName)
	return min(pct, 99), err
}

// FetchProgressPercent returns the most recent IO_PROGRESS percent emitted by
// the named Job's pod. Returns ErrNoProgress if no progress line is found yet.
func FetchProgressPercent(ctx context.Context, clientset kubernetes.Interface, namespace, jobName string) (int, error) {
	if clientset == nil {
		return 0, fmt.Errorf("clientset is nil")
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", jobNameLabel, jobName),
	})
	if err != nil {
		return 0, fmt.Errorf("listing pods for job %s/%s: %w", namespace, jobName, err)
	}
	pod := mostRecentPod(pods.Items)
	if pod == nil {
		return 0, ErrNoProgress
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		TailLines: ptr.To(logTailLines),
	})
	stream, err := req.Stream(ctx)
	if err != nil {
		// Container not started yet, or already gone — surface as not-found
		// rather than a hard failure so the caller keeps the stored value.
		if apierrors.IsNotFound(err) || apierrors.IsBadRequest(err) {
			return 0, ErrNoProgress
		}
		return 0, fmt.Errorf("opening log stream for pod %s/%s: %w", namespace, pod.Name, err)
	}
	defer stream.Close()

	logs, err := io.ReadAll(stream)
	if err != nil {
		return 0, fmt.Errorf("reading log stream for pod %s/%s: %w", namespace, pod.Name, err)
	}

	matches := progressLineRE.FindAllSubmatch(logs, -1)
	if len(matches) == 0 {
		return 0, ErrNoProgress
	}
	last := matches[len(matches)-1][1]
	pct, err := strconv.ParseFloat(string(last), 64)
	if err != nil {
		return 0, fmt.Errorf("parsing progress percent %q: %w", string(last), err)
	}
	if pct < 0 {
		return 0, nil
	}
	if pct > 100 {
		return 100, nil
	}
	return int(pct), nil
}

// ErrNoProgress is returned when no IO_PROGRESS line is available yet (pod
// not started, no log output yet, or no progress lines emitted).
var ErrNoProgress = fmt.Errorf("no progress line in pod logs")

// mostRecentPod picks the pod with the latest CreationTimestamp; on a job
// retry there may be multiple pods, and we want the active/most recent one.
func mostRecentPod(pods []corev1.Pod) *corev1.Pod {
	var newest *corev1.Pod
	for i := range pods {
		p := &pods[i]
		if newest == nil || p.CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = p
		}
	}
	return newest
}
