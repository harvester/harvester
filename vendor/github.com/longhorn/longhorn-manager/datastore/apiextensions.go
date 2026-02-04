package datastore

import (
	"context"

	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/runtime"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateCustomResourceDefinition creates a CustomResourceDefinition resource with the given CustomResourceDefinition object
func (s *DataStore) CreateCustomResourceDefinition(crd *apiextensionsv1.CustomResourceDefinition) (*apiextensionsv1.CustomResourceDefinition, error) {
	ret, err := s.extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), crd, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "custom resource definition", func(name string) (runtime.Object, error) {
		return s.GetCustomResourceDefinition(name)
	})
	if err != nil {
		return nil, err
	}

	ret, ok := obj.(*apiextensionsv1.CustomResourceDefinition)
	if !ok {
		return nil, errors.Errorf("BUG: datastore: verifyCreation returned wrong type for CustomResourceDefinition")
	}

	return ret.DeepCopy(), nil
}

// GetCustomResourceDefinition get the CustomResourceDefinition resource of the given name
func (s *DataStore) GetCustomResourceDefinition(name string) (*apiextensionsv1.CustomResourceDefinition, error) {
	return s.extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), name, metav1.GetOptions{})
}

// UpdateCustomResourceDefinition updates the CustomResourceDefinition resource with the given object and verifies update
func (s *DataStore) UpdateCustomResourceDefinition(crd *apiextensionsv1.CustomResourceDefinition) (*apiextensionsv1.CustomResourceDefinition, error) {
	obj, err := s.extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Update(context.TODO(), crd, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	verifyUpdate(crd.Name, obj, func(name string) (runtime.Object, error) {
		return s.GetCustomResourceDefinition(name)
	})
	return obj, nil
}
