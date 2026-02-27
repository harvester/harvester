package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func TestGetOnwerReference(t *testing.T) {
	obj1 := &harvesterv1.VirtualMachineTemplateVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foobar",
			UID:  "abcdef-foobar-xabcdefghijkl",
		},
	}

	ref1, err := GetOwnerReferenceFor(obj1)
	assert.Nil(t, err)
	assert.NotNil(t, ref1)
	assert.Equal(t, obj1.APIVersion, ref1.APIVersion, "api version mismatch")
	assert.Equal(t, obj1.Kind, ref1.Kind, "kind mismatch")
	assert.Equal(t, obj1.Name, ref1.Name, "name mismatch")
	assert.Equal(t, obj1.UID, ref1.UID, "uid mismatch")

	obj2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "barfoo",
			UID:  "defghi-abcdef-foobarbarfoo",
		},
	}

	ref2, err := GetOwnerReferenceFor(obj2)
	assert.Nil(t, err)
	assert.NotNil(t, ref2)
	assert.Equal(t, obj2.APIVersion, ref2.APIVersion, "api version mismatch")
	assert.Equal(t, obj2.Kind, ref2.Kind, "kind mismatch")
	assert.Equal(t, obj2.Name, ref2.Name, "name mismatch")
	assert.Equal(t, obj2.UID, ref2.UID, "uid mismatch")

	ref3, err := GetOwnerReferenceFor(&struct{ Foo string }{Foo: "bar"})
	assert.Nil(t, ref3)
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "struct struct { Foo string } lacks embedded TypeMeta type")
}
