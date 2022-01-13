package pod

import (
	"encoding/json"
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
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
}

func NewMutator(settingCache v1beta1.SettingCache) types.Mutator {
	return &podMutator{
		setttingCache: settingCache,
	}
}

// podMutator injects Harvester settings like http proxy envs and trusted CA certs to system pods that may access
// external services. It includes harvester apiserver and longhorn backing-image-data-source pods.
type podMutator struct {
	types.DefaultMutator
	setttingCache v1beta1.SettingCache
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
