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

// +name:"Rewrite"
// +weight:"200"
type _hugoRewrite interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Rewrite](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/)"
/*
Rewrite filters can be used to modify record contents. Logging operator currently supports the following rewrite functions:

- [group_unset](#groupunset)
- [rename](#rename)
- [set](#set)
- [substitute](#subst)
- [unset](#unset)

> Note: All rewrite functions support an optional `condition` which has the same syntax as the [match filter](../match/).

For details on how rewrite rules work in syslog-ng, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/).

## Group unset {#groupunset}

The `group_unset` function removes from the record a group of fields matching a pattern.

{{< highlight yaml >}}
  filters:
  - rewrite:
    - group_unset:
        pattern: "json.kubernetes.annotations.*"
{{</ highlight >}}

For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/rewrite-unset/).

## Rename

The `rename` function changes the name of an existing field name.

{{< highlight yaml >}}
  filters:
  - rewrite:
    - rename:
        oldName: "json.kubernetes.labels.app"
        newName: "json.kubernetes.labels.app.kubernetes.io/name"
{{</ highlight >}}

For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/rewrite-rename/).

## Set

The `set` function sets the value of a field.

{{< highlight yaml >}}
  filters:
  - rewrite:
    - set:
        field: "json.kubernetes.cluster"
        value: "prod-us"
{{</ highlight >}}

For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/rewrite-set/).

## Substitute (subst) {#subst}

The `subst` function replaces parts of a field with a replacement value based on a pattern.

{{< highlight yaml >}}
  filters:
  - rewrite:
    - subst:
        pattern: "\d\d\d\d-\d\d\d\d-\d\d\d\d-\d\d\d\d"
        replace: "[redacted bank card number]"
        field: "MESSAGE"
{{</ highlight >}}

The function also supports the `type` and `flags` fields for specifying pattern type and flags as described in the [match expression regexp function](../match/).

For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/rewrite-replace/).

## Unset

You can unset macros or fields of the message.

> Note: Unsetting a field completely deletes any previous value of the field.

{{< highlight yaml >}}
  filters:
  - rewrite:
    - unset:
        field: "json.kubernetes.cluster"
{{</ highlight >}}

For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/rewrite-unset/).
*/
type _docRewrite interface{} //nolint:deadcode,unused

// +name:"Syslog-NG Rewrite"
// +url:"https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/77"
// +version:"more info"
// +description:"Rewrite parts of the message"
// +status:"GA"
type _metaRewrite interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type RewriteConfig struct {
	GroupUnset *GroupUnsetConfig `json:"group_unset,omitempty" syslog-ng:"rewrite-drv,name=groupunset"`
	Rename     *RenameConfig     `json:"rename,omitempty" syslog-ng:"rewrite-drv,name=rename"`
	Set        *SetConfig        `json:"set,omitempty" syslog-ng:"rewrite-drv,name=set"`
	Substitute *SubstituteConfig `json:"subst,omitempty" syslog-ng:"rewrite-drv,name=subst"`
	Unset      *UnsetConfig      `json:"unset,omitempty" syslog-ng:"rewrite-drv,name=unset"`
}

// +kubebuilder:object:generate=true
// For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/rewrite-rename/).
type RenameConfig struct {
	OldFieldName string     `json:"oldName" syslog-ng:"pos=0"`
	NewFieldName string     `json:"newName" syslog-ng:"pos=1"`
	Condition    *MatchExpr `json:"condition,omitempty" syslog-ng:"name=condition,optional"`
}

// +kubebuilder:object:generate=true
// For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/rewrite-set/).
type SetConfig struct {
	FieldName string     `json:"field" syslog-ng:"name=value"` // NOTE: this is specified as `value(<field name>)` in the syslog-ng config
	Value     string     `json:"value" syslog-ng:"pos=0"`
	Condition *MatchExpr `json:"condition,omitempty" syslog-ng:"name=condition,optional"`
}

// +kubebuilder:object:generate=true
// For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/rewrite-set/).
type SubstituteConfig struct {
	Pattern     string     `json:"pattern" syslog-ng:"pos=0"`
	Replacement string     `json:"replace" syslog-ng:"pos=1"`
	FieldName   string     `json:"field" syslog-ng:"name=value"`
	Flags       []string   `json:"flags,omitempty" syslog-ng:"name=flags,optional"` // https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/81#TOPIC-1829224
	Type        string     `json:"type,omitempty" syslog-ng:"name=type,optional"`   // https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/81#TOPIC-1829223
	Condition   *MatchExpr `json:"condition,omitempty" syslog-ng:"name=condition,optional"`
}

// +kubebuilder:object:generate=true
// For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/rewrite-unset/).
type UnsetConfig struct {
	FieldName string     `json:"field" syslog-ng:"name=value"` // NOTE: this is specified as `value(<field name>)` in the syslog-ng config
	Condition *MatchExpr `json:"condition,omitempty" syslog-ng:"name=condition,optional"`
}

// +kubebuilder:object:generate=true
// For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/modifying-messages/rewrite-unset/).
type GroupUnsetConfig struct {
	Pattern   string     `json:"pattern" syslog-ng:"name=values"` // NOTE: this is specified as `value(<field name>)` in the syslog-ng config
	Condition *MatchExpr `json:"condition,omitempty" syslog-ng:"name=condition,optional"`
}
