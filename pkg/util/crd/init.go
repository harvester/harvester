package crd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/name"
	"github.com/sirupsen/logrus"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
)

// FromGV returns an abstract namespace-scope CRD handler via group version and kind.
func FromGV(gv schema.GroupVersion, kind string, obj interface{}) crd.CRD {
	return crd.FromGV(gv, kind).WithSchemaFromStruct(obj)
}

// NonNamespacedFromGV returns a cluster-scope CRD abstract handler via group version and kind.
func NonNamespacedFromGV(gv schema.GroupVersion, kind string, obj interface{}) crd.CRD {
	var c = crd.FromGV(gv, kind).WithSchemaFromStruct(obj)
	c.NonNamespace = true
	return c
}

// NewFactoryFromClient returns a CRD initialization factory.
func NewFactoryFromClient(ctx context.Context, config *rest.Config) (*Factory, error) {
	apply, err := apply.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	f, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Factory{
		ctx:    ctx,
		client: f,
		apply:  apply.WithDynamicLookup().WithNoDelete(),
	}, nil
}

type Factory struct {
	err    error
	wg     sync.WaitGroup
	ctx    context.Context
	client clientset.Interface
	apply  apply.Apply
}

func (f *Factory) BatchWait() error {
	f.wg.Wait()
	return f.err
}

// CreateCRDsIfNotExisted concurrently creates the target CRDs which are not existed.
func (f *Factory) BatchCreateCRDsIfNotExisted(crds ...crd.CRD) *Factory {
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		if _, err := f.CreateCRDs(f.ctx, false, crds...); err != nil && f.err == nil {
			f.err = err
		}
	}()
	return f

}

func getCRDName(pluralName, group string) string {
	return fmt.Sprintf("%s.%s", pluralName, group)
}

func (f *Factory) CreateCRDs(ctx context.Context, updateIfExisted bool, crds ...crd.CRD) (map[schema.GroupVersionKind]*apiext.CustomResourceDefinition, error) {
	if len(crds) == 0 {
		return nil, nil
	}

	if ok, err := f.ensureAccess(ctx); err != nil {
		return nil, err
	} else if !ok {
		logrus.Infof("No access to list CRDs, assuming CRDs are pre-created.")
		return nil, err
	}

	crdStatus := map[schema.GroupVersionKind]*apiext.CustomResourceDefinition{}

	ready, err := f.getReadyCRDs(ctx)
	if err != nil {
		return nil, err
	}

	for _, crdDef := range crds {
		plural := crdDef.PluralName
		if len(plural) == 0 {
			plural = strings.ToLower(name.GuessPluralName(crdDef.GVK.Kind))
		}
		crdName := getCRDName(plural, crdDef.GVK.Group)

		// skip to update existing CRD
		if !updateIfExisted && ready[crdName] != nil {
			logrus.Infof("Do not update the existing CRD %s", crdDef.GVK.Kind)
			continue
		}

		crd, err := f.createCRD(ctx, crdDef, ready)
		if err != nil {
			return nil, err
		}
		crdStatus[crdDef.GVK] = crd
	}

	ready, err = f.getReadyCRDs(ctx)
	if err != nil {
		return nil, err
	}

	for gvk, crd := range crdStatus {
		if readyCrd, ok := ready[crd.Name]; ok {
			crdStatus[gvk] = readyCrd
		} else {
			if err := f.waitCRD(ctx, crd.Name, gvk, crdStatus); err != nil {
				return nil, err
			}
		}
	}

	return crdStatus, nil
}

func (f *Factory) createCRD(ctx context.Context, crdDef crd.CRD, ready map[string]*apiext.CustomResourceDefinition) (*apiext.CustomResourceDefinition, error) {
	crd, err := crdDef.ToCustomResourceDefinition()
	if err != nil {
		return nil, err
	}

	meta, err := meta.Accessor(crd)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Applying CRD %s", meta.GetName())
	if err := f.apply.WithOwner(crd).ApplyObjects(crd); err != nil {
		return nil, err
	}

	return f.client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, meta.GetName(), metav1.GetOptions{})

}

func (f *Factory) ensureAccess(ctx context.Context) (bool, error) {
	_, err := f.client.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if apierrors.IsForbidden(err) {
		return false, nil
	}
	return true, err
}

func (f *Factory) getReadyCRDs(ctx context.Context) (map[string]*apiext.CustomResourceDefinition, error) {
	list, err := f.client.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := map[string]*apiext.CustomResourceDefinition{}

	for i, crd := range list.Items {
		for _, cond := range crd.Status.Conditions {
			switch cond.Type {
			case apiext.Established:
				if cond.Status == apiext.ConditionTrue {
					result[crd.Name] = &list.Items[i]
				}
			}
		}
	}

	return result, nil
}

func (f *Factory) waitCRD(ctx context.Context, crdName string, gvk schema.GroupVersionKind, crdStatus map[schema.GroupVersionKind]*apiext.CustomResourceDefinition) error {
	logrus.Infof("Waiting for CRD %s to become available", crdName)
	defer logrus.Infof("Done waiting for CRD %s to become available", crdName)

	first := true
	return wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		if !first {
			logrus.Infof("Waiting for CRD %s to become available", crdName)
		}
		first = false

		crd, err := f.client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		for _, cond := range crd.Status.Conditions {
			switch cond.Type {
			case apiext.Established:
				if cond.Status == apiext.ConditionTrue {
					crdStatus[gvk] = crd
					return true, err
				}
			case apiext.NamesAccepted:
				if cond.Status == apiext.ConditionFalse {
					logrus.Infof("Name conflict on %s: %v\n", crdName, cond.Reason)
				}
			}
		}

		return false, ctx.Err()
	})
}
