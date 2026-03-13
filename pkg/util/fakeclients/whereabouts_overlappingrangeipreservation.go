package fakeclients

import (
	"context"

	whereaboutsv1alpha1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	whereaboutstype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/whereabouts.cni.cncf.io/v1alpha1"
)

type WhereaboutsOverlappingRangeIPReservationClient func() whereaboutstype.OverlappingRangeIPReservationInterface

func (c WhereaboutsOverlappingRangeIPReservationClient) Create(reservation *whereaboutsv1alpha1.OverlappingRangeIPReservation) (*whereaboutsv1alpha1.OverlappingRangeIPReservation, error) {
	reservationCopy := reservation.DeepCopy()
	reservationCopy.Namespace = ""
	return c().Create(context.TODO(), reservationCopy, metav1.CreateOptions{})
}

func (c WhereaboutsOverlappingRangeIPReservationClient) Update(reservation *whereaboutsv1alpha1.OverlappingRangeIPReservation) (*whereaboutsv1alpha1.OverlappingRangeIPReservation, error) {
	reservationCopy := reservation.DeepCopy()
	reservationCopy.Namespace = ""
	return c().Update(context.TODO(), reservationCopy, metav1.UpdateOptions{})
}

func (c WhereaboutsOverlappingRangeIPReservationClient) UpdateStatus(*whereaboutsv1alpha1.OverlappingRangeIPReservation) (*whereaboutsv1alpha1.OverlappingRangeIPReservation, error) {
	panic("implement me")
}

func (c WhereaboutsOverlappingRangeIPReservationClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c WhereaboutsOverlappingRangeIPReservationClient) Get(namespace, name string, options metav1.GetOptions) (*whereaboutsv1alpha1.OverlappingRangeIPReservation, error) {
	return c().Get(context.TODO(), name, options)
}

func (c WhereaboutsOverlappingRangeIPReservationClient) List(namespace string, opts metav1.ListOptions) (*whereaboutsv1alpha1.OverlappingRangeIPReservationList, error) {
	return c().List(context.TODO(), opts)
}

func (c WhereaboutsOverlappingRangeIPReservationClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c WhereaboutsOverlappingRangeIPReservationClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *whereaboutsv1alpha1.OverlappingRangeIPReservation, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c WhereaboutsOverlappingRangeIPReservationClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*whereaboutsv1alpha1.OverlappingRangeIPReservation, *whereaboutsv1alpha1.OverlappingRangeIPReservationList], error) {
	panic("implement me")
}

type WhereaboutsOverlappingRangeIPReservationCache func() whereaboutstype.OverlappingRangeIPReservationInterface

func (c WhereaboutsOverlappingRangeIPReservationCache) Get(namespace, name string) (*whereaboutsv1alpha1.OverlappingRangeIPReservation, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c WhereaboutsOverlappingRangeIPReservationCache) List(namespace string, selector labels.Selector) ([]*whereaboutsv1alpha1.OverlappingRangeIPReservation, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*whereaboutsv1alpha1.OverlappingRangeIPReservation, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, nil
}

func (c WhereaboutsOverlappingRangeIPReservationCache) AddIndexer(_ string, _ generic.Indexer[*whereaboutsv1alpha1.OverlappingRangeIPReservation]) {
	panic("implement me")
}

func (c WhereaboutsOverlappingRangeIPReservationCache) GetByIndex(_, _ string) ([]*whereaboutsv1alpha1.OverlappingRangeIPReservation, error) {
	panic("implement me")
}
