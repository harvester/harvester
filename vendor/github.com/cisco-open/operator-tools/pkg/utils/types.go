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
	"maps"
	"slices"
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
		maps.Copy(mergedLabels, labels)
	}
	return mergedLabels
}

// DerefOrZero returns the value referenced by p, or the zero-value of the type
func DerefOrZero[T any](p *T) T {
	return DerefOr(p, *new(T))
}

func DerefOr[T any](p *T, defVal T) T {
	if p == nil {
		return defVal
	}
	return *p
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
	return slices.Contains(s, e)
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
