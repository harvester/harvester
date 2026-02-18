package summary

import (
	"strings"

	"github.com/rancher/wrangler/v3/pkg/data"
)

func checkCattleReady(obj data.Object, condition []Condition, summary Summary) Summary {
	if strings.Contains(obj.String("apiVersion"), "cattle.io/") {
		for _, condition := range condition {
			if condition.Type() == "Ready" && condition.Status() == "False" && condition.Message() != "" {
				summary.Message = append(summary.Message, condition.Message())
				summary.Error = true
				return summary
			}
		}
	}

	return summary
}

func checkCattleTypes(obj data.Object, condition []Condition, summary Summary) Summary {
	return checkRelease(obj, condition, summary)
}

func checkRelease(obj data.Object, _ []Condition, summary Summary) Summary {
	if !isKind(obj, "App", "catalog.cattle.io") {
		return summary
	}
	if obj.String("status", "summary", "state") != "deployed" {
		return summary
	}
	for _, resources := range obj.Slice("spec", "resources") {
		summary.Relationships = append(summary.Relationships, Relationship{
			Name:       resources.String("name"),
			Namespace:  resources.String("namespace"),
			Kind:       resources.String("kind"),
			APIVersion: resources.String("apiVersion"),
			Type:       "helmresource",
		})
	}
	return summary
}
