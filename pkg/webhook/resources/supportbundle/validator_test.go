package supportbundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestSupportBundleValidator_Create(t *testing.T) {
	tests := []struct {
		name          string
		supportBundle *harvesterv1.SupportBundle
		namespaces    []*corev1.Namespace
		expectedError bool
		errorMessage  string
	}{
		{
			name: "valid support bundle with existing namespaces",
			supportBundle: &harvesterv1.SupportBundle{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bundle",
					Namespace: "default",
				},
				Spec: harvesterv1.SupportBundleSpec{
					Description:               "test bundle",
					ExtraCollectionNamespaces: []string{"default", "kube-system"},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "kube-system",
					},
				},
			},
			expectedError: false,
		},
		{
			name: "support bundle with non-existing namespace",
			supportBundle: &harvesterv1.SupportBundle{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bundle",
					Namespace: "default",
				},
				Spec: harvesterv1.SupportBundleSpec{
					Description:               "test bundle",
					ExtraCollectionNamespaces: []string{"default", "non-existing-namespace"},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			expectedError: true,
			errorMessage:  "namespace non-existing-namespace not found",
		},
		{
			name: "support bundle with empty namespace",
			supportBundle: &harvesterv1.SupportBundle{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bundle",
					Namespace: "default",
				},
				Spec: harvesterv1.SupportBundleSpec{
					Description:               "test bundle",
					ExtraCollectionNamespaces: []string{"default", ""},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			expectedError: false,
		},
		{
			name: "support bundle with no extra namespaces",
			supportBundle: &harvesterv1.SupportBundle{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bundle",
					Namespace: "default",
				},
				Spec: harvesterv1.SupportBundleSpec{
					Description:               "test bundle",
					ExtraCollectionNamespaces: []string{},
				},
			},
			namespaces:    []*corev1.Namespace{},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coreclientset := corefake.NewSimpleClientset()
			for _, ns := range tt.namespaces {
				err := coreclientset.Tracker().Add(ns.DeepCopy())
				assert.Nil(t, err, "Mock resource should add into fake controller tracker")
			}

			fakeNamespaceCache := fakeclients.NamespaceCache(coreclientset.CoreV1().Namespaces)

			validator := NewValidator(fakeNamespaceCache).(*supportBundleValidator)

			err := validator.Create(nil, tt.supportBundle)

			if tt.expectedError {
				assert.NotNil(t, err, tt.name)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage, tt.name)
				}
			} else {
				assert.Nil(t, err, tt.name)
			}
		})
	}
}
