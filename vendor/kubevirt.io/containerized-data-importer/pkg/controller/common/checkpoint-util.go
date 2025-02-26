/*
Copyright 2023 The CDI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

const (
	// ImportPaused provides a const to indicate that a multistage import is waiting for the next stage
	ImportPaused = "ImportPaused"
	// MessageImportPaused provides a const for a "multistage import paused" message
	MessageImportPaused = "Multistage import into PVC %s is paused"
)

// CheckpointRecord is set after comparing the list of checkpoints in the DataVolume/VolumeImportSource
// spec with the annotations on the PVC indicating which checkpoints have already been copied.
// Return the first checkpoint that does not have this annotation, meaning the first checkpoint that has not yet been copied.
type CheckpointRecord struct {
	cdiv1.DataVolumeCheckpoint
	IsFinal bool
}

// CheckpointArgs is a struct used to store all checkpoint-related arguments to simplify passing them.
type CheckpointArgs struct {
	Client client.Client
	Log    logr.Logger
	// Checkpoints is a list of DataVolumeCheckpoints, representing stages in a multistage import.
	Checkpoints []cdiv1.DataVolumeCheckpoint
	// IsFinal indicates whether the current DataVolumeCheckpoint is the final checkpoint.
	IsFinal bool
}

// UpdatesMultistageImportSucceeded handles multi-stage annotations when the importer pod is succeeded
func UpdatesMultistageImportSucceeded(pvc *corev1.PersistentVolumeClaim, args *CheckpointArgs) error {
	if multiStageImport := metav1.HasAnnotation(pvc.ObjectMeta, AnnCurrentCheckpoint); !multiStageImport {
		return nil
	}

	// The presence of the current checkpoint annotation indicates it is a stage in a multistage import.
	// If all the checkpoints have been copied, then we need to remove the annotations from the PVC.
	// Otherwise, we need to change the annotations to advance to the next checkpoint.
	currentCheckpoint := pvc.Annotations[AnnCurrentCheckpoint]
	alreadyCopied := checkpointAlreadyCopied(pvc, currentCheckpoint)
	finalCheckpoint, _ := strconv.ParseBool(pvc.Annotations[AnnFinalCheckpoint])

	if finalCheckpoint && alreadyCopied {
		// Last checkpoint done, so clean up
		if err := deleteMultistageImportAnnotations(pvc, args); err != nil {
			return err
		}
	} else {
		// Advances annotations to next checkpoint
		if err := setPvcMultistageImportAnnotations(pvc, args); err != nil {
			return err
		}
	}
	return nil
}

// MaybeSetPvcMultiStageAnnotation sets the annotation if pvc needs it, and does not have it yet
func MaybeSetPvcMultiStageAnnotation(pvc *corev1.PersistentVolumeClaim, args *CheckpointArgs) error {
	if pvc.Status.Phase == corev1.ClaimBound {
		// If a PVC already exists with no multi-stage annotations, check if it
		// needs them set (if not already finished with an import).
		multiStageImport := (len(args.Checkpoints) > 0)
		multiStageAnnotationsSet := metav1.HasAnnotation(pvc.ObjectMeta, AnnCurrentCheckpoint)
		multiStageAlreadyDone := metav1.HasAnnotation(pvc.ObjectMeta, AnnMultiStageImportDone)
		if multiStageImport && !multiStageAnnotationsSet && !multiStageAlreadyDone {
			err := setPvcMultistageImportAnnotations(pvc, args)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Set the PVC annotations related to multi-stage imports so that they point to the next checkpoint to copy.
func setPvcMultistageImportAnnotations(pvc *corev1.PersistentVolumeClaim, args *CheckpointArgs) error {
	pvcCopy := pvc.DeepCopy()

	// Only mark this checkpoint complete if it was completed by the current pod.
	// This keeps us from skipping over checkpoints when a reconcile fails at a bad time.
	uuidAlreadyUsed := false
	for key, value := range pvcCopy.Annotations {
		if strings.HasPrefix(key, getCheckpointCopiedKey("")) { // Blank checkpoint name to get just the prefix
			if value == pvcCopy.Annotations[AnnCurrentPodID] {
				uuidAlreadyUsed = true
				break
			}
		}
	}
	if !uuidAlreadyUsed {
		// Mark checkpoint complete by saving UID of current pod to a
		// PVC annotation specific to this checkpoint.
		currentCheckpoint := pvcCopy.Annotations[AnnCurrentCheckpoint]
		if currentCheckpoint != "" {
			currentPodID := pvcCopy.Annotations[AnnCurrentPodID]
			annotation := getCheckpointCopiedKey(currentCheckpoint)
			pvcCopy.ObjectMeta.Annotations[annotation] = currentPodID
			args.Log.V(1).Info("UUID not already used, marking checkpoint completed by current pod ID.", "checkpoint", currentCheckpoint, "podId", currentPodID)
		} else {
			args.Log.Info("Cannot mark empty checkpoint complete. Check spec for empty checkpoints.")
		}
	}
	// else: If the UID was already used for another transfer, then we are
	// just waiting for a new pod to start up to transfer the next checkpoint.

	// Set multi-stage PVC annotations so further reconcile loops will create new pods as needed.
	checkpoint := GetNextCheckpoint(pvcCopy, args)
	if checkpoint != nil { // Only move to the next checkpoint if there is a next checkpoint to move to
		pvcCopy.ObjectMeta.Annotations[AnnCurrentCheckpoint] = checkpoint.Current
		pvcCopy.ObjectMeta.Annotations[AnnPreviousCheckpoint] = checkpoint.Previous
		pvcCopy.ObjectMeta.Annotations[AnnFinalCheckpoint] = strconv.FormatBool(checkpoint.IsFinal)

		// Check to see if there is a running pod for this PVC. If there are
		// more checkpoints to copy but the PVC is stopped in Succeeded,
		// reset the phase to get another pod started for the next checkpoint.
		podNamespace := pvc.Namespace

		phase := pvcCopy.ObjectMeta.Annotations[AnnPodPhase]
		pod, _ := GetPodFromPvc(args.Client, podNamespace, pvcCopy)
		if phase == string(corev1.PodSucceeded) {
			if pod != nil {
				// Once pod succeeds we can safely assume it's either being deleted or
				// intentionally retained for debugging purposes.
				// TODO: This pod should always have a deletion timestamp (GetPodFromPvc ignores multi-stage pods with AnnPodRetainAfterCompletion).
				//       Consider handling the scenario where it lacks one.
				args.Log.V(3).Info(fmt.Sprintf("Pod %s still exists but it's being deleted, continuing with multi-stage import", pod.Name))
			}
			// Reset PVC phase so importer will create a new pod
			pvcCopy.ObjectMeta.Annotations[AnnPodPhase] = string(corev1.PodUnknown)
			delete(pvcCopy.ObjectMeta.Annotations, AnnImportPod)
		}
		// else: There's a pod already running, no need to try to start a new one.
	}
	// else: There aren't any checkpoints ready to be copied over.

	// only update if something has changed
	if !reflect.DeepEqual(pvc, pvcCopy) {
		args.Log.V(1).Info("Updating PVC with new checkpoint info.", "current checkpoint:", pvcCopy.Annotations[AnnCurrentCheckpoint], "is final:", pvcCopy.Annotations[AnnFinalCheckpoint])
		return args.Client.Update(context.TODO(), pvcCopy)
	}
	return nil
}

// Clean up PVC annotations after a multi-stage import.
func deleteMultistageImportAnnotations(pvc *corev1.PersistentVolumeClaim, args *CheckpointArgs) error {
	pvcCopy := pvc.DeepCopy()
	delete(pvcCopy.Annotations, AnnCurrentCheckpoint)
	delete(pvcCopy.Annotations, AnnPreviousCheckpoint)
	delete(pvcCopy.Annotations, AnnFinalCheckpoint)
	delete(pvcCopy.Annotations, AnnCurrentPodID)

	prefix := getCheckpointCopiedKey("")
	for key := range pvcCopy.Annotations {
		if strings.HasPrefix(key, prefix) {
			delete(pvcCopy.Annotations, key)
		}
	}

	pvcCopy.ObjectMeta.Annotations[AnnMultiStageImportDone] = "true"

	// only update if something has changed
	if !reflect.DeepEqual(pvc, pvcCopy) {
		return args.Client.Update(context.TODO(), pvcCopy)
	}
	return nil
}

// Single place to hold the scheme for annotations that indicate a checkpoint
// has already been copied. Currently storage.checkpoint.copied.[checkpoint] = ID,
// where ID is the UID of the pod that successfully transferred that checkpoint.
func getCheckpointCopiedKey(checkpoint string) string {
	return AnnCheckpointsCopied + "." + checkpoint
}

// Find out if this checkpoint has already been copied by looking for an annotation
// like storage.checkpoint.copied.[checkpoint]. If it exists, then this checkpoint
// was already copied.
func checkpointAlreadyCopied(pvc *corev1.PersistentVolumeClaim, checkpoint string) bool {
	annotation := getCheckpointCopiedKey(checkpoint)
	return metav1.HasAnnotation(pvc.ObjectMeta, annotation)
}

// GetNextCheckpoint returns the appropriate checkpoint according to multistage annotations
func GetNextCheckpoint(pvc *corev1.PersistentVolumeClaim, args *CheckpointArgs) *CheckpointRecord {
	numCheckpoints := len(args.Checkpoints)
	if numCheckpoints < 1 {
		return nil
	}

	// If there are no annotations, get the first checkpoint from the spec
	if pvc.ObjectMeta.Annotations[AnnCurrentCheckpoint] == "" {
		checkpoint := &CheckpointRecord{
			cdiv1.DataVolumeCheckpoint{
				Current:  args.Checkpoints[0].Current,
				Previous: args.Checkpoints[0].Previous,
			},
			(numCheckpoints == 1) && args.IsFinal,
		}
		return checkpoint
	}

	// If there are annotations, keep checking the spec checkpoint list for an existing "copied.X" annotation until the first one not found
	for count, specCheckpoint := range args.Checkpoints {
		if specCheckpoint.Current == "" {
			args.Log.Info(fmt.Sprintf("DataVolume spec has a blank 'current' entry in checkpoint %d", count))
			continue
		}
		if !checkpointAlreadyCopied(pvc, specCheckpoint.Current) {
			checkpoint := &CheckpointRecord{
				cdiv1.DataVolumeCheckpoint{
					Current:  specCheckpoint.Current,
					Previous: specCheckpoint.Previous,
				},
				(numCheckpoints == (count + 1)) && args.IsFinal,
			}
			return checkpoint
		}
	}

	return nil
}
