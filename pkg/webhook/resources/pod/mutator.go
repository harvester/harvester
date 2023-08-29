package pod

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

var matchingLabels = []labels.Set{
	{
		"longhorn.io/component": "backing-image-data-source",
	},
	{
		"app.kubernetes.io/name":      "harvester",
		"app.kubernetes.io/component": "apiserver",
	},
	{
		"app": "rancher",
	},
	{
		"kubevirt.io": "virt-launcher",
	},
}

func NewMutator(settingCache v1beta1.SettingCache, vmCache ctlkubevirtv1.VirtualMachineCache) types.Mutator {
	return &podMutator{
		setttingCache: settingCache,
		vmCache:       vmCache,
	}
}

const (
	kubeVirtLabelKey   = "kubevirt.io"
	kubeVirtLabelValue = "virt-launcher"
	CapNetAdmin        = "NET_ADMIN"
	CapNetRaw          = "NET_RAW"
	CapNetBindService  = "NET_BIND_SERVICE"
	CapSysPtrace       = "SYS_PTRACE"
	CapSysNice         = "SYS_NICE"
	vmLabelPrefix      = "harvesterhci.io/vmName"
)

// podMutator injects Harvester settings like http proxy envs and trusted CA certs to system pods that may access
// external services. It includes harvester apiserver and longhorn backing-image-data-source pods.
type podMutator struct {
	types.DefaultMutator
	setttingCache v1beta1.SettingCache
	vmCache       ctlkubevirtv1.VirtualMachineCache
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

func (m *podMutator) Create(request *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	pod := newObj.(*corev1.Pod)

	podLabels := labels.Set(pod.Labels)
	var match bool
	for _, v := range matchingLabels {
		if v.AsSelector().Matches(podLabels) {
			match = true
			break
		}
	}
	if !match {
		logrus.Infof("skipping pod %s due to missing label match", pod.GetName())
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
	netAdminPatches, err := m.sideCarPatches(pod)
	if err != nil {
		return nil, err
	}
	patchOps = append(patchOps, netAdminPatches...)

	return patchOps, nil
}

func (m *podMutator) httpProxyPatches(pod *corev1.Pod) (types.PatchOps, error) {
	proxySetting, err := m.setttingCache.Get("http-proxy")
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
	additionalCASetting, err := m.setttingCache.Get("additional-ca")
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
					DefaultMode: pointer.Int32(400),
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

// hack to add CapNetAdmin permission to virt-launcher pods.
// also injects a sidecar if needed to watch and apply security groups
// which allows sidecar to run the iptable rule application
func (m *podMutator) sideCarPatches(pod *corev1.Pod) (types.PatchOps, error) {
	val, ok := pod.Labels[kubeVirtLabelKey]
	if !ok || val != kubeVirtLabelValue {
		logrus.Debugf("pod %s missing kubevirt key/value label", pod.GetName())
		return nil, nil
	}

	vmName, ok := pod.Labels[vmLabelPrefix]
	if !ok || vmName == "" {
		logrus.Debugf("pod %s missing vmName label. ignoring", pod.Name)
		return nil, nil
	}

	vmObj, err := m.vmCache.Get(pod.Namespace, vmName)
	if err != nil {
		return nil, err
	}

	attachedSecurityGroup, ok := vmObj.Annotations[harvesterv1beta1.SecurityGroupPrefix]
	if !ok || attachedSecurityGroup == "" {
		logrus.Debugf("vm %s has no securiy group attached. ignoring", vmObj.Name)
		return nil, err
	}

	sidecarImageSetting, err := m.setttingCache.Get(settings.VMNetworkPolicySideCarSettingName)
	if err != nil {
		return nil, err
	}

	logrus.Infof("attempting to patch pod spec: %s", pod.GetName())
	capAddPath := "/spec/containers/0/securityContext/capabilities/add/-"
	generatedPatch := make([]string, 0)
	for _, v := range []string{CapNetAdmin, CapNetRaw} {
		patch := fmt.Sprintf(`{"op":"add", "path":"%s", "value": "%s"}`, capAddPath, v)
		generatedPatch = append(generatedPatch, patch)
	}
	capRemovePath := "/spec/containers/0/securityContext/capabilities/drop/0"
	patch := fmt.Sprintf(`{"op":"remove", "path":"%s"}`, capRemovePath)
	generatedPatch = append(generatedPatch, patch)
	patch, err = generateNetworkPolicySidecar(vmObj.Name, vmObj.Namespace, sidecarImageSetting)
	if err != nil {
		return nil, err
	}
	generatedPatch = append(generatedPatch, patch)
	automountSAPath := "/spec/automountServiceAccountToken"
	patch = fmt.Sprintf(`{"op":"replace", "path":"%s", "value": %v}`, automountSAPath, true)
	generatedPatch = append(generatedPatch, patch)
	logrus.Debugf("generated patch for pod %s: %s", pod.GetName(), generatedPatch)
	return generatedPatch, nil
}

func generateNetworkPolicySidecar(name, namespace string, imageSetting *harvesterv1beta1.Setting) (string, error) {
	imageJSON := imageSetting.Default
	if imageSetting.Value != "" {
		imageJSON = imageSetting.Value
	}

	imageObj := &settings.Image{}
	if err := json.Unmarshal([]byte(imageJSON), imageObj); err != nil {
		return "", err
	}

	container := corev1.Container{
		Name:  "vmnetworkpolicy",
		Image: fmt.Sprintf("%s:%s", imageObj.Repository, imageObj.Tag),
		Env: []corev1.EnvVar{
			{
				Name:  "HARVESTER_VM_NAME",
				Value: name,
			},
			{
				Name:  "HARVESTER_VM_NAMESPACE",
				Value: namespace,
			},
		},
		ImagePullPolicy: imageObj.ImagePullPolicy,
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					CapNetAdmin,
					CapNetBindService,
					CapSysNice,
					CapSysPtrace,
					CapNetRaw,
				},
			},
		},
	}

	patch, err := json.Marshal(map[string]any{
		"op":    "add",
		"path":  "/spec/containers/-",
		"value": container,
	})
	if err != nil {
		logrus.Errorf("error marshalling container spec: %v", err)
		return "", err
	}
	return string(patch), nil
}
