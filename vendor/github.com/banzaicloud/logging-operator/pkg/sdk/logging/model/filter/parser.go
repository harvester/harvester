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
	"fmt"

	"emperror.dev/errors"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

// +name:"Parser"
// +weight:"200"
type _hugoParser interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Parser Filter](https://docs.fluentd.org/filter/parser)"
// Parses a string field in event records and mutates its event record with the parsed result.
type _docParser interface{} //nolint:deadcode,unused

// +name:"Parser"
// +url:"https://docs.fluentd.org/filter/parser"
// +version:"more info"
// +description:"Parses a string field in event records and mutates its event record with the parsed result."
// +status:"GA"
type _metaParser interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type ParserConfig struct {
	// Specify field name in the record to parse. If you leave empty the Container Runtime default will be used.
	KeyName string `json:"key_name,omitempty"`
	// Keep original event time in parsed result.
	ReserveTime bool `json:"reserve_time,omitempty"`
	// Keep original key-value pair in parsed result.
	ReserveData bool `json:"reserve_data,omitempty"`
	// Remove key_name field when parsing is succeeded
	RemoveKeyNameField bool `json:"remove_key_name_field,omitempty"`
	// If true, invalid string is replaced with safe characters and re-parse it.
	ReplaceInvalidSequence bool `json:"replace_invalid_sequence,omitempty"`
	// Store parsed values with specified key name prefix.
	InjectKeyPrefix string `json:"inject_key_prefix,omitempty"`
	// Store parsed values as a hash value in a field.
	HashValueField string `json:"hash_value_field,omitempty"`
	// Emit invalid record to @ERROR label. Invalid cases are: key not exist, format is not matched, unexpected error
	EmitInvalidRecordToError *bool `json:"emit_invalid_record_to_error,omitempty"`
	// +docLink:"Parse Section,#parse-section"
	Parse ParseSection `json:"parse,omitempty"`
	// Deprecated, use `parse` instead
	Parsers []ParseSection `json:"parsers,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Parse Section"
type ParseSection struct {
	// Parse type: apache2, apache_error, nginx, syslog, csv, tsv, ltsv, json, multiline, none, logfmt, grok, multiline_grok
	Type string `json:"type,omitempty"`
	// Regexp expression to evaluate
	Expression string `json:"expression,omitempty"`
	// Specify time field for event time. If the event doesn't have this field, current time is used.
	TimeKey string `json:"time_key,omitempty"`
	// Names for fields on each line. (seperated by coma)
	Keys string `json:"keys,omitempty"`
	//  Specify null value pattern.
	NullValuePattern string `json:"null_value_pattern,omitempty"`
	// If true, empty string field is replaced with nil
	NullEmptyString bool `json:"null_empty_string,omitempty"`
	// If true, use Fluent::EventTime.now(current time) as a timestamp when time_key is specified.
	EstimateCurrentEvent bool `json:"estimate_current_event,omitempty"`
	// If true, keep time field in the record.
	KeepTimeKey bool `json:"keep_time_key,omitempty"`
	// Types casting the fields to proper types example: field1:type, field2:type
	Types string `json:"types,omitempty"`
	// Process value using specified format. This is available only when time_type is string
	TimeFormat string `json:"time_format,omitempty"`
	// Parse/format value according to this type available values: float, unixtime, string (default: string)
	TimeType string `json:"time_type,omitempty"`
	// Ff true, use local time. Otherwise, UTC is used. This is exclusive with utc. (default: true)
	LocalTime bool `json:"local_time,omitempty"`
	// If true, use UTC. Otherwise, local time is used. This is exclusive with localtime (default: false)
	UTC bool `json:"utc,omitempty"`
	// Use specified timezone. one can parse/format the time value in the specified timezone. (default: nil)
	Timezone string `json:"timezone,omitempty"`
	// Only available when using type: multi_format
	Format string `json:"format,omitempty"`
	// Only available when using type: multi_format
	FormatFirstline string `json:"format_firstline,omitempty"`
	// Only available when using type: ltsv (default: "\t")
	Delimiter string `json:"delimiter,omitempty"`
	// Only available when using type: ltsv
	DelimiterPattern string `json:"delimiter_pattern,omitempty"`
	// Only available when using type: ltsv (default: ":")
	LabelDelimiter string `json:"label_delimiter,omitempty"`
	// The multiline parser plugin parses multiline logs.
	Multiline []string `json:"multiline,omitempty"`
	// Only available when using type: multi_format
	// +docLink:"Parse Section,#parse-section"
	Patterns []SingleParseSection `json:"patterns,omitempty"`
	// Only available when using type: grok, multiline_grok.
	// The pattern of grok. You cannot specify multiple grok pattern with this.
	GrokPattern string `json:"grok_pattern,omitempty"`
	// Only available when using type: grok, multiline_grok.
	// File that includes custom grok patterns.
	CustomPatternPath *secret.Secret `json:"custom_pattern_path,omitempty"`
	// Only available when using type: grok, multiline_grok.
	// The key has grok failure reason.
	GrokFailureKey string `json:"grok_failure_key,omitempty"`
	// Only available when using type: grok, multiline_grok.
	// The key name to store grok section's name.
	GrokNameKey string `json:"grok_name_key,omitempty"`
	// Only available when using type: multiline_grok
	// The regexp to match beginning of multiline.
	MultilineStartRegexp string `json:"multiline_start_regexp,omitempty"`
	// Only available when using type: grok, multiline_grok.
	// +docLink:"Grok Section,#grok-section"
	// Specify grok pattern series set.
	GrokPatterns []GrokSection `json:"grok_patterns,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Parse Section (single)"
type SingleParseSection struct {
	// Parse type: apache2, apache_error, nginx, syslog, csv, tsv, ltsv, json, multiline, none, logfmt, grok, multiline_grok
	Type string `json:"type,omitempty"`
	// Regexp expression to evaluate
	Expression string `json:"expression,omitempty"`
	// Specify time field for event time. If the event doesn't have this field, current time is used.
	TimeKey string `json:"time_key,omitempty"`
	//  Specify null value pattern.
	NullValuePattern string `json:"null_value_pattern,omitempty"`
	// If true, empty string field is replaced with nil
	NullEmptyString bool `json:"null_empty_string,omitempty"`
	// If true, use Fluent::EventTime.now(current time) as a timestamp when time_key is specified.
	EstimateCurrentEvent bool `json:"estimate_current_event,omitempty"`
	// If true, keep time field in the record.
	KeepTimeKey bool `json:"keep_time_key,omitempty"`
	// Types casting the fields to proper types example: field1:type, field2:type
	Types string `json:"types,omitempty"`
	// Process value using specified format. This is available only when time_type is string
	TimeFormat string `json:"time_format,omitempty"`
	// Parse/format value according to this type available values: float, unixtime, string (default: string)
	TimeType string `json:"time_type,omitempty"`
	// Ff true, use local time. Otherwise, UTC is used. This is exclusive with utc. (default: true)
	LocalTime bool `json:"local_time,omitempty"`
	// If true, use UTC. Otherwise, local time is used. This is exclusive with localtime (default: false)
	UTC bool `json:"utc,omitempty"`
	// Use specified timezone. one can parse/format the time value in the specified timezone. (default: nil)
	Timezone string `json:"timezone,omitempty"`
	// Only available when using type: multi_format
	Format string `json:"format,omitempty"`
	// Only available when using format: grok, multiline_grok.
	// The pattern of grok. You cannot specify multiple grok pattern with this.
	GrokPattern string `json:"grok_pattern,omitempty"`
	// Only available when using format: grok, multiline_grok.
	// File that includes custom grok patterns.
	CustomPatternPath *secret.Secret `json:"custom_pattern_path,omitempty"`
	// Only available when using format: grok, multiline_grok.
	// The key has grok failure reason.
	GrokFailureKey string `json:"grok_failure_key,omitempty"`
	// Only available when using format: grok, multiline_grok.
	// The key name to store grok section's name.
	GrokNameKey string `json:"grok_name_key,omitempty"`
	// Only available when using format: multiline_grok
	// The regexp to match beginning of multiline.
	MultilineStartRegexp string `json:"multiline_start_regexp,omitempty"`
	// Only available when using format: grok, multiline_grok.
	// +docLink:"Grok Section,#grok-section"
	// Specify grok pattern series set.
	GrokPatterns []GrokSection `json:"grok_patterns,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Grok Section"
type GrokSection struct {
	// The name of grok section.
	Name string `json:"name,omitempty"`
	// The pattern of grok.
	Pattern string `json:"pattern" plugin:"required"`
	// If true, keep time field in the record.
	KeepTimeKey bool `json:"keep_time_key,omitempty"`
	// Specify time field for event time. If the event doesn't have this field, current time is used.
	TimeKey string `json:"time_key,omitempty" plugin:"default:time"`
	// Process value using specified format. This is available only when time_type is string.
	TimeFormat string `json:"time_format,omitempty"`
	// Use specified timezone. one can parse/format the time value in the specified timezone.
	Timezone string `json:"timezone,omitempty"`
}

// ## Example `Parser` filter configurations
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
//	  - parser:
//	      remove_key_name_field: true
//	      reserve_data: true
//	      parse:
//	        type: multi_format
//	        patterns:
//	        - format: nginx
//	        - format: regexp
//	          expression: /foo/
//	        - format: none
//	selectors: {}
//	localOutputRefs:
//	  - demo-output
//
// ```
//
// #### Fluentd Config Result
// ```yaml
// <filter **>
//
//	@type parser
//	@id test_parser
//	key_name message
//	remove_key_name_field true
//	reserve_data true
//	<parse>
//	  @type multi_format
//	  <pattern>
//	    format nginx
//	  </pattern>
//	  <pattern>
//	    expression /foo/
//	    format regexp
//	  </pattern>
//	  <pattern>
//	    format none
//	  </pattern>
//	</parse>
//
// </filter>
// ```
type _expParser interface{} //nolint:deadcode,unused

func (p *SingleParseSection) ToPatternDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	parseSection := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      p.Type,
			Directive: "pattern",
		},
	}
	section := p.DeepCopy()
	section.Type = ""
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(section); err != nil {
		return nil, err
	} else {
		parseSection.Params = params
	}
	for _, grokRule := range section.GrokPatterns {
		if meta, err := grokRule.ToGrokDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			parseSection.SubDirectives = append(parseSection.SubDirectives, meta)
		}
	}
	return parseSection, nil
}

func (p *GrokSection) ToGrokDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	parseMeta := types.PluginMeta{
		Directive: "grok",
	}
	section := p.DeepCopy()
	return types.NewFlatDirective(parseMeta, section, secretLoader)
}

func (p *ParseSection) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	parseSection := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      p.Type,
			Directive: "parse",
		},
	}
	section := p.DeepCopy()
	section.Type = ""
	section.Multiline = nil
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(section); err != nil {
		return nil, err
	} else {
		parseSection.Params = params
	}
	if len(p.Multiline) > 0 && p.Type == "multiline" {
		parseSection.Params["format_firstline"] = p.FormatFirstline
		for i, v := range p.Multiline {
			key := fmt.Sprintf("format%d", i+1)
			parseSection.Params[key] = v
		}
	}
	for _, parseRule := range section.Patterns {
		if parseRule.Format != "" && p.Type != "multi_format" {
			return nil, errors.Errorf("format parameter only works with multi_format type")
		}
		if parseRule.Format == "none" && p.Type != "multi_format" {
			return nil, errors.Errorf("none format type parameter only works with multi_format type")
		}
		if meta, err := parseRule.ToPatternDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			parseSection.SubDirectives = append(parseSection.SubDirectives, meta)
		}
	}
	for _, grokRule := range section.GrokPatterns {
		if meta, err := grokRule.ToGrokDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			parseSection.SubDirectives = append(parseSection.SubDirectives, meta)
		}
	}
	return parseSection, nil
}

func (p *ParserConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "parser"
	parser := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "filter",
			Tag:       "**",
			Id:        id,
		},
	}
	parserConfig := p.DeepCopy()

	if parserConfig.KeyName == "" {
		parserConfig.KeyName = types.GetLogKey()
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(parserConfig); err != nil {
		return nil, err
	} else {
		parser.Params = params
	}

	if len(parserConfig.Parsers) > 0 {
		return nil, errors.Errorf("`parsers` field is deprecated, use `parse`")
	}

	if meta, err := parserConfig.Parse.ToDirective(secretLoader, ""); err != nil {
		return nil, err
	} else {
		parser.SubDirectives = append(parser.SubDirectives, meta)
	}
	return parser, nil
}
