package summary

import (
	"encoding/json"
	"os"
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
		"Reconciled":                  "reconciling",
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
		"Completed":           "activating",
		"Ready":               "unavailable",
		"Available":           "updating",
		"BootstrapReady":      reason,
		"InfrastructureReady": reason,
		"NodeHealthy":         reason,
	}

	// True == transitioning
	// False ==
	// Unknown == error
	TransitioningTrue = map[string]string{
		"Reconciling": "reconciling",
	}

	Summarizers          []Summarizer
	ConditionSummarizers []Summarizer
)

type Summarizer func(obj data.Object, conditions []Condition, summary Summary) Summary

func init() {
	ConditionSummarizers = []Summarizer{
		checkErrors,
		checkTransitioning,
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

func checkTransitioning(_ data.Object, conditions []Condition, summary Summary) Summary {
	for _, c := range conditions {
		newState, ok := TransitioningUnknown[c.Type()]
		if !ok {
			continue
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
