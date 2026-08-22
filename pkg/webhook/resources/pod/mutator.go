package pod

import (
	"encoding/json"
	"fmt"
	"strings"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	harvSettings "github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

var matchingLabelsForNamespace = map[string][]labels.Set{
	util.LonghornSystemNamespaceName: {
		{
			"longhorn.io/component": "backing-image-data-source",
		},
	},
	util.HarvesterSystemNamespaceName: {
		{
			"app.kubernetes.io/name":      "harvester",
			"app.kubernetes.io/component": "apiserver",
		},
	},
	util.CattleSystemNamespaceName: {
		{
			"app": "rancher",
		},
	},
}

const (
	forkliftAppKey                     = "forklift.app"
	forkliftAppVal                     = "virt-v2v"
	shareManagerKind                   = "ShareManager"
	shareManagerPodStaticIPAnnotation  = util.ShareManagerStaticIPAnno
	shareManagerPodIfaceAnnotation     = util.ShareManagerIfaceAnno
	shareManagerPodIPAnnotation        = util.ShareManagerIPAnno
	shareManagerPodMACAnnotation       = util.ShareManagerMACAnno
	shareManagerPodStaticNADAnnotation = util.ShareManagerStaticNADAnno
)

func NewMutator(settingCache v1beta1.SettingCache, shareManagerCache ctllonghornv1.ShareManagerCache) types.Mutator {
	return &podMutator{
		settingCache:      settingCache,
		shareManagerCache: shareManagerCache,
	}
}

// podMutator injects Harvester settings like http proxy envs and trusted CA certs to system pods that may access
// external services. It includes harvester apiserver and longhorn backing-image-data-source pods.
type podMutator struct {
	types.DefaultMutator
	settingCache      v1beta1.SettingCache
	shareManagerCache ctllonghornv1.ShareManagerCache
}

func newResource(ops []admissionregv1.OperationType) types.Resource {
	return types.Resource{
		Names:          []string{string(corev1.ResourcePods)},
		Scope:          admissionregv1.NamespacedScope,
		APIGroup:       corev1.SchemeGroupVersion.Group,
		APIVersion:     corev1.SchemeGroupVersion.Version,
		ObjectType:     &corev1.Pod{},
		OperationTypes: ops,
	}
}

func (m *podMutator) Resource() types.Resource {
	return newResource([]admissionregv1.OperationType{
		admissionregv1.Create,
	})
}

func (m *podMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	pod := newObj.(*corev1.Pod)

	if isForkliftV2VPod(pod) {
		return generateForkliftV2VPatch(pod)
	}

	if isShareManagerPod(pod) {
		return m.shareManagerNetworkPatches(pod)
	}

	if !shouldPatch(pod) {
		return nil, nil
	}

	var patchOps types.PatchOps
	httpProxyPatches, err := m.httpProxyPatches(pod)
	if err != nil {
		return nil, err
	}
	patchOps = append(patchOps, httpProxyPatches...)
	additionalCAPatches, err := m.additionalCAPatches(pod)
	if err != nil {
		return nil, err
	}
	patchOps = append(patchOps, additionalCAPatches...)

	return patchOps, nil
}

func (m *podMutator) shareManagerNetworkPatches(pod *corev1.Pod) (types.PatchOps, error) {
	if m.shareManagerCache == nil {
		return nil, nil
	}

	shareManagerName, ok := shareManagerOwnerName(pod)
	if !ok {
		return nil, nil
	}

	shareManager, err := m.shareManagerCache.Get(pod.Namespace, shareManagerName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	shareManagerAnnotations := shareManager.Annotations
	if !strings.EqualFold(shareManagerAnnotations[shareManagerPodStaticIPAnnotation], "true") {
		return nil, nil
	}

	nad := shareManagerAnnotations[shareManagerPodStaticNADAnnotation]
	ip := shareManagerAnnotations[shareManagerPodIPAnnotation]
	mac := shareManagerAnnotations[shareManagerPodMACAnnotation]
	iface := shareManagerAnnotations[shareManagerPodIfaceAnnotation]
	if nad == "" || ip == "" || mac == "" || iface == "" {
		return nil, nil
	}

	podAnnotations := pod.Annotations
	networksAnnotation, changed, err := mutateNetworksAnnotation(podAnnotations[networkv1.NetworkAttachmentAnnot], nad, ip, mac, iface)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil
	}

	patchOp, err := stringAnnotationPatch(podAnnotations, networkv1.NetworkAttachmentAnnot, networksAnnotation)
	if err != nil {
		return nil, err
	}

	return types.PatchOps{patchOp}, nil
}

func (m *podMutator) httpProxyPatches(pod *corev1.Pod) (types.PatchOps, error) {
	proxySetting, err := m.settingCache.Get(harvSettings.HTTPProxySettingName)
	if err != nil || proxySetting.Value == "" {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	var httpProxyConfig util.HTTPProxyConfig
	if err := json.Unmarshal([]byte(proxySetting.Value), &httpProxyConfig); err != nil {
		return nil, err
	}
	if httpProxyConfig.HTTPProxy == "" && httpProxyConfig.HTTPSProxy == "" && httpProxyConfig.NoProxy == "" {
		return nil, nil
	}

	var proxyEnvs = []corev1.EnvVar{
		{
			Name:  util.HTTPProxyEnv,
			Value: httpProxyConfig.HTTPProxy,
		},
		{
			Name:  util.HTTPSProxyEnv,
			Value: httpProxyConfig.HTTPSProxy,
		},
		{
			Name:  util.NoProxyEnv,
			Value: util.AddBuiltInNoProxy(httpProxyConfig.NoProxy),
		},
	}
	var patchOps types.PatchOps
	for idx, container := range pod.Spec.Containers {
		envPatches, err := envPatches(container.Env, proxyEnvs, fmt.Sprintf("/spec/containers/%d/env", idx))
		if err != nil {
			return nil, err
		}
		patchOps = append(patchOps, envPatches...)
	}
	return patchOps, nil
}

func envPatches(target, envVars []corev1.EnvVar, basePath string) (types.PatchOps, error) {
	var (
		patchOps types.PatchOps
		value    interface{}
		path     string
		first    = len(target) == 0
	)
	for _, envVar := range envVars {
		if first {
			first = false
			path = basePath
			value = []corev1.EnvVar{envVar}
		} else {
			path = basePath + "/-"
			value = envVar
		}
		valueStr, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "add", "path": "%s", "value": %s}`, path, valueStr))
	}
	return patchOps, nil
}

func (m *podMutator) additionalCAPatches(pod *corev1.Pod) (types.PatchOps, error) {
	additionalCASetting, err := m.settingCache.Get("additional-ca")
	if err != nil || additionalCASetting.Value == "" {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	var (
		additionalCAvolume = corev1.Volume{
			Name: "additional-ca-volume",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: ptr.To(int32(400)),
					SecretName:  util.AdditionalCASecretName,
				},
			},
		}
		additionalCAVolumeMount = corev1.VolumeMount{
			Name:      "additional-ca-volume",
			MountPath: "/etc/ssl/certs/" + util.AdditionalCAFileName,
			SubPath:   util.AdditionalCAFileName,
			ReadOnly:  true,
		}
		patchOps types.PatchOps
	)

	volumePatch, err := volumePatch(pod.Spec.Volumes, additionalCAvolume)
	if err != nil {
		return nil, err
	}
	patchOps = append(patchOps, volumePatch)

	for idx, container := range pod.Spec.Containers {
		volumeMountPatch, err := volumeMountPatch(container.VolumeMounts, fmt.Sprintf("/spec/containers/%d/volumeMounts", idx), additionalCAVolumeMount)
		if err != nil {
			return nil, err
		}
		patchOps = append(patchOps, volumeMountPatch)
	}

	return patchOps, nil
}

func volumePatch(target []corev1.Volume, volume corev1.Volume) (string, error) {
	var (
		value      interface{} = []corev1.Volume{volume}
		path                   = "/spec/volumes"
		first                  = len(target) == 0
		valueBytes []byte
		err        error
	)
	if !first {
		value = volume
		path = path + "/-"
	}
	valueBytes, err = json.Marshal(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"op": "add", "path": "%s", "value": %s}`, path, valueBytes), nil
}

func volumeMountPatch(target []corev1.VolumeMount, path string, volumeMount corev1.VolumeMount) (string, error) {
	var (
		value interface{} = []corev1.VolumeMount{volumeMount}
		first             = len(target) == 0
	)
	if !first {
		path = path + "/-"
		value = volumeMount
	}
	valueStr, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"op": "add", "path": "%s", "value": %s}`, path, valueStr), nil
}

func shouldPatch(pod *corev1.Pod) bool {
	matchingLabels, exists := matchingLabelsForNamespace[pod.Namespace]
	if !exists {
		return false
	}

	podLabels := labels.Set(pod.Labels)
	for _, ls := range matchingLabels {
		if ls.AsSelector().Matches(podLabels) {
			return true
		}
	}
	return false
}

func shareManagerOwnerName(pod *corev1.Pod) (string, bool) {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == shareManagerKind && ownerRef.Name != "" {
			return ownerRef.Name, true
		}
	}
	return "", false
}

func isShareManagerPod(pod *corev1.Pod) bool {
	_, ok := shareManagerOwnerName(pod)
	return ok
}

func mutateNetworksAnnotation(annotation, nad, ip, mac, iface string) (string, bool, error) {
	networkName, networkNamespace, err := parseNADNamespacedName(nad)
	if err != nil {
		return "", false, err
	}

	selections := []networkv1.NetworkSelectionElement{}
	if annotation != "" {
		if err := json.Unmarshal([]byte(annotation), &selections); err != nil {
			return "", false, err
		}
	}

	desiredIPs := []string{ip}
	changed := false
	found := false

	for i := range selections {
		if selections[i].InterfaceRequest != iface {
			continue
		}

		found = true
		if selections[i].Name == networkName &&
			selections[i].Namespace == networkNamespace &&
			stringSlicesEqual(selections[i].IPRequest, desiredIPs) &&
			selections[i].MacRequest == mac &&
			selections[i].IPAMClaimReference == "" {
			continue
		}

		selections[i].Name = networkName
		selections[i].Namespace = networkNamespace
		selections[i].IPRequest = desiredIPs
		selections[i].MacRequest = mac
		selections[i].IPAMClaimReference = ""
		changed = true
	}

	if !found {
		return "", false, nil
	}

	if !changed {
		return "", false, nil
	}

	value, err := json.Marshal(selections)
	if err != nil {
		return "", false, err
	}
	return string(value), true, nil
}

func parseNADNamespacedName(nad string) (string, string, error) {
	parts := strings.Split(nad, "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return "", "", fmt.Errorf("invalid network attachment definition name %q", nad)
		}
		return parts[0], "", nil
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("invalid network attachment definition name %q", nad)
		}
		return parts[1], parts[0], nil
	default:
		return "", "", fmt.Errorf("invalid network attachment definition name %q", nad)
	}
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func stringAnnotationPatch(annotations map[string]string, key, value string) (string, error) {
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	if len(annotations) == 0 {
		annotationsValue, err := json.Marshal(map[string]string{key: value})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"op": "add", "path": "/metadata/annotations", "value": %s}`, annotationsValue), nil
	}

	path := fmt.Sprintf("/metadata/annotations/%s", escapeJSONPointerToken(key))
	if _, exists := annotations[key]; exists {
		return fmt.Sprintf(`{"op": "replace", "path": "%s", "value": %s}`, path, valueBytes), nil
	}

	return fmt.Sprintf(`{"op": "add", "path": "%s", "value": %s}`, path, valueBytes), nil
}

func escapeJSONPointerToken(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
}

// identifies if pod is a forklift virt-v2v conversion pod
func isForkliftV2VPod(pod *corev1.Pod) bool {
	if pod.Labels == nil {
		return false
	}

	if val, ok := pod.Labels[forkliftAppKey]; ok && val == forkliftAppVal {
		return true
	}
	return false
}

// generateForkliftV2VPatch amends the SecurityContext on the virt-v2v pod to allow it to run on harvester
// it also patches the ImagePullPolicy on the containers to `IfNotPresent`
func generateForkliftV2VPatch(pod *corev1.Pod) (types.PatchOps, error) {
	var patchOps types.PatchOps
	podSecurityContext := corev1.PodSecurityContext{
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeUnconfined,
		},
	}
	podSecurityContextString, err := json.Marshal(podSecurityContext)
	if err != nil {
		return nil, fmt.Errorf("error generating pod seccomp patch for virt-v2v pods: %w", err)
	}
	podSecurityContextPath := "/spec/securityContext"
	patch := fmt.Sprintf(`{"op": "replace", "path": "%s", "value": %s}`, podSecurityContextPath, podSecurityContextString)
	securityContext := corev1.SecurityContext{
		Privileged: ptr.To(true),
	}
	patchOps = append(patchOps, patch)

	securityContextString, err := json.Marshal(securityContext)
	if err != nil {
		return nil, fmt.Errorf("error generating container security context: %w", err)
	}

	for i := range pod.Spec.Containers {
		containerSecurityContextPath := fmt.Sprintf("/spec/containers/%d/securityContext", i)
		patch := fmt.Sprintf(`{"op": "replace", "path": "%s", "value": %s}`, containerSecurityContextPath, securityContextString)
		patchOps = append(patchOps, patch)
		imagePullPolicyPath := fmt.Sprintf("/spec/containers/%d/imagePullPolicy", i)
		patch = fmt.Sprintf(`{"op": "replace", "path": "%s", "value": "%s"}`, imagePullPolicyPath, corev1.PullIfNotPresent)
		patchOps = append(patchOps, patch)
	}

	return patchOps, nil
}
