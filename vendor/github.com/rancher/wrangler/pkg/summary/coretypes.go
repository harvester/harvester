package summary

import (
	"github.com/rancher/wrangler/pkg/data"
	"github.com/rancher/wrangler/pkg/data/convert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func checkHasPodTemplate(obj data.Object, condition []Condition, summary Summary) Summary {
	template := obj.Map("spec", "template")
	if template == nil {
		return summary
	}

	if !isKind(obj, "ReplicaSet", "apps/", "extension/") &&
		!isKind(obj, "DaemonSet", "apps/", "extension/") &&
		!isKind(obj, "StatefulSet", "apps/", "extension/") &&
		!isKind(obj, "Deployment", "apps/", "extension/") &&
		!isKind(obj, "Job", "batch/") &&
		!isKind(obj, "Service") {
		return summary
	}

	return checkPodTemplate(template, condition, summary)
}

func checkHasPodSelector(obj data.Object, condition []Condition, summary Summary) Summary {
	selector := obj.Map("spec", "selector")
	if selector == nil {
		return summary
	}

	if !isKind(obj, "ReplicaSet", "apps/", "extension/") &&
		!isKind(obj, "DaemonSet", "apps/", "extension/") &&
		!isKind(obj, "StatefulSet", "apps/", "extension/") &&
		!isKind(obj, "Deployment", "apps/", "extension/") &&
		!isKind(obj, "Job", "batch/") &&
		!isKind(obj, "Service") {
		return summary
	}

	_, hasMatch := selector["matchLabels"]
	if !hasMatch {
		_, hasMatch = selector["matchExpressions"]
	}
	sel := metav1.LabelSelector{}
	if hasMatch {
		if err := convert.ToObj(selector, &sel); err != nil {
			return summary
		}
	} else {
		sel.MatchLabels = map[string]string{}
		for k, v := range selector {
			sel.MatchLabels[k] = convert.ToString(v)
		}
	}

	t := "creates"
	if obj["kind"] == "Service" {
		t = "selects"
	}

	summary.Relationships = append(summary.Relationships, Relationship{
		Kind:       "Pod",
		APIVersion: "v1",
		Type:       t,
		Selector:   &sel,
	})
	return summary
}

func checkPod(obj data.Object, condition []Condition, summary Summary) Summary {
	if !isKind(obj, "Pod") {
		return summary
	}
	if obj.String("kind") != "Pod" || obj.String("apiVersion") != "v1" {
		return summary
	}
	return checkPodTemplate(obj, condition, summary)
}

func checkPodTemplate(obj data.Object, condition []Condition, summary Summary) Summary {
	summary = checkPodConfigMaps(obj, condition, summary)
	summary = checkPodSecrets(obj, condition, summary)
	summary = checkPodServiceAccount(obj, condition, summary)
	summary = checkPodProjectedVolume(obj, condition, summary)
	summary = checkPodPullSecret(obj, condition, summary)
	return summary
}

func checkPodPullSecret(obj data.Object, _ []Condition, summary Summary) Summary {
	for _, pullSecret := range obj.Slice("imagePullSecrets") {
		if name := pullSecret.String("name"); name != "" {
			summary.Relationships = append(summary.Relationships, Relationship{
				Name:       name,
				Kind:       "Secret",
				APIVersion: "v1",
				Type:       "uses",
			})
		}
	}
	return summary
}

func checkPodProjectedVolume(obj data.Object, _ []Condition, summary Summary) Summary {
	for _, vol := range obj.Slice("spec", "volumes") {
		for _, source := range vol.Slice("projected", "sources") {
			if secretName := source.String("secret", "name"); secretName != "" {
				summary.Relationships = append(summary.Relationships, Relationship{
					Name:       secretName,
					Kind:       "Secret",
					APIVersion: "v1",
					Type:       "uses",
				})
			}
			if configMap := source.String("configMap", "name"); configMap != "" {
				summary.Relationships = append(summary.Relationships, Relationship{
					Name:       configMap,
					Kind:       "ConfigMap",
					APIVersion: "v1",
					Type:       "uses",
				})
			}
		}
	}
	return summary
}

func addEnvRef(summary Summary, names map[string]bool, obj data.Object, fieldPrefix, kind string) Summary {
	for _, container := range obj.Slice("spec", "containers") {
		for _, env := range container.Slice("envFrom") {
			name := env.String(fieldPrefix+"Ref", "name")
			if name == "" || names[name] {
				continue
			}
			names[name] = true
			summary.Relationships = append(summary.Relationships, Relationship{
				Name:       name,
				Kind:       kind,
				APIVersion: "v1",
				Type:       "uses",
			})
		}
		for _, env := range container.Slice("env") {
			name := env.String("valueFrom", fieldPrefix+"KeyRef", "name")
			if name == "" || names[name] {
				continue
			}
			names[name] = true
			summary.Relationships = append(summary.Relationships, Relationship{
				Name:       name,
				Kind:       kind,
				APIVersion: "v1",
				Type:       "uses",
			})
		}
	}

	return summary
}

func checkPodConfigMaps(obj data.Object, _ []Condition, summary Summary) Summary {
	names := map[string]bool{}
	for _, vol := range obj.Slice("spec", "volumes") {
		name := vol.String("configMap", "name")
		if name == "" || names[name] {
			continue
		}
		names[name] = true
		summary.Relationships = append(summary.Relationships, Relationship{
			Name:       name,
			Kind:       "ConfigMap",
			APIVersion: "v1",
			Type:       "uses",
		})
	}
	summary = addEnvRef(summary, names, obj, "configMap", "ConfigMap")
	return summary
}

func checkPodSecrets(obj data.Object, _ []Condition, summary Summary) Summary {
	names := map[string]bool{}
	for _, vol := range obj.Slice("spec", "volumes") {
		name := vol.String("secret", "secretName")
		if name == "" || names[name] {
			continue
		}
		names[name] = true
		summary.Relationships = append(summary.Relationships, Relationship{
			Name:       name,
			Kind:       "Secret",
			APIVersion: "v1",
			Type:       "uses",
		})
	}
	summary = addEnvRef(summary, names, obj, "secret", "Secret")
	return summary
}

func checkPodServiceAccount(obj data.Object, _ []Condition, summary Summary) Summary {
	saName := obj.String("spec", "serviceAccountName")
	summary.Relationships = append(summary.Relationships, Relationship{
		Name:       saName,
		Kind:       "ServiceAccount",
		APIVersion: "v1",
		Type:       "uses",
	})
	return summary

}
