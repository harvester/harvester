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
	"fmt"

	"emperror.dev/errors"
	"github.com/cisco-open/operator-tools/pkg/secret"
)

var ContainerRuntime = "containerd"

func GetLogKey() string {
	switch ContainerRuntime {
	case "docker":
		return "log"
	case "containerd":
		return "message"
	default:
		return "message"
	}
}

type Directive interface {
	GetPluginMeta() *PluginMeta
	GetParams() Params
	GetSections() []Directive
}

type Filter interface {
	Directive
}

type Input interface {
	Directive
}

type Output interface {
	Directive
}

func Value(value string) *PluginParam {
	return &PluginParam{
		Value: value,
	}
}

type OutputPlugin = GenericDirective

type PluginMeta struct {
	Type      string `json:"type,omitempty"`
	Id        string `json:"id,omitempty"`
	LogLevel  string `json:"log_level,omitempty"`
	Directive string `json:"directive"`
	Label     string `json:"label,omitempty"`
	Tag       string `json:"tag,omitempty"`
}

type GenericDirective struct {
	PluginMeta
	Params        Params      `json:"params,omitempty"`
	SubDirectives []Directive `json:"sections,omitempty"`
}

func (d *GenericDirective) GetPluginMeta() *PluginMeta {
	return &d.PluginMeta
}

func (d *GenericDirective) GetParams() Params {
	return d.Params
}

func (d *GenericDirective) GetSections() []Directive {
	return d.SubDirectives
}

type PluginParam struct {
	Description string
	Default     string
	Value       string
	Required    bool
}

type PluginParams map[string]*PluginParam

// Equals check for exact matching of 2 PluginParams by Values
func (p PluginParams) Equals(target PluginParams) error {
	keySet := map[string]bool{}
	// Iterate through the p
	for key, body := range p {
		// Check keys at the matching
		matchBody, ok := target[key]
		if !ok {
			// There is no such key in the target PluginParams
			return fmt.Errorf("missing key %q from target", key)
		}
		if body != nil {
			if matchBody != nil {
				// The values does not target
				if body.Value != matchBody.Value {
					return fmt.Errorf("the values at %q mistmatch %q != %q", key, body.Value, matchBody.Value)
				}
			} else {
				// There is no body at the target PluginParams for the matching key
				return fmt.Errorf("missing body at %q from target", key)
			}
		}
		// Add key to the keySet
		keySet[key] = true
	}
	for key := range target {
		if _, ok := keySet[key]; !ok {
			// We have more keys int the matching PluginParams
			return fmt.Errorf("unexpected key %q at target", key)
		}
	}
	return nil
}

type Params = map[string]string

func NewFlatDirective(meta PluginMeta, config interface{}, secretLoader secret.SecretLoader) (Directive, error) {
	directive := &GenericDirective{
		PluginMeta: meta,
	}
	if params, err := NewStructToStringMapper(secretLoader).StringsMap(config); err != nil {
		return nil, errors.WrapIf(err, "failed to convert struct to map[string]string params")
	} else {
		directive.Params = params
	}
	return directive, nil
}

func NewCopyDirective(directives []Output) Directive {
	directive := &GenericDirective{
		PluginMeta: PluginMeta{
			Directive: "match",
			Type:      "copy",
			Tag:       "**",
		},
	}
	for _, d := range directives {
		newCopySection := &GenericDirective{
			PluginMeta: PluginMeta{
				Type:      d.GetPluginMeta().Type,
				Id:        d.GetPluginMeta().Id,
				LogLevel:  d.GetPluginMeta().LogLevel,
				Directive: "store",
			},
			Params:        d.GetParams(),
			SubDirectives: d.GetSections(),
		}
		directive.SubDirectives = append(directive.SubDirectives, newCopySection)
	}
	return directive
}
