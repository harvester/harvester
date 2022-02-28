package ref

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

func TestAnnotationSchemaReferences_MarshalJSON(t *testing.T) {
	type input struct {
		refs AnnotationSchemaReferences
	}
	type output struct {
		bytes []byte
		err   error
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "include elements",
			given: input{
				refs: NewAnnotationSchemaOwnerReferences("a", "b", "c"),
			},
			expected: output{
				bytes: []byte(`["a","b","c"]`),
				err:   nil,
			},
		},
		{
			name: "none elements",
			given: input{
				refs: NewAnnotationSchemaOwnerReferences(),
			},
			expected: output{
				bytes: []byte(`[]`),
				err:   nil,
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		actual.bytes, actual.err = json.Marshal(tc.given.refs)
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

func TestAnnotationSchemaReferences_UnmarshalJSON(t *testing.T) {
	type input struct {
		bytes []byte
	}
	type output struct {
		refs AnnotationSchemaReferences
		err  error
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "include elements",
			given: input{
				bytes: []byte(`["a","b","c"]`),
			},
			expected: output{
				refs: NewAnnotationSchemaOwnerReferences("a", "b", "c"),
				err:  nil,
			},
		},
		{
			name: "none elements",
			given: input{
				bytes: []byte(`[]`),
			},
			expected: output{
				refs: NewAnnotationSchemaOwnerReferences(),
				err:  nil,
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		actual.err = json.Unmarshal(tc.given.bytes, &actual.refs)
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

func TestAnnotationSchemaOwners_MarshalJSON(t *testing.T) {
	type input struct {
		owners AnnotationSchemaOwners
	}
	type output struct {
		bytes []byte
		err   error
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "include elements",
			given: input{
				owners: AnnotationSchemaOwners{
					"b": AnnotationSchemaReference{
						SchemaID:   "b",
						References: NewAnnotationSchemaOwnerReferences("b-d", "b-a", "b-c"),
					},
					"a": AnnotationSchemaReference{
						SchemaID:   "a",
						References: NewAnnotationSchemaOwnerReferences("a-a", "a-c", "a-b"),
					},
				},
			},
			expected: output{
				bytes: []byte(`[{"schema":"a","refs":["a-a","a-b","a-c"]},{"schema":"b","refs":["b-a","b-c","b-d"]}]`),
				err:   nil,
			},
		},
		{
			name: "none elements",
			given: input{
				owners: AnnotationSchemaOwners{},
			},
			expected: output{
				bytes: []byte(`[]`),
				err:   nil,
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		actual.bytes, actual.err = json.Marshal(tc.given.owners)
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

func TestAnnotationSchemaOwners_UnmarshalJSON(t *testing.T) {
	type input struct {
		bytes []byte
	}
	type output struct {
		owners AnnotationSchemaOwners
		err    error
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "include elements",
			given: input{
				bytes: []byte(`[{"schema":"b","refs":["b-a","b-c","b-d"]},{"schema":"a","refs":["a-a","a-c","a-b"]}]`),
			},
			expected: output{
				owners: AnnotationSchemaOwners{
					"a": AnnotationSchemaReference{
						SchemaID:   "a",
						References: NewAnnotationSchemaOwnerReferences("a-a", "a-c", "a-b"),
					},
					"b": AnnotationSchemaReference{
						SchemaID:   "b",
						References: NewAnnotationSchemaOwnerReferences("b-a", "b-c", "b-d"),
					},
				},
				err: nil,
			},
		},
		{
			name: "merge the duplicated schemaID elements",
			given: input{
				bytes: []byte(`[{"schema":"b","refs":["b-a","b-c","b-d"]},{"schema":"a","refs":["a-a","a-c","a-b"]},{"schema":"a","refs":["a-d"]}]`),
			},
			expected: output{
				owners: AnnotationSchemaOwners{
					"a": AnnotationSchemaReference{
						SchemaID:   "a",
						References: NewAnnotationSchemaOwnerReferences("a-a", "a-c", "a-b", "a-d"),
					},
					"b": AnnotationSchemaReference{
						SchemaID:   "b",
						References: NewAnnotationSchemaOwnerReferences("b-a", "b-c", "b-d"),
					},
				},
				err: nil,
			},
		},
		{
			name: "none elements",
			given: input{
				bytes: []byte(`[]`),
			},
			expected: output{
				owners: AnnotationSchemaOwners{},
				err:    nil,
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		actual.err = json.Unmarshal(tc.given.bytes, &actual.owners)
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

func TestAnnotationSchemaOwners_Add(t *testing.T) {
	type input struct {
		schemaOwners AnnotationSchemaOwners
		ownerGK      schema.GroupKind
		owner        metav1.Object
	}
	type output struct {
		schemaOwners AnnotationSchemaOwners
		ret          bool
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore if owned",
			given: input{
				schemaOwners: AnnotationSchemaOwners{
					"kubevirt.io.virtualmachine": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachine",
						References: NewAnnotationSchemaOwnerReferences("default/zzz", "default/test"),
					},
				},
				ownerGK: kubevirtv1.VirtualMachineGroupVersionKind.GroupKind(),
				owner: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
				},
			},
			expected: output{
				schemaOwners: AnnotationSchemaOwners{
					"kubevirt.io.virtualmachine": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachine",
						References: NewAnnotationSchemaOwnerReferences("default/zzz", "default/test"),
					},
				},
				ret: false,
			},
		},
		{
			name: "add if ownless",
			given: input{
				schemaOwners: AnnotationSchemaOwners{
					"kubevirt.io.virtualmachine": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachine",
						References: NewAnnotationSchemaOwnerReferences("default/zzz"),
					},
					"kubevirt.io.virtualmachineinstance": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachineinstance",
						References: NewAnnotationSchemaOwnerReferences("default/yyy"),
					},
				},
				ownerGK: kubevirtv1.VirtualMachineGroupVersionKind.GroupKind(),
				owner: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
				},
			},
			expected: output{
				schemaOwners: AnnotationSchemaOwners{
					"kubevirt.io.virtualmachine": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachine",
						References: NewAnnotationSchemaOwnerReferences("default/zzz", "default/test"),
					},
					"kubevirt.io.virtualmachineinstance": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachineinstance",
						References: NewAnnotationSchemaOwnerReferences("default/yyy"),
					},
				},
				ret: true,
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		var schemaOwners = tc.given.schemaOwners
		actual.ret = schemaOwners.Add(tc.given.ownerGK, tc.given.owner)
		actual.schemaOwners = schemaOwners
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

func TestAnnotationSchemaOwners_Delete(t *testing.T) {
	type input struct {
		schemaOwners AnnotationSchemaOwners
		ownerGK      schema.GroupKind
		owner        metav1.Object
	}
	type output struct {
		schemaOwners AnnotationSchemaOwners
		ret          bool
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore if ownless",
			given: input{
				schemaOwners: AnnotationSchemaOwners{
					"kubevirt.io.virtualmachine": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachine",
						References: NewAnnotationSchemaOwnerReferences("default/zzz"),
					},
				},
				ownerGK: kubevirtv1.VirtualMachineGroupVersionKind.GroupKind(),
				owner: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
				},
			},
			expected: output{
				schemaOwners: AnnotationSchemaOwners{
					"kubevirt.io.virtualmachine": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachine",
						References: NewAnnotationSchemaOwnerReferences("default/zzz"),
					},
				},
				ret: false,
			},
		},
		{
			name: "remove if owned",
			given: input{
				schemaOwners: AnnotationSchemaOwners{
					"kubevirt.io.virtualmachine": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachine",
						References: NewAnnotationSchemaOwnerReferences("default/zzz", "default/test"),
					},
					"kubevirt.io.virtualmachineinstance": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachineinstance",
						References: NewAnnotationSchemaOwnerReferences("default/yyy"),
					},
				},
				ownerGK: kubevirtv1.VirtualMachineGroupVersionKind.GroupKind(),
				owner: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
				},
			},
			expected: output{
				schemaOwners: AnnotationSchemaOwners{
					"kubevirt.io.virtualmachine": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachine",
						References: NewAnnotationSchemaOwnerReferences("default/zzz"),
					},
					"kubevirt.io.virtualmachineinstance": AnnotationSchemaReference{
						SchemaID:   "kubevirt.io.virtualmachineinstance",
						References: NewAnnotationSchemaOwnerReferences("default/yyy"),
					},
				},
				ret: true,
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		var schemaOwners = tc.given.schemaOwners
		actual.ret = schemaOwners.Remove(tc.given.ownerGK, tc.given.owner)
		actual.schemaOwners = schemaOwners
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}
