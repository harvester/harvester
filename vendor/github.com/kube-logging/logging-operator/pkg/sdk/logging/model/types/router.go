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
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/maps/mapstrstr"
)

// OutputPlugin plugin: https://github.com/kube-logging/fluent-plugin-label-router
type Router struct {
	PluginMeta
	Routes []Directive `json:"routes"`
	Params Params
}

func (r *Router) GetPluginMeta() *PluginMeta {
	return &r.PluginMeta
}

func (r *Router) GetParams() Params {
	return r.Params
}

func (r *Router) GetSections() []Directive {
	return r.Routes
}

type FlowMatch struct {
	// Optional set of kubernetes labels
	Labels map[string]string `json:"labels,omitempty"`
	// Optional namespace
	Namespaces []string `json:"namespaces,omitempty"`
	// ContainerNames
	ContainerNames []string `json:"container_names,omitempty"`
	// Hosts
	Hosts []string `json:"hosts,omitempty"`
	// Negate
	Negate bool `json:"negate,omitempty"`
}

func (f FlowMatch) GetPluginMeta() *PluginMeta {
	return &PluginMeta{
		Directive: "match",
	}
}
func (f FlowMatch) GetParams() Params {
	params := Params{
		"negate": strconv.FormatBool(f.Negate),
	}
	if len(f.Namespaces) > 0 {
		params["namespaces"] = strings.Join(f.Namespaces, ",")
	}
	if len(f.ContainerNames) > 0 {
		params["container_names"] = strings.Join(f.ContainerNames, ",")
	}
	if len(f.Hosts) > 0 {
		params["hosts"] = strings.Join(f.Hosts, ",")
	}
	if len(f.Labels) > 0 {
		var sb []string
		keys := mapstrstr.Keys(f.Labels)
		sort.Strings(keys)
		for _, key := range keys {
			sb = append(sb, key+":"+f.Labels[key])
		}
		params["labels"] = strings.Join(sb, ",")
	}
	return params
}

func (f FlowMatch) GetSections() []Directive {
	return nil
}

type FlowRoute struct {
	PluginMeta
	Params  Params
	Matches []Directive
}

func (f *FlowRoute) GetPluginMeta() *PluginMeta {
	return &f.PluginMeta
}

func (f *FlowRoute) GetParams() Params {
	return f.Params
}

func (f *FlowRoute) GetSections() []Directive {
	return f.Matches
}

func (r *Router) AddRoute(flow *Flow) *Router {
	route := &FlowRoute{
		PluginMeta: PluginMeta{
			Directive: "route",
			Label:     flow.FlowLabel,
		},
		Params: Params{},
	}
	if flow.FlowID != "" {
		metricsLabels, _ := json.Marshal(map[string]string{"id": flow.FlowID})
		route.Params["metrics_labels"] = string(metricsLabels)
	}
	for _, f := range flow.Matches {
		route.Matches = append(route.Matches, f)
	}
	r.Routes = append(r.Routes, route)
	return r
}

func NewRouter(id string, params Params) *Router {
	return &Router{
		PluginMeta: PluginMeta{
			Type:      "label_router",
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
		Params: params,
	}
}
