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
	"fmt"
	"reflect"
	"strings"

	"emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

type Converter func(interface{}) (string, error)

type StructToStringMapper struct {
	TagName         string
	PluginTagName   string
	ConversionHooks map[string]Converter
	SecretLoader    secret.SecretLoader
}

func NewStructToStringMapper(secretLoader secret.SecretLoader) *StructToStringMapper {
	return &StructToStringMapper{
		TagName:         "json",
		PluginTagName:   "plugin",
		ConversionHooks: make(map[string]Converter),
		SecretLoader:    secretLoader,
	}
}

func (s *StructToStringMapper) WithConverter(name string, c Converter) *StructToStringMapper {
	s.ConversionHooks[name] = c
	return s
}

func (s *StructToStringMapper) StringsMap(in interface{}) (map[string]string, error) {
	out := make(map[string]string)
	err := s.fillMap(strctVal(in), out)
	return out, err
}

func (s *StructToStringMapper) fillMap(value reflect.Value, out map[string]string) error {
	if out == nil {
		return nil
	}

	fields := s.structFields(value)

	var errs error
	for _, field := range fields {
		errs = errors.Append(errs, s.processField(field, value.FieldByIndex(field.Index), out))
	}
	return errs
}

func (s *StructToStringMapper) processField(field reflect.StructField, value reflect.Value, out map[string]string) error {
	name := field.Name

	tagName, tagOpts := parseTagWithName(field.Tag.Get(s.TagName))
	if tagName != "" {
		name = tagName
	}

	pluginTagOpts := parseTag(field.Tag.Get(s.PluginTagName))
	required := pluginTagOpts.Has("required")
	if pluginTagOpts.Has("hidden") {
		return nil
	}

	if tagOpts.Has("omitempty") {
		if required {
			return errors.Errorf("tags for field %q are conflicting: required and omitempty cannot be set simultaneously", name)
		}
		if value.IsZero() {
			if ok, def := pluginTagOpts.ValueForPrefix("default:"); ok {
				out[name] = def
			}
			return nil
		}
	}

	if ok, converterName := pluginTagOpts.ValueForPrefix("converter:"); ok {
		if hook, ok := s.ConversionHooks[converterName]; ok {
			convertedValue, err := hook(value.Interface())
			if err != nil {
				return errors.WrapIff(err, "failed to convert field %q with converter %q", name, converterName)
			}
			out[name] = convertedValue
			return nil
		} else {
			return errors.Errorf("unable to convert field %q as the specified converter %q is not registered", name, converterName)
		}
	}

	if s.SecretLoader != nil {
		if secretItem, _ := value.Interface().(*secret.Secret); secretItem != nil {
			loadedSecret, err := s.SecretLoader.Load(secretItem)
			if err != nil {
				return errors.WrapIff(err, "failed to load secret for field %q", name)
			}
			out[name] = loadedSecret
			return nil
		}
	}

	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	switch value.Kind() { // nolint:exhaustive
	case reflect.String, reflect.Int, reflect.Bool:
		val := fmt.Sprintf("%v", value.Interface())
		if val == "" {
			// check if default has been set and use it
			if ok, def := pluginTagOpts.ValueForPrefix("default:"); ok {
				val = def
			}
		}
		// can't return an empty string when it's required
		if val == "" && required {
			return errors.Errorf("field %q is required", name)
		}
		out[name] = val
	case reflect.Slice:
		switch actual := value.Interface().(type) {
		case []string, []int:
			if value.Len() > 0 { // nolint:nestif
				b, err := json.Marshal(actual)
				if err != nil {
					return errors.WrapIff(err, "can't marshal field %q with value %v as json", name, actual)
				}
				out[name] = string(b)
			} else {
				if ok, def := pluginTagOpts.ValueForPrefix("default:"); ok {
					if _, ok := actual.([]string); ok {
						def = strings.Join(strings.Split(def, ","), `","`)
						if def != "" {
							def = `"` + def + `"`
						}
					}
					sliceStr := fmt.Sprintf("[%s]", def)
					if _, ok := actual.([]string); !ok {
						if err := json.Unmarshal([]byte(sliceStr), &actual); err != nil {
							return errors.WrapIff(err, "can't unmarshal default value %q into field %q", sliceStr, name)
						}
					}
					out[name] = sliceStr
				}
			}
		}
	case reflect.Map:
		if mapStringString, ok := value.Interface().(map[string]string); ok { // nolint:nestif
			if len(mapStringString) > 0 {
				b, err := json.Marshal(mapStringString)
				if err != nil {
					return errors.WrapIff(err, "can't marshal field %q with value %v as json", name, mapStringString)
				}
				out[name] = string(b)
			} else {
				if ok, def := pluginTagOpts.ValueForPrefix("default:"); ok {
					validate := map[string]string{}
					if err := json.Unmarshal([]byte(def), &validate); err != nil {
						return errors.WrapIff(err, "can't unmarshal default value %q into field %q", def, name)
					}
					out[name] = def
				}
			}
		}
	}
	return nil
}

func strctVal(s interface{}) reflect.Value {
	v := reflect.ValueOf(s)

	// if pointer get the underlying element≤
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		panic("not struct")
	}

	return v
}

func (s *StructToStringMapper) structFields(value reflect.Value) []reflect.StructField {
	t := value.Type()

	var f []reflect.StructField

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// we can't access the value of unexported fields
		if field.PkgPath != "" {
			continue
		}

		// don't check if it's omitted
		if tag := field.Tag.Get(s.TagName); tag == "-" {
			continue
		}

		f = append(f, field)
	}

	return f
}

// parseTag splits a struct field's tag into its name and a list of options
// which comes after a name. A tag is in the form of: "name,option1,option2".
// The name can be neglectected.
func parseTagWithName(tag string) (string, tagOptions) {
	// tag is one of followings:
	// ""
	// "name"
	// "name,opt"
	// "name,opt,opt2"
	// ",opt"

	res := strings.Split(tag, ",")
	return res[0], res[1:]
}

// tagOptions contains a slice of tag options
type tagOptions []string

// Has returns true if the given option is available in tagOptions
func (t tagOptions) Has(opt string) bool {
	for _, tagOpt := range t {
		if tagOpt == opt {
			return true
		}
	}

	return false
}

// Has returns true if the given option is available in tagOptions
func (t tagOptions) ValueForPrefix(opt string) (bool, string) {
	for _, tagOpt := range t {
		if strings.HasPrefix(tagOpt, opt) {
			return true, strings.Replace(tagOpt, opt, "", 1)
		}
	}
	return false, ""
}

// parseTag returns all the options in the tag
func parseTag(tag string) tagOptions {
	return tagOptions(strings.Split(tag, ","))
}
