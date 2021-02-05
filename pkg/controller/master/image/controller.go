package image

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/util"
)

const (
	controllerAgentName = "vm-image-controller"
)

var (
	syncProgressInterval = 2 * time.Second
	importIdleTimeout    = 5 * time.Minute
)

func RegisterController(ctx context.Context, management *config.Management, options config.Options) {
	images := management.HarvesterFactory.Harvester().V1alpha1().VirtualMachineImage()
	controller := &Handler{
		images:     images,
		imageCache: images.Cache(),
		options:    options,
	}

	images.OnChange(ctx, controllerAgentName, controller.OnImageChanged)
	images.OnRemove(ctx, controllerAgentName, controller.OnImageRemove)
}

// Handler implements harvester image import
type Handler struct {
	images     v1alpha1.VirtualMachineImageClient
	imageCache v1alpha1.VirtualMachineImageCache
	options    config.Options
}

func (h *Handler) OnImageChanged(key string, image *apisv1alpha1.VirtualMachineImage) (*apisv1alpha1.VirtualMachineImage, error) {
	if image == nil {
		return nil, nil
	}

	if image.Spec.URL != "" && image.Status.AppliedURL != "" &&
		image.Spec.URL != image.Status.AppliedURL {
		//Import URL is changed, abort previous import and reset
		cleanUpImport(image.Name, image.Status.AppliedURL)
		toUpdate := image.DeepCopy()
		resetImageStatus(toUpdate)
		return h.images.Update(toUpdate)
	}

	if image.Spec.URL == "" || apisv1alpha1.ImageImported.GetStatus(image) != "" {
		return image, nil
	}

	//do import
	ctx, cancel := context.WithCancel(context.Background())
	if _, loaded := importContexts.LoadOrStore(image.Name, &ImportContext{
		ID:     image.Name,
		URL:    image.Spec.URL,
		Ctx:    ctx,
		Cancel: cancel,
	}); loaded {
		return image, nil
	}

	go func() {
		defer cleanUpImport(image.Name, image.Spec.URL)

		if err := h.importImageToMinio(ctx, cancel, image); err != nil {
			logrus.Errorf("error importing image from %s: %v", image.Spec.URL, err)
			currentImage, getErr := h.imageCache.Get(image.Namespace, image.Name)
			if getErr != nil {
				logrus.Errorln(getErr)
				return
			}
			toUpdate := currentImage.DeepCopy()
			toUpdate.Status.AppliedURL = image.Spec.URL
			apisv1alpha1.ImageImported.False(toUpdate)
			apisv1alpha1.ImageImported.Message(toUpdate, err.Error())
			if _, err := updateStatusRetryOnConflict(h.images, h.imageCache, toUpdate); err != nil {
				logrus.Errorln(err)
			}
		}
	}()

	return image, nil
}

func (h *Handler) OnImageRemove(key string, image *apisv1alpha1.VirtualMachineImage) (*apisv1alpha1.VirtualMachineImage, error) {
	if image == nil {
		return nil, nil
	}
	cleanUpImport(image.Name, image.Status.AppliedURL)
	if !apisv1alpha1.ImageImported.IsTrue(image) {
		return image, nil
	}
	logrus.Debugf("removing image %s in minio", image.Name)
	return image, removeImageFromStorage(image, h.options)
}

func removeImageFromStorage(image *apisv1alpha1.VirtualMachineImage, options config.Options) error {
	mc, err := util.NewMinioClient(options)
	if err != nil {
		return err
	}
	return mc.RemoveObject(util.BucketName, image.Name)
}

func (h *Handler) importImageToMinio(ctx context.Context, cancel context.CancelFunc, image *apisv1alpha1.VirtualMachineImage) error {
	logrus.Debugln("start importing image to minio")
	client := http.Client{
		//timeout by calling cancel when idle time is exceeded
	}
	rr, err := http.NewRequest("GET", image.Spec.URL, nil)
	if err != nil {
		return err
	}
	rr = rr.WithContext(ctx)

	resp, err := client.Do(rr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected 200 status from %s, got %s", image.Spec.URL, resp.Status)
	}

	var fileSize int64
	if resp.Header.Get("Content-Length") == "" {
		// -1 indicates unknown size
		fileSize = -1
	} else {
		fileSize, err = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if err != nil {
			return err
		}
	}
	importer := MinioImporter{
		Images:     h.images,
		ImageCache: h.imageCache,
		Options:    h.options,
	}
	return importer.ImportImageToMinio(ctx, cancel, resp.Body, image, fileSize)
}

func updateStatusRetryOnConflict(images v1alpha1.VirtualMachineImageClient,
	imageCache v1alpha1.VirtualMachineImageCache, image *apisv1alpha1.VirtualMachineImage) (*apisv1alpha1.VirtualMachineImage, error) {
	retry := 3
	for i := 0; i < retry; i++ {
		current, err := imageCache.Get(image.Namespace, image.Name)
		if err != nil {
			return nil, err
		}
		if current.DeletionTimestamp != nil {
			return current, nil
		}
		toUpdate := current.DeepCopy()
		toUpdate.Status = image.Status
		current, err = images.Update(toUpdate)
		if err == nil {
			return current, nil
		}
		if !apierrors.IsConflict(err) {
			return nil, err
		}
		time.Sleep(2 * time.Second)
	}
	return nil, errors.New("failed to update status, max retries exceeded")
}

func resetImageStatus(image *apisv1alpha1.VirtualMachineImage) {
	image.Status.AppliedURL = ""
	apisv1alpha1.ImageImported.SetStatus(image, "")
	apisv1alpha1.ImageImported.Message(image, "")
	image.Status.Progress = 0
	image.Status.DownloadedBytes = 0
	image.Status.DownloadURL = ""
}
