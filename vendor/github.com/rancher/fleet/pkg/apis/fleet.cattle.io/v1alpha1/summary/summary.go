package summary

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
