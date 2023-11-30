package util

import (
	"time"

	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	lhinformers "github.com/longhorn/longhorn-manager/k8s/pkg/client/informers/externalversions"
)

type InformerFactories struct {
	KubeInformerFactory                  informers.SharedInformerFactory
	KubeNamespaceFilteredInformerFactory informers.SharedInformerFactory
	LhInformerFactory                    lhinformers.SharedInformerFactory
}

func NewInformerFactories(namespace string, kubeClient clientset.Interface, lhClient versioned.Interface, resyncPeriod time.Duration) *InformerFactories {
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, resyncPeriod)
	kubeNamespaceFilteredInformerFactory := informers.NewSharedInformerFactoryWithOptions(kubeClient, resyncPeriod, informers.WithNamespace(namespace))
	lhInformerFactory := lhinformers.NewSharedInformerFactory(lhClient, resyncPeriod)

	return &InformerFactories{
		KubeInformerFactory:                  kubeInformerFactory,
		KubeNamespaceFilteredInformerFactory: kubeNamespaceFilteredInformerFactory,
		LhInformerFactory:                    lhInformerFactory,
	}
}

func (f *InformerFactories) Start(stopCh <-chan struct{}) {
	go f.KubeInformerFactory.Start(stopCh)
	go f.KubeNamespaceFilteredInformerFactory.Start(stopCh)
	go f.LhInformerFactory.Start(stopCh)
}
