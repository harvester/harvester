package kubernetes

import (
	"context"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	whereaboutsv1alpha1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	wbclient "github.com/k8snetworkplumbingwg/whereabouts/pkg/generated/clientset/versioned"
	"github.com/k8snetworkplumbingwg/whereabouts/pkg/logging"
	"github.com/k8snetworkplumbingwg/whereabouts/pkg/storage"
)

const listRequestTimeout = 30 * time.Second

// Client has info on how to connect to the kubernetes cluster
type Client struct {
	client    wbclient.Interface
	clientSet kubernetes.Interface
	retries   int
}

func NewClient() (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return newClient(config)
}

func NewClientViaKubeconfig(kubeconfigPath string) (*Client, error) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{}).ClientConfig()

	if err != nil {
		return nil, err
	}

	return newClient(config)
}

func newClient(config *rest.Config) (*Client, error) {
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	c, err := wbclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return NewKubernetesClient(c, clientSet), nil
}

func NewKubernetesClient(k8sClient wbclient.Interface, k8sClientSet kubernetes.Interface) *Client {
	return &Client{
		client:    k8sClient,
		clientSet: k8sClientSet,
		retries:   storage.DatastoreRetries,
	}
}

func (i *Client) ListIPPools() ([]storage.IPPool, error) {
	logging.Debugf("listing IP pools")

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), listRequestTimeout)
	defer cancel()

	ipPoolList, err := i.client.WhereaboutsV1alpha1().IPPools(metav1.NamespaceAll).List(ctxWithTimeout, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var whereaboutsApiIPPoolList []storage.IPPool
	for idx, pool := range ipPoolList.Items {
		firstIP, _, err := pool.ParseCIDR()
		if err != nil {
			return nil, err
		}
		whereaboutsApiIPPoolList = append(
			whereaboutsApiIPPoolList,
			&KubernetesIPPool{client: i.client, firstIP: firstIP, pool: &ipPoolList.Items[idx]})
	}
	return whereaboutsApiIPPoolList, nil
}

func (i *Client) ListPods() ([]v1.Pod, error) {
	logging.Debugf("listing Pods")

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), listRequestTimeout)
	defer cancel()

	podList, err := i.clientSet.CoreV1().Pods(metav1.NamespaceAll).List(ctxWithTimeout, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return podList.Items, nil
}

func (i *Client) GetPod(namespace, name string) (*v1.Pod, error) {
	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), storage.RequestTimeout)
	defer cancel()

	pod, err := i.clientSet.CoreV1().Pods(namespace).Get(ctxWithTimeout, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return pod, nil
}

func (i *Client) ListOverlappingIPs() ([]whereaboutsv1alpha1.OverlappingRangeIPReservation, error) {
	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), listRequestTimeout)
	defer cancel()

	overlappingIPsList, err := i.client.WhereaboutsV1alpha1().OverlappingRangeIPReservations(metav1.NamespaceAll).List(ctxWithTimeout, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return overlappingIPsList.Items, nil
}

func (i *Client) DeleteOverlappingIP(clusterWideIP *whereaboutsv1alpha1.OverlappingRangeIPReservation) error {
	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), storage.RequestTimeout)
	defer cancel()

	return i.client.WhereaboutsV1alpha1().OverlappingRangeIPReservations(clusterWideIP.GetNamespace()).Delete(
		ctxWithTimeout, clusterWideIP.GetName(), metav1.DeleteOptions{})
}
