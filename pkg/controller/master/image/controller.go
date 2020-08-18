package image

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/minio/minio-go/v6"
	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/util"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	controllerAgentName = "vm-image-controller"
)

var (
	syncProgressInterval = 2 * time.Second
	importIdleTimeout    = 5 * time.Minute
)

func RegisterController(ctx context.Context, management *config.Management) {
	images := management.HarvesterFactory.Harvester().V1alpha1().VirtualMachineImage()
	controller := &Handler{
		images:     images,
		imageCache: images.Cache(),
	}

	images.OnChange(ctx, controllerAgentName, controller.OnImageChanged)
	images.OnRemove(ctx, controllerAgentName, controller.OnImageRemove)
}

// Handler implements harvester image import
type Handler struct {
	images     v1alpha1.VirtualMachineImageController
	imageCache v1alpha1.VirtualMachineImageCache
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
			apisv1alpha1.ImageImported.False(toUpdate)
			apisv1alpha1.ImageImported.Reason(toUpdate, err.Error())
			if _, err := h.UpdateStatusRetryOnConflict(toUpdate); err != nil {
				logrus.Errorln(err)
			}
		}
	}()

	return nil, nil
}

func (h *Handler) OnImageRemove(key string, image *apisv1alpha1.VirtualMachineImage) (*apisv1alpha1.VirtualMachineImage, error) {
	logrus.Debugf("removing %s object in minio", image.Name)
	cleanUpImport(image.Name, image.Status.AppliedURL)
	return image, util.MinioClient.RemoveObject(util.BucketName, image.Name)
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
	size, err := strconv.Atoi(resp.Header.Get("Content-Length"))
	if err != nil {
		return err
	}
	fileSize := int64(size)
	contentType := resp.Header.Get("Content-Type")
	reader := &util.CountingReader{
		Reader: resp.Body,
		Total:  fileSize,
	}

	toUpdate := image.DeepCopy()
	apisv1alpha1.ImageImported.Unknown(toUpdate)
	apisv1alpha1.ImageImported.Message(toUpdate, "started image importing")
	toUpdate.Status.AppliedURL = toUpdate.Spec.URL
	toUpdate.Status.Progress = 0
	toUpdate, err = h.UpdateStatusRetryOnConflict(toUpdate)
	if err != nil {
		return err
	}

	go h.syncProgress(ctx, cancel, reader, image)

	n, err := util.MinioClient.PutObjectWithContext(ctx, util.BucketName, image.Name, reader, fileSize, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return err
	}

	//Succeed
	currentImage, err := h.imageCache.Get(image.Namespace, image.Name)
	if err != nil {
		return err
	}
	toUpdate = currentImage.DeepCopy()
	toUpdate.Status.Progress = 100
	apisv1alpha1.ImageImported.True(toUpdate)
	apisv1alpha1.ImageImported.Message(toUpdate, "completed image importing")
	toUpdate.Status.DownloadURL = fmt.Sprintf("%s/%s/%s", config.ImageStorageEndpoint, util.BucketName, image.Name)
	if _, err := h.UpdateStatusRetryOnConflict(toUpdate); err != nil {
		return err
	}
	logrus.Debugf("Successfully imported image in size %d from %s\n", n, image.Spec.URL)
	return nil
}

func (h *Handler) UpdateStatusRetryOnConflict(image *apisv1alpha1.VirtualMachineImage) (*apisv1alpha1.VirtualMachineImage, error) {
	retry := 3
	for i := 0; i < retry; i++ {
		current, err := h.imageCache.Get(image.Namespace, image.Name)
		if err != nil {
			return nil, err
		}
		if current.DeletionTimestamp != nil {
			return current, nil
		}
		toUpdate := current.DeepCopy()
		toUpdate.Status = image.Status
		current, err = h.images.Update(toUpdate)
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
	image.Status.DownloadURL = ""
}

func (h Handler) syncProgress(ctx context.Context, cancel context.CancelFunc, reader *util.CountingReader, image *apisv1alpha1.VirtualMachineImage) {
	count := reader.Current
	lastUpdate := time.Now()
	for {
		if count < reader.Current {
			progress := int(float32(count) * 100 / float32(reader.Total))
			currentImage, err := h.imageCache.Get(image.Namespace, image.Name)
			if err != nil {
				logrus.Errorf("fail to get image syncing progress: %v", err)
			}
			if currentImage.Status.Progress != progress {
				toUpdate := currentImage.DeepCopy()
				toUpdate.Status.Progress = progress
				if _, err := h.images.Update(toUpdate); err != nil && !apierrors.IsConflict(err) {
					logrus.Errorf("fail to update image syncing progress: %v", err)
				}
			}
			lastUpdate = time.Now()
			count = reader.Current
		}

		if time.Now().After(lastUpdate.Add(importIdleTimeout)) {
			logrus.Errorf("exceeded idle time importing image %q", image.Name)
			cancel()
		}

		if count == reader.Total {
			return
		}

		select {
		case <-time.After(syncProgressInterval):
			continue
		case <-ctx.Done():
			return
		}
	}
}
