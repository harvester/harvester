package client

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	wranglerClients "github.com/rancher/wrangler/pkg/clients"
	wranglerSchemes "github.com/rancher/wrangler/pkg/schemes"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
)

type Clients struct {
	wranglerClients.Clients
	MetricsClient *metricsclientset.Clientset
	Scheme        *runtime.Scheme
	Namespace     string
	Datastore     *datastore.DataStore
	StopCh        <-chan struct{}
}

func NewClients(kubeconfigPath string, stopCh <-chan struct{}) (*Clients, error) {
	namespace := os.Getenv(types.EnvPodNamespace)
	if namespace == "" {
		logrus.Warnf("Cannot detect pod namespace, environment variable %v is missing, using default namespace", types.EnvPodNamespace)
		namespace = corev1.NamespaceDefault
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get client config")
	}

	config.Burst = 100
	config.QPS = 50

	if err := wranglerSchemes.Register(appsv1.AddToScheme); err != nil {
		return nil, err
	}

	// Create k8s client
	clients, err := wranglerClients.NewFromConfig(config, nil)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get k8s client")
	}

	// Create Longhorn client
	lhClient, err := lhclientset.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get clientset")
	}

	scheme := runtime.NewScheme()
	if err := longhorn.SchemeBuilder.AddToScheme(scheme); err != nil {
		return nil, errors.Wrap(err, "unable to create scheme")
	}

	// Create API extension client
	extensionsClient, err := apiextensionsclientset.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get k8s extension client")
	}

	// Create metrics client
	metricsClient, err := metricsclientset.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get metrics client")
	}

	// TODO: there shouldn't be a need for a 30s resync period unless our code is buggy and our controllers aren't really
	//  level based. What we are effectively doing with this is hiding faulty logic in production.
	//  Another reason for increasing this substantially, is that it introduces a lot of unnecessary work and will
	//  lead to scalability problems, since we dump the whole cache of each object back in to the reconciler every 30 seconds.
	//  if a specific controller requires a periodic resync, one enable it only for that informer, add a resync to the event handler, go routine, etc.
	//  some refs to look at: https://github.com/kubernetes-sigs/controller-runtime/issues/521
	informerFactories := util.NewInformerFactories(namespace, clients.K8s, lhClient, 30*time.Second)
	ds := datastore.NewDataStore(namespace, lhClient, clients.K8s, extensionsClient, informerFactories)

	informerFactories.Start(stopCh)
	if !ds.Sync(stopCh) {
		return nil, fmt.Errorf("datastore cache sync up failed")
	}

	return &Clients{
		Clients:       *clients,
		MetricsClient: metricsClient,
		Scheme:        scheme,
		Namespace:     namespace,
		Datastore:     ds,
		StopCh:        stopCh,
	}, nil
}
