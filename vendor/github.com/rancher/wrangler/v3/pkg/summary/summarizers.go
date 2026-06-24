package summary

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/rancher/wrangler/v3/pkg/data"
	"github.com/rancher/wrangler/v3/pkg/data/convert"
	"github.com/rancher/wrangler/v3/pkg/kv"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kstatus "sigs.k8s.io/cli-utils/pkg/kstatus/status"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	kindSep                    = ", Kind="
	reason                     = "%REASON%"
	checkGVKErrorMappingEnvVar = "CATTLE_WRANGLER_CHECK_GVK_ERROR_MAPPING"
)

var (
	// stepCounterMessagePattern matches "X of Y completed" messages from CAPI infrastructure providers.
	// These are from the deprecated v1beta1 conditions merge strategy and are not useful to end users.
	stepCounterMessagePattern = regexp.MustCompile(`^\d+ of \d+ completed$`)

	// True ==
	// False == error
	// Unknown == transitioning
	TransitioningUnknown = map[string]string{
		"Active":                      "activating",
		"AddonDeploy":                 "provisioning",
		"AgentDeployed":               "provisioning",
		"BackingNamespaceCreated":     "configuring",
		"Built":                       "building",
		"CertsGenerated":              "provisioning",
		"ConfigOK":                    "configuring",
		"Created":                     "creating",
		"CreatorMadeOwner":            "configuring",
		"DefaultNamespaceAssigned":    "configuring",
		"DefaultNetworkPolicyCreated": "configuring",
		"DefaultProjectCreated":       "configuring",
		"DockerProvisioned":           "provisioning",
		"Deployed":                    "deploying",
		"Drained":                     "draining",
		"Downloaded":                  "downloading",
		"etcd":                        "provisioning",
		"Inactive":                    "deactivating",
		"Initialized":                 "initializing",
		"Installed":                   "installing",
		"NodesCreated":                "provisioning",
		"Pending":                     "pending",
		"PodScheduled":                "scheduling",
		"Provisioned":                 "provisioning",
		"Reconciled":                  "reconciling", // CAPI Machine, RKEControlPlane
		"Refreshed":                   "refreshed",
		"Registered":                  "registering",
		"Removed":                     "removing",
		"Saved":                       "saving",
		"Updated":                     "updating",
		"Updating":                    "updating",
		"Upgraded":                    "upgrading",
		"Waiting":                     "waiting",
		"InitialRolesPopulated":       "activating",
		"ScalingActive":               "pending",
		"AbleToScale":                 "pending",
		"RunCompleted":                "running",
		"Processed":                   "processed",
		"NodeHealthy":                 reason, // CAPI Machine
		"NodeReady":                   reason, // CAPI Machine
	}

	// For given GVK, This condition Type and this Status, indicates an error or not
	// e.g.: GVK: helm.cattle.io/v1, HelmChart
	//		--> JobCreated: [], indicates True or False are not errors
	//		--> Failed: ["True"], indicates "True" status is considered error
	//		--> Worked: ["False"], indicates "False" status is considered error
	//		--> Unknown: ["True", "False"] indicated "True" or "False" are considered errors
	GVKConditionErrorMapping = ConditionTypeStatusErrorMapping{
		{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChart"}: {
			"JobCreated": sets.New[metav1.ConditionStatus](),
			"Failed":     sets.New[metav1.ConditionStatus](metav1.ConditionTrue),
		},
		{Group: "", Version: "v1", Kind: "Node"}: {
			"OutOfDisk":          sets.New[metav1.ConditionStatus](metav1.ConditionTrue),
			"MemoryPressure":     sets.New[metav1.ConditionStatus](metav1.ConditionTrue),
			"DiskPressure":       sets.New[metav1.ConditionStatus](metav1.ConditionTrue),
			"NetworkUnavailable": sets.New[metav1.ConditionStatus](metav1.ConditionTrue),
		},
		{Group: "apps", Version: "v1", Kind: "Deployment"}: {
			"ReplicaFailure": sets.New[metav1.ConditionStatus](metav1.ConditionTrue),
			"Progressing":    sets.New[metav1.ConditionStatus](metav1.ConditionFalse),
		},
		{Group: "apps", Version: "v1", Kind: "ReplicaSet"}: {
			"ReplicaFailure": sets.New[metav1.ConditionStatus](metav1.ConditionTrue),
		},

		// FALLBACK: In case we cannot match any Groups, Versions and Kinds then we fallback to this mapping.
		{Group: "", Version: "", Kind: ""}: {
			"Stalled": sets.New[metav1.ConditionStatus](metav1.ConditionTrue),
			"Failed":  sets.New[metav1.ConditionStatus](metav1.ConditionTrue),
		},
	}

	// True ==
	// False == transitioning
	// Unknown == error
	TransitioningFalse = map[string]string{
		"Completed":            "activating",
		"Ready":                "unavailable",
		"Available":            "updating",
		"BootstrapConfigReady": reason,     // CAPI Machine
		"InfrastructureReady":  reason,     // CAPI Machine
		"MachinesReady":        "updating", // CAPI MachineDeployment, MachineSet
	}

	// True == transitioning
	// False ==
	// Unknown == error
	TransitioningTrue = map[string]string{
		"Reconciling": "reconciling",
		"ScalingUp":   reason, // CAPI Cluster, MachineDeployment, MachineSet
		"ScalingDown": reason, // CAPI Cluster, MachineDeployment, MachineSet
		"Deleting":    reason, // CAPI Cluster, MachineDeployment, MachineSet, Machine
		"Paused":      reason, // CAPI Cluster, MachineDeployment, MachineSet, Machine
	}

	Summarizers          []Summarizer
	ConditionSummarizers []Summarizer
)

type Summarizer func(obj data.Object, conditions []Condition, summary Summary) Summary

func init() {
	ConditionSummarizers = []Summarizer{
		checkErrors,
		checkGenericTransitioning,
		checkRemoving,
		checkCattleReady,
	}

	Summarizers = []Summarizer{
		checkStatusSummary,
		checkErrors,
		checkTransitioning,
		checkActive,
		checkPhase,
		checkInitializing,
		checkRemoving,
		checkStandard,
		checkLoadBalancer,
		checkPod,
		checkHasPodSelector,
		checkHasPodTemplate,
		checkOwner,
		checkApplyOwned,
		checkCattleTypes,
		checkGeneration,
	}

	initializeCheckErrors()
}

func initializeCheckErrors() {
	gvkConfig := os.Getenv(checkGVKErrorMappingEnvVar)
	if gvkConfig != "" {
		logrus.Debugf("GVK Error Mapping Provided")
		gvkErrorMapping := ConditionTypeStatusErrorMapping{}
		if err := json.Unmarshal([]byte(gvkConfig), &gvkErrorMapping); err != nil {
			logrus.Errorln("Unable to parse GVK config: ", err.Error())
			return
		}

		// Merging GVK + Conditions
		//
		// IMPORTANT: In case you add a condition that exists already, we replace the set that holds the Status
		// completely of that condition by yours, this makes it possible to deactivate certain statuses for
		// debugging reasons.
		//
		// eg.:
		//
		// Existing one:
		//
		// helm.cattle.io, Kind=HelmChart
		// JobCreated => []
		// Failed => ["True"]
		//
		// In case you set Failed = ["False"] and add Ready = ["False"]:
		//
		// helm.cattle.io, Kind=HelmChart
		// JobCreated => []			<<<= not changed
		// Failed => ["False"]		<<<= replaced completely the set.
		// Ready => ["False"] 		<<<= merged to existing conditions.
		//
		// So, we've merged the conditions, but not the status set values.
		for gvk, newConditionsMap := range gvkErrorMapping {
			if _, exists := GVKConditionErrorMapping[gvk]; !exists {
				GVKConditionErrorMapping[gvk] = map[string]sets.Set[metav1.ConditionStatus]{}
			}

			existingConditionsMap := GVKConditionErrorMapping[gvk]
			for condition, errorMapping := range newConditionsMap {
				existingConditionsMap[condition] = errorMapping
			}
			GVKConditionErrorMapping[gvk] = existingConditionsMap
		}
		logrus.Debugf("GVK Error Mapping Set")
		return
	}
	logrus.Debugf("GVK Error Mapping not provided, using predefined values")
}

func checkGeneration(obj data.Object, _ []Condition, summary Summary) Summary {
	if summary.State != "" {
		return summary
	}

	if summary.HasObservedGeneration {
		metadataGeneration, metadataFound, errMetadata := unstructured.NestedInt64(obj, "metadata", "generation")
		if errMetadata != nil {
			return summary
		}
		if !metadataFound {
			return summary
		}

		observedGeneration, _, errObserved := unstructured.NestedInt64(obj, "status", "observedGeneration")
		if errObserved != nil {
			return summary
		}

		if observedGeneration != metadataGeneration {
			summary.State = "in-progress"
			summary.Transitioning = true
		}
	}

	return summary
}

func checkOwner(obj data.Object, conditions []Condition, summary Summary) Summary {
	ustr := &unstructured.Unstructured{
		Object: obj,
	}
	for _, ownerref := range ustr.GetOwnerReferences() {
		rel := Relationship{
			Name:       ownerref.Name,
			Kind:       ownerref.Kind,
			APIVersion: ownerref.APIVersion,
			Type:       "owner",
			Inbound:    true,
		}
		if ownerref.Controller != nil && *ownerref.Controller {
			rel.ControlledBy = true
		}

		summary.Relationships = append(summary.Relationships, rel)
	}

	return summary
}

func checkStatusSummary(obj data.Object, _ []Condition, summary Summary) Summary {
	summaryObj := obj.Map("status", "display")
	if len(summaryObj) == 0 {
		summaryObj = obj.Map("status", "summary")
		if len(summaryObj) == 0 {
			return summary
		}
	}
	obj = summaryObj

	if _, ok := obj["state"]; ok {
		summary.State = obj.String("state")
	}
	if _, ok := obj["transitioning"]; ok {
		summary.Transitioning = obj.Bool("transitioning")
	}
	if _, ok := obj["error"]; ok {
		summary.Error = obj.Bool("error")
	}
	if _, ok := obj["message"]; ok {
		summary.Message = append(summary.Message, obj.String("message"))
	}

	return summary
}

func checkStandard(obj data.Object, _ []Condition, summary Summary) Summary {
	if summary.State != "" {
		return summary
	}

	// this is a hack to not call the standard summarizers on norman mapped objects
	if strings.HasPrefix(obj.String("type"), "/") {
		return summary
	}

	result, err := kstatus.Compute(&unstructured.Unstructured{Object: obj})
	if err != nil {
		return summary
	}

	switch result.Status {
	case kstatus.InProgressStatus:
		summary.State = "in-progress"
		summary.Message = append(summary.Message, result.Message)
		summary.Transitioning = true
	case kstatus.FailedStatus:
		summary.State = "failed"
		summary.Message = append(summary.Message, result.Message)
		summary.Error = true
	case kstatus.CurrentStatus:
		summary.State = "active"
		summary.Message = append(summary.Message, result.Message)
	case kstatus.TerminatingStatus:
		summary.State = "removing"
		summary.Message = append(summary.Message, result.Message)
		summary.Transitioning = true
	}

	return summary
}

func checkErrors(data data.Object, conditions []Condition, summary Summary) Summary {
	if len(conditions) == 0 {
		return summary
	}

	ustr := &unstructured.Unstructured{
		Object: data,
	}

	conditionMapping, found := GVKConditionErrorMapping[ustr.GroupVersionKind()]
	if !found {
		conditionMapping = GVKConditionErrorMapping[schema.GroupVersionKind{}]
	}

	for _, c := range conditions {
		status, found := conditionMapping[c.Type()]
		reasonIsError := c.Reason() == "Error"

		if !found && !reasonIsError {
			continue
		}

		if reasonIsError || status.Has(metav1.ConditionStatus(c.Status())) {
			summary.Error = true
			summary.Message = append(summary.Message, c.Message())
			if summary.State == "active" || summary.State == "" {
				summary.State = "error"
			}
		}

	}

	return summary
}

func checkTransitioning(obj data.Object, conditions []Condition, summary Summary) Summary {
	if isCAPIMachine(obj) {
		return checkCAPIMachineTransitioning(conditions, summary)
	}
	if isCAPIMachineSet(obj) || isCAPIMachineDeployment(obj) {
		return checkCAPIMachineSetAndDeploymentTransitioning(obj, conditions, summary)
	}
	if isCAPICluster(obj) {
		return checkCAPIClusterTransitioning(obj, conditions, summary)
	}
	return checkGenericTransitioning(obj, conditions, summary)
}

// checkCAPIMachineTransitioning computes summary state for CAPI Machine objects
// using the Ready (aggregate) and Reconciled conditions from v1beta2.
//
// The Ready condition is a composite condition whose message aggregates
// sub-condition issues as bullet points (e.g., "* InfrastructureReady: <detail>").
// The Reconciled condition is a Rancher-specific condition that indicates
// whether the machine's desired state has been fully applied.
//
// Priority order (first match wins):
//  1. Deleting=True -> state="deleting", transitioning=true
//  2. Paused=True   -> state="paused", transitioning=true
//  3. Reconciled present and NOT True -> state="reconciling",
//     message=Reconciled.message(if has message), transitioning=true(if status=Unknown), error=true(if status=False)
//  4. Ready=False (NotReady) — parse first bullet of Ready.message:
//     a. First bullet starts with "* BootstrapConfigReady:" -> state="waitingforinfrastructure"
//     b. First bullet starts with "* InfrastructureReady:"  -> state="waitingfornoderef"
//     c. Otherwise -> pass through (no state set)
//     Message is the stripped detail text from the first bullet.
//  5. Ready=Unknown -> state="reconciling", message=stripped detail from first
//     bullet of Ready.message, transitioning=true
//  6. Ready=True (or any other case) -> pass through, no state set
func checkCAPIMachineTransitioning(conditions []Condition, summary Summary) Summary {
	var reconciled *Condition
	var ready *Condition

	for i := range conditions {
		c := conditions[i]
		typ := c.Type()

		// Check Deleting and Paused first — these take absolute priority.
		if typ == "Deleting" && c.Status() == "True" {
			summary.State = "deleting"
			summary.Transitioning = true
			if msg := c.Message(); msg != "" {
				if strings.HasPrefix(msg, "Drain not completed yet") {
					msg = "Draining node"
				}
				summary.Message = append(summary.Message, msg)
			}
			return summary
		}
		if typ == "Paused" && c.Status() == "True" {
			summary.State = "paused"
			summary.Transitioning = true
			return summary
		}

		// Collect key conditions for priority evaluation below.
		switch typ {
		case "Reconciled":
			reconciled = &conditions[i]
		case "Ready":
			ready = &conditions[i]
		}
	}

	// Priority 3: Reconciled condition (when present and not True).
	if reconciled != nil && reconciled.Status() != "True" {
		switch reconciled.Status() {
		case "Unknown":
			summary.Transitioning = true
			summary.State = "reconciling"
		case "False":
			summary.Error = true
			summary.State = "reconciling"
		}
		if msg := reconciled.Message(); msg != "" {
			summary.Message = append(summary.Message, msg)
		}
		return summary
	}

	// Priority 4 & 5: Ready condition.
	if ready != nil {
		switch ready.Status() {
		case "False":
			// Parse the first bullet line to determine the state.
			detail, prefix := parseMessage(ready.Message())
			switch prefix {
			case "BootstrapConfigReady":
				summary.Transitioning = true
				summary.State = "waitingforinfrastructure"
				if strings.HasSuffix(detail, "status.initialization.dataSecretCreated is false") {
					detail = ""
				}
				if detail != "" {
					summary.Message = append(summary.Message, detail)
				}
			case "InfrastructureReady":
				summary.Transitioning = true
				summary.State = "waitingfornoderef"
				if strings.HasSuffix(detail, "status.initialization.provisioned is false") {
					detail = ""
				}
				if stepCounterMessagePattern.MatchString(detail) {
					detail = ""
				}
				if detail != "" {
					summary.Message = append(summary.Message, detail)
				}
			}
			// If the first bullet doesn't match known patterns, pass through.
			return summary

		case "Unknown":
			summary.Transitioning = true
			summary.State = "reconciling"
			detail, prefix := parseMessage(ready.Message())
			if prefix == "NodeHealthy" {
				if strings.HasSuffix(detail, "to report spec.providerID") {
					detail = ""
				}
			}
			if detail != "" {
				summary.Message = append(summary.Message, detail)
			}
			return summary
		}
		// Ready=True: pass through, let downstream summarizers handle it.
	}

	return summary
}

// checkCAPIMachineSetAndDeploymentTransitioning computes summary state for CAPI MachineSet
// and MachineDeployment objects using v1beta2 conditions and replica counts.
//
// The function reads spec.replicas, status.replicas, and status.readyReplicas
// to detect scaling operations, even when the controller hasn't yet updated
// the ScalingUp/ScalingDown conditions (stale observedGeneration).
//
// Priority order (first match wins):
//  1. Deleting=True                                          -> state="deleting", transitioning=true
//  2. Paused=True                                            -> state="paused", transitioning=true
//  3. RollingOut=True                                        -> state="rollingout", transitioning=true
//  4. ScalingDown=True OR status.replicas > spec.replicas    -> state="scalingdown", transitioning=true
//  5. ScalingUp=True OR spec.replicas > status.readyReplicas -> state="scalingup", transitioning=true
//  6. Otherwise                                              -> pass through (no state set)
func checkCAPIMachineSetAndDeploymentTransitioning(obj data.Object, conditions []Condition, summary Summary) Summary {
	// Read replica counts from the object. We use nestedInt64 instead of
	// unstructured.NestedInt64 because YAML/JSON decoders store numbers as
	// float64, not int64.
	specReplicas, specFound, _ := nestedInt64(obj, "spec", "replicas")
	statusReplicas, statusFound, _ := nestedInt64(obj, "status", "replicas")
	readyReplicas, readyFound, _ := nestedInt64(obj, "status", "readyReplicas")
	upToDateReplicas, upToDateFound, _ := nestedInt64(obj, "status", "upToDateReplicas")

	var scalingUp *Condition
	var scalingDown *Condition
	var rollingOut *Condition

	for i := range conditions {
		c := conditions[i]
		typ := c.Type()

		// Priority 1 & 2: Deleting and Paused take absolute priority.
		if typ == "Deleting" && c.Status() == "True" {
			summary.State = "deleting"
			summary.Transitioning = true
			if msg := c.Message(); msg != "" {
				summary.Message = append(summary.Message, msg)
			}
			return summary
		}
		if typ == "Paused" && c.Status() == "True" {
			summary.State = "paused"
			summary.Transitioning = true
			return summary
		}

		switch typ {
		case "ScalingUp":
			scalingUp = &conditions[i]
		case "ScalingDown":
			scalingDown = &conditions[i]
		case "RollingOut":
			rollingOut = &conditions[i]
		}
	}

	// Priority 3: Rolling out — RollingOut=True indicates a rolling upgrade
	// is in progress. This takes priority over ScalingDown/ScalingUp because
	// those are side effects of the rollout strategy (maxSurge creates extra
	// machines, then old ones are deleted). Only MachineDeployments have
	// rolling upgrades; MachineSets do not set this condition.
	if rollingOut != nil && rollingOut.Status() == "True" {
		summary.State = "rollingout"
		summary.Transitioning = true
		if statusFound && upToDateFound {
			notUpToDate := statusReplicas - upToDateReplicas
			summary.Message = append(summary.Message,
				fmt.Sprintf("rolling out %d not up-to-date replicas", notUpToDate))
		}
		return summary
	}

	// Priority 4: Scaling down — detected by condition OR replica count mismatch.
	scalingDownByCondition := scalingDown != nil && scalingDown.Status() == "True"
	scalingDownByReplicas := specFound && statusFound && statusReplicas > specReplicas
	if scalingDownByCondition || scalingDownByReplicas {
		summary.State = "scalingdown"
		summary.Transitioning = true
		if specFound && statusFound {
			summary.Message = append(summary.Message,
				fmt.Sprintf("Scaling down from %d to %d replicas, waiting for machines to be deleted", statusReplicas, specReplicas))
		}
		return summary
	}

	// Priority 5: Scaling up — detected by spec.replicas > status.readyReplicas.
	// This catches both the transient ScalingUp=True period and the long tail
	// where ScalingUp has already gone False but the new machine isn't ready yet.
	scalingUpByReplicas := specFound && readyFound && specReplicas > readyReplicas
	scalingUpByCondition := scalingUp != nil && scalingUp.Status() == "True"
	if scalingUpByReplicas || scalingUpByCondition {
		summary.State = "scalingup"
		summary.Transitioning = true
		if specFound && readyFound {
			summary.Message = append(summary.Message,
				fmt.Sprintf("Scaling up from %d to %d replicas, waiting for machines to be ready", readyReplicas, specReplicas))
		}
		return summary
	}

	return summary
}

// checkCAPIClusterTransitioning computes summary state for CAPI Cluster objects
// using v1beta2 conditions and worker replica counts.
//
// The Cluster object has conditions like Available, ScalingUp, ScalingDown,
// Deleting, Paused, WorkersAvailable, WorkerMachinesReady, ControlPlaneAvailable,
// ControlPlaneInitialized, etc. Worker replica counts are under status.workers.*.
//
// Priority order (first match wins):
//  1. Deleting=True                                                                    -> state="deleting", transitioning=true
//  2. Paused=True                                                                      -> state="paused", transitioning=true
//  3. RollingOut=True                                                                  -> state="rollingout", transitioning=true
//  4. ScalingDown=True OR status.workers.replicas > status.workers.desiredReplicas     -> state="updating", transitioning=true
//  5. ScalingUp=True OR status.workers.desiredReplicas > status.workers.readyReplicas  -> state="updating", transitioning=true
//  6. Available=False                                                                  -> state="updating", transitioning=true
//  7. Available=True (or any other case)                                               -> pass through (no state set)
func checkCAPIClusterTransitioning(obj data.Object, conditions []Condition, summary Summary) Summary {
	// Read worker replica counts from status.workers.*.
	desiredReplicas, desiredFound, _ := nestedInt64(obj, "status", "workers", "desiredReplicas")
	workersReplicas, workersFound, _ := nestedInt64(obj, "status", "workers", "replicas")
	readyReplicas, readyFound, _ := nestedInt64(obj, "status", "workers", "readyReplicas")
	upToDateReplicas, upToDateFound, _ := nestedInt64(obj, "status", "workers", "upToDateReplicas")

	var scalingUp *Condition
	var scalingDown *Condition
	var available *Condition
	var rollingOut *Condition

	for i := range conditions {
		c := conditions[i]
		typ := c.Type()

		// Priority 1 & 2: Deleting and Paused take absolute priority.
		if typ == "Deleting" && c.Status() == "True" {
			summary.State = "deleting"
			summary.Transitioning = true
			if msg := c.Message(); msg != "" {
				if strings.HasPrefix(msg, "* MachineDeployments") {
					msg = "waiting for workers deletion"
				}
				summary.Message = append(summary.Message, msg)
			}
			return summary
		}
		if typ == "Paused" && c.Status() == "True" {
			summary.State = "paused"
			summary.Transitioning = true
			return summary
		}

		switch typ {
		case "ScalingUp":
			scalingUp = &conditions[i]
		case "ScalingDown":
			scalingDown = &conditions[i]
		case "Available":
			available = &conditions[i]
		case "RollingOut":
			rollingOut = &conditions[i]
		}
	}

	// Priority 3: Rolling out — RollingOut=True indicates one or more
	// MachineDeployments are performing a rolling upgrade. This takes
	// priority over ScalingDown/ScalingUp because those are side effects
	// of the rollout strategy.
	if rollingOut != nil && rollingOut.Status() == "True" {
		summary.State = "rollingout"
		summary.Transitioning = true
		if workersFound && upToDateFound {
			notUpToDate := workersReplicas - upToDateReplicas
			summary.Message = append(summary.Message,
				fmt.Sprintf("rolling out %d not up-to-date replicas", notUpToDate))
		}
		return summary
	}

	// Priority 4: Scaling down — detected by condition OR replica count mismatch.
	scalingDownByCondition := scalingDown != nil && scalingDown.Status() == "True"
	scalingDownByReplicas := desiredFound && workersFound && workersReplicas > desiredReplicas
	if scalingDownByCondition || scalingDownByReplicas {
		summary.State = "updating"
		summary.Transitioning = true
		if desiredFound && workersFound {
			summary.Message = append(summary.Message,
				fmt.Sprintf("Scaling down from %d to %d machines", workersReplicas, desiredReplicas))
		}
		return summary
	}

	// Priority 5: Scaling up — Scaling up — detected by condition OR desired > readyReplicas.
	scalingUpByCondition := scalingUp != nil && scalingUp.Status() == "True"
	scalingUpByReplicas := desiredFound && readyFound && desiredReplicas > readyReplicas
	if scalingUpByCondition || scalingUpByReplicas {
		summary.State = "updating"
		summary.Transitioning = true
		if desiredFound && readyFound {
			summary.Message = append(summary.Message,
				fmt.Sprintf("Scaling up from %d to %d machines", readyReplicas, desiredReplicas))
		}
		return summary
	}

	// Priority 5: Available=False — the cluster is not yet available.
	if available != nil {
		switch available.Status() {
		case "False":
			summary.Transitioning = true
			summary.State = "updating"
			// Parse the first bullet line to determine the state.
			detail, prefix := parseMessage(available.Message())
			switch prefix {
			case "RemoteConnectionProbe":
				summary.Message = append(summary.Message, "establishing connection to control plane")
			case "ControlPlaneAvailable":
				summary.Message = append(summary.Message, "Waiting for control plane to be available")
			case "MachineDeployment":
				summary.Message = append(summary.Message, "Waiting for workers to be available")
			default:
				// If the first bullet doesn't match known patterns, pass through.
				if detail != "" {
					summary.Message = append(summary.Message, detail)
				}
			}

		case "Unknown":
			summary.Error = true
			summary.State = "unavailable"
			summary.Message = append(summary.Message, available.Message())
		}
		// Ready=True: pass through, let downstream summarizers handle it.
	}

	// Priority 6: Available=True or no conditions — pass through.
	return summary
}

func checkGenericTransitioning(_ data.Object, conditions []Condition, summary Summary) Summary {
	for _, c := range conditions {
		newState, ok := TransitioningUnknown[c.Type()]
		if !ok {
			continue
		}

		if newState == reason {
			newState = c.Reason()
		}

		if c.Status() == "False" {
			summary.Error = true
			summary.State = newState
			summary.Message = append(summary.Message, c.Message())
		} else if c.Status() == "Unknown" && summary.State == "" {
			summary.Transitioning = true
			summary.State = newState
			summary.Message = append(summary.Message, c.Message())
		}
	}

	for _, c := range conditions {
		if summary.State != "" {
			break
		}
		newState, ok := TransitioningTrue[c.Type()]
		if !ok {
			continue
		}

		if newState == reason {
			newState = c.Reason()
		}

		if c.Status() == "True" {
			summary.Transitioning = true
			summary.State = newState
			summary.Message = append(summary.Message, c.Message())
		}
	}

	ready := true
	readyMessage := ""
	for _, c := range conditions {
		if summary.State != "" {
			break
		}

		if c.Type() == "Ready" && c.Status() == "False" {
			ready = false
			readyMessage = c.Message()

			// ignore the message that is in the form of "x of y completed",
			// seen on AWSMachine.infrastructure.cluster.x-k8s.io/v1beta2
			if stepCounterMessagePattern.MatchString(c.Message()) {
				readyMessage = ""
			}
			continue
		}
		newState, ok := TransitioningFalse[c.Type()]
		if !ok {
			continue
		}
		if newState == reason {
			newState = c.Reason()
		}
		if c.Status() == "False" {
			summary.Transitioning = true
			summary.State = newState
			summary.Message = append(summary.Message, c.Message())
		} else if c.Status() == "Unknown" {
			summary.Error = true
			summary.State = newState
			summary.Message = append(summary.Message, c.Message())
		}
	}

	if summary.State == "" && !ready {
		summary.Transitioning = true
		summary.State = "unavailable"
		summary.Message = append(summary.Message, readyMessage)
	}

	return summary
}

func checkActive(obj data.Object, _ []Condition, summary Summary) Summary {
	if summary.State != "" {
		return summary
	}

	switch obj.String("spec", "active") {
	case "true":
		summary.State = "active"
	case "false":
		summary.State = "inactive"
	}

	return summary
}

func checkPhase(obj data.Object, _ []Condition, summary Summary) Summary {
	phase := obj.String("status", "phase")
	if phase == "Succeeded" {
		summary.State = "succeeded"
		summary.Transitioning = false
	} else if phase == "Bound" {
		summary.State = "bound"
		summary.Transitioning = false
	} else if phase != "" && summary.State == "" {
		summary.State = phase
	}
	return summary
}

func checkInitializing(obj data.Object, conditions []Condition, summary Summary) Summary {
	apiVersion := obj.String("apiVersion")
	_, hasConditions := obj.Map("status")["conditions"]
	if summary.State == "" && hasConditions && len(conditions) == 0 && strings.Contains(apiVersion, "cattle.io") {
		val := obj.String("metadata", "created")
		if i, err := convert.ToTimestamp(val); err == nil {
			if time.Unix(i/1000, 0).Add(5 * time.Second).After(time.Now()) {
				summary.State = "initializing"
				summary.Transitioning = true
			}
		}
	}
	return summary
}

func checkRemoving(obj data.Object, conditions []Condition, summary Summary) Summary {
	removed := obj.String("metadata", "removed")
	if removed == "" {
		return summary
	}

	summary.State = "removing"
	summary.Transitioning = true

	finalizers := obj.StringSlice("metadata", "finalizers")
	if len(finalizers) == 0 {
		finalizers = obj.StringSlice("spec", "finalizers")
	}

	for _, cond := range conditions {
		if cond.Type() == "Removed" && (cond.Status() == "Unknown" || cond.Status() == "False") && cond.Message() != "" {
			summary.Message = append(summary.Message, cond.Message())
		}
	}

	if len(finalizers) == 0 {
		return summary
	}

	_, f := kv.RSplit(finalizers[0], "controller.cattle.io/")
	if f == "foregroundDeletion" {
		f = "object cleanup"
	}

	summary.Message = append(summary.Message, "waiting on "+f)
	if i, err := convert.ToTimestamp(removed); err == nil {
		if time.Unix(i/1000, 0).Add(5 * time.Minute).Before(time.Now()) {
			summary.Error = true
		}
	}

	return summary
}

func checkLoadBalancer(obj data.Object, _ []Condition, summary Summary) Summary {
	if (summary.State == "active" || summary.State == "") &&
		obj.String("kind") == "Service" &&
		(obj.String("spec", "serviceKind") == "LoadBalancer" ||
			obj.String("spec", "type") == "LoadBalancer") {
		addresses := obj.Slice("status", "loadBalancer", "ingress")
		if len(addresses) == 0 {
			summary.State = "pending"
			summary.Transitioning = true
			summary.Message = append(summary.Message, "Load balancer is being provisioned")
		}
	}

	return summary
}

func isKind(obj data.Object, kind string, apiGroups ...string) bool {
	if obj.String("kind") != kind {
		return false
	}

	if len(apiGroups) == 0 {
		return obj.String("apiVersion") == "v1"
	}

	if len(apiGroups) == 0 {
		apiGroups = []string{""}
	}

	for _, group := range apiGroups {
		switch {
		case group == "":
			if obj.String("apiVersion") == "v1" {
				return true
			}
		case group[len(group)-1] == '/':
			if strings.HasPrefix(obj.String("apiVersion"), group) {
				return true
			}
		default:
			if obj.String("apiVersion") != group {
				return true
			}
		}
	}

	return false
}

func checkApplyOwned(obj data.Object, conditions []Condition, summary Summary) Summary {
	if len(obj.Slice("metadata", "ownerReferences")) > 0 {
		return summary
	}

	annotations := obj.Map("metadata", "annotations")
	gvkString := convert.ToString(annotations["objectset.rio.cattle.io/owner-gvk"])
	i := strings.Index(gvkString, kindSep)
	if i <= 0 {
		return summary
	}

	name := convert.ToString(annotations["objectset.rio.cattle.io/owner-name"])
	namespace := convert.ToString(annotations["objectset.rio.cattle.io/owner-namespace"])

	apiVersion := gvkString[:i]
	kind := gvkString[i+len(kindSep):]

	rel := Relationship{
		Name:       name,
		Namespace:  namespace,
		Kind:       kind,
		APIVersion: apiVersion,
		Type:       "applies",
		Inbound:    true,
	}

	summary.Relationships = append(summary.Relationships, rel)

	return summary
}

// isCAPIMachine returns true if the object is a CAPI Machine (cluster.x-k8s.io/*/Machine).
func isCAPIMachine(obj data.Object) bool {
	return strings.HasPrefix(obj.String("apiVersion"), "cluster.x-k8s.io/") &&
		obj.String("kind") == "Machine"
}

// isCAPIMachineSet returns true if the object is a CAPI MachineSet (cluster.x-k8s.io/*/MachineSet).
func isCAPIMachineSet(obj data.Object) bool {
	return strings.HasPrefix(obj.String("apiVersion"), "cluster.x-k8s.io/") &&
		obj.String("kind") == "MachineSet"
}

// isCAPIMachineDeployment returns true if the object is a CAPI MachineDeployment (cluster.x-k8s.io/*/MachineDeployment).
func isCAPIMachineDeployment(obj data.Object) bool {
	return strings.HasPrefix(obj.String("apiVersion"), "cluster.x-k8s.io/") &&
		obj.String("kind") == "MachineDeployment"
}

// isCAPICluster returns true if the object is a CAPI Cluster (cluster.x-k8s.io/*/Cluster).
func isCAPICluster(obj data.Object) bool {
	return strings.HasPrefix(obj.String("apiVersion"), "cluster.x-k8s.io/") &&
		obj.String("kind") == "Cluster"
}

// parseMessage parses the first bullet line from a condition message.
// The condition message format is:
//
//	\* <ConditionType>: <detail message>
//	\* <ConditionType>: <detail message>
//
// or with nested sub-bullets when the top-level detail is empty:
//
//	\* <ConditionType>:
//	  \* <SubCondition>: <detail message>
//
// Multiple lines are separated by newlines. This function returns:
//   - detail: the stripped detail text (without "* ConditionType: " prefix).
//     When the top-level bullet has no inline detail, the detail from
//     the first indented sub-bullet is returned instead.
//   - prefix: the condition type name from the top-level bullet.
func parseMessage(message string) (detail, prefix string) {
	if message == "" {
		return "", ""
	}

	lines := strings.Split(message, "\n")

	// Parse the first line.
	firstLine := strings.TrimPrefix(lines[0], "* ")

	// Split on ": " to separate condition type from detail.
	colonIdx := strings.Index(firstLine, ": ")
	if colonIdx < 0 {
		// No ": " separator. Check if the line ends with ":" (empty detail
		// with sub-bullets on subsequent lines).
		if strings.HasSuffix(firstLine, ":") {
			prefix = strings.TrimSuffix(firstLine, ":")
		} else {
			return firstLine, ""
		}
	} else {
		prefix = firstLine[:colonIdx]
		detail = firstLine[colonIdx+2:]
	}

	// If the top-level bullet has inline detail, return it.
	if detail != "" {
		return detail, prefix
	}

	// The top-level bullet has no inline detail (e.g., "* NodeHealthy:").
	// Look for indented sub-bullets to extract the detail from.
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "* ") {
			continue
		}
		// Found a sub-bullet: "* SubCondition: detail message"
		subContent := strings.TrimPrefix(trimmed, "* ")
		subColonIdx := strings.Index(subContent, ": ")
		if subColonIdx >= 0 {
			detail = subContent[subColonIdx+2:]
		} else {
			detail = subContent
		}
		break // Only use the first sub-bullet.
	}

	return detail, prefix
}

// nestedInt64 retrieves a nested numeric field from an unstructured object and
// returns it as int64. Unlike unstructured.NestedInt64, which requires the value
// to be exactly int64, this function also handles float64 (produced by JSON/YAML
// decoders), json.Number, and string representations of integers.
func nestedInt64(obj data.Object, fields ...string) (int64, bool, error) {
	val, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return 0, found, err
	}
	n, err := convert.ToNumber(val)
	if err != nil {
		return 0, false, err
	}
	return n, true, nil
}
