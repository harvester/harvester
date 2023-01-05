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

// +name:"Match"
// +weight:"200"
type _hugoMatch interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Match"
// Match filters can be used to select the log records to process. These filters have the same options and syntax as [syslog-ng flow match expressions]({{< relref "/docs/logging-operator/configuration/plugins/syslog-ng-filters/match.md" >}}).
//
// {{< highlight yaml >}}
//
//	filters:
//	- match:
//	    or:
//	    - regexp:
//	        value: json.kubernetes.labels.app.kubernetes.io/name
//	        pattern: apache
//	        type: string
//	    - regexp:
//	        value: json.kubernetes.labels.app.kubernetes.io/name
//	        pattern: nginx
//	        type: string
//
// {{</ highlight >}}
type _docMatch interface{} //nolint:deadcode,unused

// +name:"Syslog-NG Match"
// +url:"https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/65#TOPIC-1829159"
// +version:"more info"
// +description:"Selectively keep records"
// +status:"GA"
type _metaMatch interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type MatchConfig MatchExpr

// +kubebuilder:object:generate=true
type MatchExpr struct {
	// +docLink:"And Directive,#And-Directive"
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	And []MatchExpr `json:"and,omitempty"`
	// +docLink:"Not Directive,#Exclude-Directive"
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Not *MatchExpr `json:"not,omitempty"`
	// +docLink:"Regexp Directive,#Regexp-Directive"
	Regexp *RegexpMatchExpr `json:"regexp,omitempty" syslog-ng:"name=match,optional"`
	// +docLink:"Or Directive,#Or-Directive"
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Or []MatchExpr `json:"or,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"[Regexp Directive](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/68#TOPIC-1829171) {#Regexp-Directive}"
// Specify filtering rule.
type RegexpMatchExpr struct {
	// Pattern expression to evaluate
	Pattern string `json:"pattern"`
	// Specify a template of the record fields to match against.
	Template string `json:"template,omitempty"`
	// Specify a field name of the record to match against the value of.
	Value string `json:"value,omitempty"`
	// Pattern flags
	Flags []string `json:"flags,omitempty"` // https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/81#TOPIC-1829224
	// Pattern type
	Type string `json:"type,omitempty"` // https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/81#TOPIC-1829223
}

// #### Example `Regexp` filter configurations
// ```yaml
// apiVersion: logging.banzaicloud.io/v1beta1
// kind: Flow
// metadata:
//
//	name: demo-flow
//
// spec:
//
//	filters:
//	  - match:
//	      regexp:
//	      - value: first
//	        pattern: ^5\d\d$
//	match: {}
//	localOutputRefs:
//	  - demo-output
//
// ```
//
// #### Syslog-NG Config Result
// ```
//
//	log {
//	   source(main_input);
//	   filter {
//	       match("^5\d\d$" value("first"));
//	   };
//	   destination(output_default_demo-output);
//	};
//
// ```
type _expRegexpMatch interface{} //nolint:deadcode,unused
