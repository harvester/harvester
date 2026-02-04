// Copyright © 2020 Banzai Cloud
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

package utils

import (
	"fmt"
	"hash/fnv"
	"sort"

	"emperror.dev/errors"
	"github.com/iancoleman/orderedmap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// MergeLabels merge into map[string]string map
func MergeLabels(labelGroups ...map[string]string) map[string]string {
	mergedLabels := make(map[string]string)
	for _, labels := range labelGroups {
		for k, v := range labels {
			mergedLabels[k] = v
		}
	}
	return mergedLabels
}

func PointerToBool(b *bool) bool {
	if b == nil {
		return false
	}

	return *b
}

func PointerToUint(i *uint) uint {
	if i == nil {
		return 0
	}

	return *i
}

func PointerToInt(i *int) int {
	if i == nil {
		return 0
	}

	return *i
}

func PointerToInt32(i *int32) int32 {
	if i == nil {
		return 0
	}

	return *i
}

func PointerToString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// IntPointer converts int32 to *int32
func IntPointer(i int32) *int32 {
	return &i
}

// IntPointer converts int64 to *int64
func IntPointer64(i int64) *int64 {
	return &i
}

// BoolPointer converts bool to *bool
func BoolPointer(b bool) *bool {
	return &b
}

// StringPointer converts string to *string
func StringPointer(s string) *string {
	return &s
}

// OrderedStringMap
func OrderedStringMap(original map[string]string) *orderedmap.OrderedMap {
	o := orderedmap.New()
	for k, v := range original {
		o.Set(k, v)
	}
	o.SortKeys(sort.Strings)
	return o
}

// Contains check if a string item exists in []string
func Contains(s []string, e string) bool {
	for _, i := range s {
		if i == e {
			return true
		}
	}
	return false
}

// Hash32 calculate for string
func Hash32(in string) (string, error) {
	hasher := fnv.New32()
	_, err := hasher.Write([]byte(in))
	if err != nil {
		return "", errors.WrapIf(err, "failed to calculate hash for the configmap data")
	}
	return fmt.Sprintf("%x", hasher.Sum32()), nil
}

func ObjectKeyFromObjectMeta(o v1.Object) types.NamespacedName {
	return types.NamespacedName{Namespace: o.GetNamespace(), Name: o.GetName()}
}
