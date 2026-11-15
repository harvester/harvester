package fakeclients

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
)

// KubernetesClient forwards typed requests to the same tracker and reactors as
// Wrangler's generated client, including simulated failures and UID assignment.
func KubernetesClient(client *fake.Clientset) kubernetes.Interface {
	result := kubefake.NewSimpleClientset()
	result.PrependReactor("*", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		object, err := client.Invokes(action, nil)
		return true, object, err
	})
	return result
}
