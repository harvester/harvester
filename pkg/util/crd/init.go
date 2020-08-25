package crd

import (
	"context"
	"fmt"

	"github.com/rancher/wrangler/pkg/crd"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// FromGV returns an abstract namespace-scope CRD handler via group version and kind.
func FromGV(gv schema.GroupVersion, kind string) crd.CRD {
	return crd.FromGV(gv, kind)
}

// NonNamespacedFromGV returns a cluster-scope CRD abstract handler via group version and kind.
func NonNamespacedFromGV(gv schema.GroupVersion, kind string) crd.CRD {
	var c = crd.FromGV(gv, kind)
	c.NonNamespace = true
	return c
}

// NewFactoryFromClient returns a CRD initialization factory.
func NewFactoryFromClient(ctx context.Context, config *rest.Config) (*Factory, error) {
	var f, err = clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	var eg, subCtx = errgroup.WithContext(ctx)
	return &Factory{
		eg:     eg,
		ctx:    subCtx,
		client: f,
	}, nil
}

type Factory struct {
	eg     *errgroup.Group
	ctx    context.Context
	client clientset.Interface
}

// CreateCRDs concurrently creates the target CRDs or overwrites the existed items.
func (f *Factory) CreateCRDs(crds ...crd.CRD) *Factory {
	return f.createCRDs(crds, true)
}

// CreateCRDs concurrently creates the target CRDs which are not existed.
func (f *Factory) CreateCRDsIfNotExisted(crds ...crd.CRD) *Factory {
	return f.createCRDs(crds, false)
}

func (f *Factory) createCRDs(crds []crd.CRD, updateIfExisted bool) *Factory {
	if len(crds) != 0 {
		for _, ci := range crds {
			var ciCopy = ci // copy
			f.eg.Go(func() error {
				var expected, err = ciCopy.ToCustomResourceDefinition()
				if err != nil {
					return fmt.Errorf("failed to convert to CRD %s: %w", ciCopy.GVK, err)
				}

				var crdCli = f.client.ApiextensionsV1beta1().CustomResourceDefinitions()
				actual, err := crdCli.Get(f.ctx, expected.Name, metav1.GetOptions{})
				switch {
				case apierrors.IsForbidden(err):
					logrus.Warnf("No permission to get CRD %s, assuming it is pre-created", expected.Name)
					return nil
				case apierrors.IsNotFound(err):
					_, err = crdCli.Create(f.ctx, &expected, metav1.CreateOptions{})
					if err != nil {
						return fmt.Errorf("failed to create CRD %s: %w", expected.Name, err)
					}
					logrus.Infof("Created CRD %s", expected.Name)
				case err != nil:
					return fmt.Errorf("failed to query CRD %s: %w", expected.Name, err)
				default:
					if !updateIfExisted {
						logrus.Warnf("Do not update the existing CRD %s", expected.Name)
						return nil
					}
					expected.SetResourceVersion(actual.GetResourceVersion())
					_, err = crdCli.Update(f.ctx, &expected, metav1.UpdateOptions{})
					if err != nil {
						return fmt.Errorf("failed to update CRD %s: %w", expected.Name, err)
					}
					logrus.Infof("Updated CRD %s", expected.Name)
				}
				return nil
			})
		}
	}
	return f
}

// Wait waits the creation progress.
func (f *Factory) Wait() error {
	return f.eg.Wait()
}
