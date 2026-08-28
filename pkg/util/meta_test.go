package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamespacedName(t *testing.T) {
	type input struct {
		namespace string
		name      string
	}
	type output struct {
		namespacedName string
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "Cluster-Scoped Name",
			given: input{
				namespace: "",
				name:      "test",
			},
			expected: output{
				namespacedName: "test",
			},
		},
		{
			name: "Namespaced Name",
			given: input{
				namespace: "default",
				name:      "test",
			},
			expected: output{
				namespacedName: "default/test",
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		actual.namespacedName = NamespacedName(tc.given.namespace, tc.given.name)
		assert.Equal(t, tc.expected, actual)
	}
}

func TestSplitNamespacedName(t *testing.T) {
	testCases := []struct {
		Name              string
		Input             string
		ExpectedNamespace string
		ExpectedName      string
		ExpectedOK        bool
	}{
		{
			Name:              "Namespaced Name",
			Input:             "default/my-resource",
			ExpectedNamespace: "default",
			ExpectedName:      "my-resource",
			ExpectedOK:        true,
		},
		{
			Name:              "Cluster-Scoped Resource",
			Input:             "cluster-resource",
			ExpectedNamespace: "",
			ExpectedName:      "cluster-resource",
			ExpectedOK:        true,
		},
		{
			Name:              "Empty Input",
			Input:             "",
			ExpectedNamespace: "",
			ExpectedName:      "",
			ExpectedOK:        false,
		},
		{
			Name:              "Input with only a slash",
			Input:             "/",
			ExpectedNamespace: "",
			ExpectedName:      "",
			ExpectedOK:        false,
		},
		{
			Name:              "Input with leading slash",
			Input:             "/resource-name",
			ExpectedNamespace: "",
			ExpectedName:      "",
			ExpectedOK:        false,
		},
		{
			Name:              "Input with trailing slash",
			Input:             "default/",
			ExpectedNamespace: "",
			ExpectedName:      "",
			ExpectedOK:        false,
		},
		{
			Name:              "Input with multiple slashes",
			Input:             "kube-system/resource/with-slashes",
			ExpectedNamespace: "kube-system",
			ExpectedName:      "resource/with-slashes",
			ExpectedOK:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			namespace, name, ok := SplitNamespacedName(tc.Input)
			assert.Equal(t, tc.ExpectedNamespace, namespace, "namespace should match")
			assert.Equal(t, tc.ExpectedName, name, "name should match")
			assert.Equal(t, tc.ExpectedOK, ok, "ok flag should match")
		})
	}
}
