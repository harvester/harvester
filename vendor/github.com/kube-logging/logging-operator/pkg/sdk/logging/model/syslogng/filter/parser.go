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

// +name:"Parser"
// +weight:"200"
type _hugoParser interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Parser](https://axoflow.com/docs/axosyslog-core/chapter-parsers/)"
// +kubebuilder:object:generate=true
/*
Parser filters can be used to extract key-value pairs from message data. Logging operator currently supports the following parsers:

- [metrics-probe](#metricsprobe)
- [regexp](#regexp)
- [syslog-parser](#syslog)

## Regexp parser {#regexp}

The regexp parser can use regular expressions to parse fields from a message.

{{< highlight yaml >}}
  filters:
  - parser:
      regexp:
        patterns:
        - ".*test_field -> (?<test_field>.*)$"
        prefix: .regexp.
{{</ highlight >}}

For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-regexp/).

## Syslog parser {#syslog}

The syslog parser can parse syslog messages. For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-syslog/).

{{< highlight yaml >}}
  filters:
  - parser:
      syslog-parser: {}
{{</ highlight >}}
*/
type _docParser interface{} //nolint:deadcode,unused

// +name:"Syslog-NG Parser"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-parsers/"
// +version:"more info"
// +description:"Parse data from records"
// +status:"GA"
type _metaParser interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Parser](https://axoflow.com/docs/axosyslog-core/chapter-parsers/)"
type ParserConfig struct {
	// The regular expression patterns that you want to find a match. `regexp-parser()` supports multiple patterns, and stops the processing at the first successful match. For details, see the [regexp-parser() documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-regexp/parser-regexp-options/#patterns).
	Regexp *RegexpParser `json:"regexp,omitempty" syslog-ng:"parser-drv,name=regexp-parser"`
	// Parse message as a [syslog message](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-syslog/).
	SyslogParser *SyslogParser `json:"syslog-parser,omitempty," syslog-ng:"parser-drv,name=syslog-parser"`
	// Counts the messages that pass through the flow, and creates labeled stats counters based on the fields of the passing messages. For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-parsers/metrics-probe/).
	MetricsProbe *MetricsProbe `json:"metrics-probe,omitempty," syslog-ng:"parser-drv,name=metrics-probe"`
}

// +kubebuilder:object:generate=true
// +docName:"[Regexp parser](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-regexp/)"
type RegexpParser struct {
	// The regular expression patterns that you want to find a match. `regexp-parser()` supports multiple patterns, and stops the processing at the first successful match. For details, see the [regexp-parser() documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-regexp/parser-regexp-options/#patterns).
	Patterns []string `json:"patterns"`
	// Insert a prefix before the name part of the parsed name-value pairs to help further processing. For details, see the [regexp-parser() documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-regexp/parser-regexp-options/#prefix).
	Prefix string `json:"prefix,omitempty"`
	// Specify a template of the record fields to match against. For details, see the [regexp-parser() documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-regexp/parser-regexp-options/#template).
	Template string `json:"template,omitempty"`
	// Flags to influence the behavior of the [regexp-parser()](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-regexp/parser-regexp-options/). For details, see the [regexp-parser() documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-regexp/parser-regexp-options/#flags).
	Flags []string `json:"flags,omitempty"` // https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/81#TOPIC-1829224
}

// +kubebuilder:object:generate=true
// +docName:"[Syslog Parser](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-syslog/)
// Parse message as a [syslog message](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-syslog/).
type SyslogParser struct {
	// Flags to influence the behavior of the [syslog-parser()](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-syslog/parser-syslog-options/). For details, see the [syslog-parser() documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-parsers/parser-syslog/parser-syslog-options/#flags).
	Flags []string `json:"flags,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"[Metrics Probe](https://axoflow.com/docs/axosyslog-core/chapter-parsers/metrics-probe/)
/*
Counts the messages that pass through the flow, and creates labeled stats counters based on the fields of the passing messages. For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-parsers/metrics-probe/).

{{< highlight yaml>}}SyslogNGFlow
apiVersion: logging.banzaicloud.io/v1beta1
kind: SyslogNGFlow
metadata:
  name: flow-mertrics-probe
  namespace: default
spec:
  filters:
    - parser:
        metrics-probe:
          key: "flow_events"
          labels:
            namespace: "${json.kubernetes.namespace_name}"{{< /highlight >}}
*/
type MetricsProbe struct {
	__meta struct{} `json:"-" syslog-ng:"name=metrics-probe"`
	// The name of the counter to create. Note that the value of this option is always prefixed with `syslogng_`, so for example `key("my-custom-key")` becomes `syslogng_my-custom-key`.
	Key string `json:"key,omitempty"`
	// The labels used to create separate counters, based on the fields of the messages processed by `metrics-probe()`. The keys of the map are the name of the label, and the values are syslog-ng templates.
	Labels ArrowMap `json:"labels,omitempty"`
	// Sets the stats level of the generated metrics (default 0).
	Level int `json:"level,omitempty"`
}

type ArrowMap map[string]string
type RawArrowMap map[string]string
