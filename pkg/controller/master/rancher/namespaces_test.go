package rancher

import (
	"testing"

	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"
)

var (
	projectAnnotationValue        = "c-fz4vx:p-lbm6g"
	invalidProjectAnnotationValue = "c-fz4vx:p-98lps"

	harvesterSystemNamespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-system",
		},
	}

	forkliftNamespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "forklift",
			Annotations: map[string]string{
				projectAnnotationKey: invalidProjectAnnotationValue,
			},
		},
	}

	demoNamespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-demo",
		},
	}
)

func Test_ProjectAddition(t *testing.T) {
	clientset := corefake.NewSimpleClientset(harvesterSystemNamespace, forkliftNamespace, demoNamespace)

	h := &Handler{
		NamespaceClient: fakeclients.NamespaceClient(clientset.CoreV1().Namespaces),
		NamespaceCache:  fakeclients.NamespaceCache(clientset.CoreV1().Namespaces),
	}

	assert := require.New(t)
	err := h.addProjectIDToHarvesterSystemNamespaces(projectAnnotationValue)
	assert.NoError(err, "expected no error during namespace reconcile")
	updatedNS, err := h.NamespaceCache.Get(harvesterSystemNamespace.Name)
	assert.NoError(err, "expected no error when getting namespace from cache")
	assert.Equal(projectAnnotationValue, updatedNS.Annotations[projectAnnotationKey], "expected to find  project annotation in harvester system namespace")
}

func Test_ProjectRemoval(t *testing.T) {
	clientset := corefake.NewSimpleClientset(harvesterSystemNamespace, forkliftNamespace, demoNamespace)

	h := &Handler{
		NamespaceClient: fakeclients.NamespaceClient(clientset.CoreV1().Namespaces),
		NamespaceCache:  fakeclients.NamespaceCache(clientset.CoreV1().Namespaces),
	}

	assert := require.New(t)
	err := h.removeProjectIDFromHarvesterSystemNamespaces()
	assert.NoError(err, "expected no error during namespace reconcile")
	updatedNS, err := h.NamespaceCache.Get(forkliftNamespace.Name)
	assert.NoError(err, "expected no error when getting namespace from cache")
	assert.Empty(updatedNS.Annotations[projectAnnotationKey], "expected project annotation to be removed from forklift namespace")
}
