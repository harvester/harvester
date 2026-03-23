package manager

import (
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

var (
	WaitForEngineImageCount    = 20
	WaitForEngineImageInterval = 6 * time.Second
)

func (m *VolumeManager) ListEngineImagesByName() (map[string]*longhorn.EngineImage, error) {
	return m.ds.ListEngineImages()
}

func (m *VolumeManager) ListEngineImagesSorted() ([]*longhorn.EngineImage, error) {
	engineImageMap, err := m.ListEngineImagesByName()
	if err != nil {
		return []*longhorn.EngineImage{}, err
	}

	engineImages := make([]*longhorn.EngineImage, len(engineImageMap))
	engineImageNames, err := util.SortKeys(engineImageMap)
	if err != nil {
		return []*longhorn.EngineImage{}, err
	}
	for i, engineImageName := range engineImageNames {
		engineImages[i] = engineImageMap[engineImageName]
	}
	return engineImages, nil

}

func (m *VolumeManager) GetEngineImageByName(name string) (*longhorn.EngineImage, error) {
	return m.ds.GetEngineImage(name)
}

func (m *VolumeManager) GetEngineImageByImage(image string) (*longhorn.EngineImage, error) {
	name := types.GetEngineImageChecksumName(image)
	return m.ds.GetEngineImage(name)
}

func (m *VolumeManager) CreateEngineImage(image string) (*longhorn.EngineImage, error) {
	name := types.GetEngineImageChecksumName(image)
	ei := &longhorn.EngineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: types.GetEngineImageLabels(name),
		},
		Spec: longhorn.EngineImageSpec{
			Image: image,
		},
	}
	ei, err := m.ds.CreateEngineImage(ei)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Created engine image %v (%v)", ei.Name, ei.Spec.Image)
	return ei, nil
}

func (m *VolumeManager) DeleteEngineImage(name string) error {
	err := m.ds.DeleteEngineImage(name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	logrus.Infof("Deleted engine image %v", name)
	return nil
}

func (m *VolumeManager) DeployEngineImage(image string) error {
	if _, err := m.GetEngineImageByImage(image); err != nil {
		if !datastore.ErrorIsNotFound(err) {
			return errors.Wrapf(err, "cannot get engine image %v", image)
		}
		if _, err = m.CreateEngineImage(image); err != nil {
			return errors.Wrapf(err, "cannot create engine image for %v", image)
		}
	}
	return nil
}
