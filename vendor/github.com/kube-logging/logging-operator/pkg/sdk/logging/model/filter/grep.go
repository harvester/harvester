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

package filter

import (
	"github.com/cisco-open/operator-tools/pkg/secret"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Grep"
// +weight:"200"
type _hugoGrep interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Grep Filter](https://docs.fluentd.org/filter/grep)"
// The grep filter plugin "greps" events by the values of specified fields.
type _docGrep interface{} //nolint:deadcode,unused

// +name:"Grep"
// +url:"https://docs.fluentd.org/filter/grep"
// +version:"more info"
// +description:"Grep events by the values"
// +status:"GA"
type _metaGrep interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type GrepConfig struct {
	// +docLink:"Regexp Directive,#Regexp-Directive"
	Regexp []RegexpSection `json:"regexp,omitempty"`
	// +docLink:"Exclude Directive,#Exclude-Directive"
	Exclude []ExcludeSection `json:"exclude,omitempty"`
	// +docLink:"Or Directive,#Or-Directive"
	Or []OrSection `json:"or,omitempty"`
	// +docLink:"And Directive,#And-Directive"
	And []AndSection `json:"and,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Regexp Directive"
// Specify filtering rule (as described in the [Fluentd documentation](https://docs.fluentd.org/filter/grep#less-than-regexp-greater-than-directive)). This directive contains two parameters.
type RegexpSection struct {
	// Specify field name in the record to parse.
	Key string `json:"key"`
	// Pattern expression to evaluate
	Pattern string `json:"pattern"`
}

//
/*
## Example `Regexp` filter configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Flow
metadata:
  name: demo-flow
spec:
  filters:
    - grep:
        regexp:
        - key: first
          pattern: /^5\d\d$/
  selectors: {}
  localOutputRefs:
    - demo-output
{{</ highlight >}}

Fluentd config result:

{{< highlight xml >}}
  <filter **>
    @type grep
    @id demo-flow_1_grep
    <regexp>
      key first
      pattern /^5\d\d$/
    </regexp>
  </filter>
{{</ highlight >}}
*/
type _expRegexp interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Exclude Directive"
// Specify filtering rule to reject events (as described in the [Fluentd documentation](https://docs.fluentd.org/filter/grep#less-than-exclude-greater-than-directive)). This directive contains two parameters.
type ExcludeSection struct {
	// Specify field name in the record to parse.
	Key string `json:"key"`
	// Pattern expression to evaluate
	Pattern string `json:"pattern"`
}

//
/*
## Example `Exclude` filter configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Flow
metadata:
  name: demo-flow
spec:
  filters:
    - grep:
        exclude:
        - key: first
          pattern: /^5\d\d$/
  selectors: {}
  localOutputRefs:
    - demo-output
{{</ highlight >}}

Fluentd config result:

{{< highlight xml >}}
  <filter **>
    @type grep
    @id demo-flow_0_grep
    <exclude>
      key first
      pattern /^5\d\d$/
    </exclude>
  </filter>
{{</ highlight >}}
*/
type _expExclude interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Or Directive"
// Specify filtering rule (as described in the [Fluentd documentation](https://docs.fluentd.org/filter/grep#less-than-or-greater-than-directive). This directive contains either `regexp` or `exclude` directive.
type OrSection struct {
	// +docLink:"Regexp Directive,#Regexp-Directive"
	Regexp []RegexpSection `json:"regexp,omitempty"`
	// +docLink:"Exclude Directive,#Exclude-Directive"
	Exclude []ExcludeSection `json:"exclude,omitempty"`
}

//
/*
## Example `Or` filter configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Flow
metadata:
  name: demo-flow
spec:
  filters:
    - grep:
        or:
          - exclude:
            - key: first
              pattern: /^5\d\d$/
            - key: second
              pattern: /\.css$/

  selectors: {}
  localOutputRefs:
    - demo-output
{{</ highlight >}}

Fluentd config result:

{{< highlight xml >}}
<or>
	<exclude>
	key first
	pattern /^5\d\d$/
	</exclude>
	<exclude>
	key second
	pattern /\.css$/
	</exclude>
</or>
{{</ highlight >}}
*/
type _expOR interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"And Directive"
// Specify filtering rule (as described in the [Fluentd documentation](https://docs.fluentd.org/filter/grep#less-than-and-greater-than-directive). This directive contains either `regexp` or `exclude` directive.
type AndSection struct {
	// +docLink:"Regexp Directive,#Regexp-Directive"
	Regexp []RegexpSection `json:"regexp,omitempty"`
	// +docLink:"Exclude Directive,#Exclude-Directive"
	Exclude []ExcludeSection `json:"exclude,omitempty"`
}

//
/*
## Example `And` filter configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Flow
metadata:
  name: demo-flow
spec:
  filters:
    - grep:
        and:
          - regexp:
            - key: first
              pattern: /^5\d\d$/
            - key: second
              pattern: /\.css$/

  selectors: {}
  localOutputRefs:
    - demo-output
{{</ highlight >}}

Fluentd config result:

{{< highlight xml >}}
	<and>
	  <regexp>
	    key first
	    pattern /^5\d\d$/
	  </regexp>
	  <regexp>
	    key second
	    pattern /\.css$/
	  </regexp>
	</and>
{{</ highlight >}}
*/
type _expAND interface{} //nolint:deadcode,unused

func (r *RegexpSection) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	meta := types.PluginMeta{
		Directive: "regexp",
	}
	return types.NewFlatDirective(meta, r, secretLoader)
}

func (r *ExcludeSection) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	meta := types.PluginMeta{
		Directive: "exclude",
	}
	return types.NewFlatDirective(meta, r, secretLoader)
}

func (r *OrSection) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	or := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Directive: "or",
		},
	}

	for _, regexp := range r.Regexp {
		if meta, err := regexp.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			or.SubDirectives = append(or.SubDirectives, meta)
		}
	}
	for _, exclude := range r.Exclude {
		if meta, err := exclude.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			or.SubDirectives = append(or.SubDirectives, meta)
		}
	}

	return or, nil
}

func (r *AndSection) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	and := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Directive: "and",
		},
	}

	for _, regexp := range r.Regexp {
		if meta, err := regexp.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			and.SubDirectives = append(and.SubDirectives, meta)
		}
	}
	for _, exclude := range r.Exclude {
		if meta, err := exclude.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			and.SubDirectives = append(and.SubDirectives, meta)
		}
	}

	return and, nil
}

func (g *GrepConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "grep"
	grep := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "filter",
			Tag:       "**",
			Id:        id,
		},
	}
	for _, regexp := range g.Regexp {
		if meta, err := regexp.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			grep.SubDirectives = append(grep.SubDirectives, meta)
		}
	}
	for _, exclude := range g.Exclude {
		if meta, err := exclude.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			grep.SubDirectives = append(grep.SubDirectives, meta)
		}
	}
	for _, or := range g.Or {
		if meta, err := or.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			grep.SubDirectives = append(grep.SubDirectives, meta)
		}
	}
	for _, and := range g.And {
		if meta, err := and.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			grep.SubDirectives = append(grep.SubDirectives, meta)
		}
	}

	return grep, nil
}
