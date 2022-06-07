package summary

import (
	"strings"

	"github.com/rancher/wrangler/pkg/data"
	unstructured2 "github.com/rancher/wrangler/pkg/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type Summary struct {
	State         string                 `json:"state,omitempty"`
	Error         bool                   `json:"error,omitempty"`
	Transitioning bool                   `json:"transitioning,omitempty"`
	Message       []string               `json:"message,omitempty"`
	Attributes    map[string]interface{} `json:"-"`
	Relationships []Relationship         `json:"-"`
}

type Relationship struct {
	Name         string
	Namespace    string
	ControlledBy bool
	Kind         string
	APIVersion   string
	Inbound      bool
	Type         string
	Selector     *metav1.LabelSelector
}

func (s Summary) String() string {
	if !s.Transitioning && !s.Error {
		return s.State
	}
	var msg string
	if s.Transitioning {
		msg = "[progressing"
	}
	if s.Error {
		if len(msg) > 0 {
			msg += ",error]"
		} else {
			msg = "error]"
		}
	} else {
		msg += "]"
	}
	if len(s.Message) > 0 {
		msg = msg + " " + strings.Join(s.Message, ", ")
	}
	return msg
}

func (s Summary) IsReady() bool {
	return !s.Error && !s.Transitioning
}

func (s *Summary) DeepCopy() *Summary {
	v := *s
	return &v
}

func (s *Summary) DeepCopyInto(v *Summary) {
	*v = *s
}

func dedupMessage(messages []string) []string {
	seen := map[string]bool{}
	var result []string

	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message == "" {
			continue
		}
		if seen[message] {
			continue
		}
		seen[message] = true
		result = append(result, message)
	}

	return result
}

func Summarize(runtimeObj runtime.Object) Summary {
	var (
		obj     data.Object
		err     error
		summary Summary
	)

	if s, ok := runtimeObj.(*SummarizedObject); ok {
		return s.Summary
	}

	unstr, ok := runtimeObj.(*unstructured.Unstructured)
	if !ok {
		unstr, err = unstructured2.ToUnstructured(runtimeObj)
		if err != nil {
			return summary
		}
	}

	if unstr != nil {
		obj = unstr.Object
	}

	conditions := getConditions(obj)

	for _, summarizer := range Summarizers {
		summary = summarizer(obj, conditions, summary)
	}

	if summary.State == "" {
		summary.State = "active"
	}

	summary.State = strings.ToLower(summary.State)
	summary.Message = dedupMessage(summary.Message)
	return summary
}
