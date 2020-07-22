package image

import (
	"context"

	"github.com/rancher/vm/pkg/apis/vm.cattle.io/v1alpha1"
	pkgcontext "github.com/rancher/vm/pkg/context"
	"github.com/sirupsen/logrus"
)

func Register(ctx context.Context, management *pkgcontext.Management) error {
	c := imageController{}
	management.VMFactory.Vm().V1alpha1().Image().OnChange(ctx, "setting-controller-sample", c.sync)
	return nil
}

type imageController struct {
}

func (c *imageController) sync(key string, obj *v1alpha1.Image) (*v1alpha1.Image, error) {
	logrus.Infof("syncing key %s", key)
	return obj, nil
}
