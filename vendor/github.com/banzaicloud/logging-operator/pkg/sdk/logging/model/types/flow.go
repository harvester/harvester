// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	"crypto/md5"
	"fmt"
	"io"
	"sort"

	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/maps/mapstrstr"
)

type FluentConfig interface {
	GetDirectives() []Directive
}

type System struct {
	Input         Input    `json:"input"`
	GlobalFilters []Filter `json:"globalFilters"`
	Router        *Router  `json:"router"`
	Flows         []*Flow  `json:"flows"`
}

func (s *System) GetDirectives() []Directive {
	// First directive is input
	directives := []Directive{
		s.Input,
	}
	// Add GlobalFilters between input and router
	for _, filter := range s.GlobalFilters {
		directives = append(directives, filter)
	}
	// Add router directive
	directives = append(directives, s.Router)
	// Add Flows after router
	for _, flow := range s.Flows {
		directives = append(directives, flow)
	}
	return directives
}

type Flow struct {
	PluginMeta

	//Flow id for metrics
	FlowID string
	// Chain of Filters that will process the event. Can be zero or more.
	Filters []Filter `json:"filters,omitempty"`
	// List of Outputs that will emit the event, at least one output is required.
	Outputs []Output `json:"outputs"`

	// Matches for select or exclude
	Matches []FlowMatch `json:"matches,omitempty"`

	// Fluentd label
	FlowLabel string `json:"-"`
}

func (f *Flow) GetPluginMeta() *PluginMeta {
	return &f.PluginMeta
}

func (f *Flow) GetParams() Params {
	return nil
}

func (f *Flow) GetSections() []Directive {
	var sections []Directive
	for _, filter := range f.Filters {
		sections = append(sections, filter)
	}
	if len(f.Outputs) > 1 {
		// We have to convert to General directive
		sections = append(sections, NewCopyDirective(f.Outputs))
	} else {
		for _, output := range f.Outputs {
			sections = append(sections, output)
		}
	}

	return sections
}

func (f *Flow) WithFilters(filter ...Filter) *Flow {
	f.Filters = append(f.Filters, filter...)
	return f
}

func (f *Flow) WithOutputs(output ...Output) *Flow {
	f.Outputs = append(f.Outputs, output...)
	return f
}

func NewFlow(matches []FlowMatch, id, name, namespace string) (*Flow, error) {
	flowLabel, err := calculateFlowLabel(matches, name, namespace)
	if err != nil {
		return nil, err
	}
	return &Flow{
		PluginMeta: PluginMeta{
			Directive: "label",
			Tag:       flowLabel,
		},
		FlowID:    id,
		FlowLabel: flowLabel,
		Matches:   matches,
	}, nil
}

func calculateFlowLabel(matches []FlowMatch, name, namespace string) (string, error) {
	b := md5.New()
	_, err := io.WriteString(b, name)
	if err != nil {
		return "", err
	}
	_, err = io.WriteString(b, namespace)
	if err != nil {
		return "", err
	}
	for _, match := range matches {
		sort.Strings(match.Namespaces)
		for _, n := range match.Namespaces {
			if _, err := io.WriteString(b, n); err != nil {
				return "", err
			}
		}
		// Make sure the generated label is consistent
		keys := mapstrstr.Keys(match.Labels)
		sort.Strings(keys)
		for _, k := range keys {
			if _, err := io.WriteString(b, k); err != nil {
				return "", err
			}
			if _, err := io.WriteString(b, match.Labels[k]); err != nil {
				return "", err
			}
		}
	}

	return fmt.Sprintf("@%x", b.Sum(nil)), nil
}
