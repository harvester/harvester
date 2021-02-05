package image

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v6"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/util"
)

type MinioImporter struct {
	Images     v1alpha1.VirtualMachineImageClient
	ImageCache v1alpha1.VirtualMachineImageCache
	Options    config.Options
}

func (h *MinioImporter) ImportImageToMinio(ctx context.Context, cancel context.CancelFunc, readCloser io.ReadCloser, image *apisv1alpha1.VirtualMachineImage, fileSize int64) error {
	logrus.Debugln("start importing image to minio")
	reader := &util.CountingReader{
		Reader: readCloser,
		Total:  fileSize,
	}

	toUpdate := image.DeepCopy()
	apisv1alpha1.ImageImported.Unknown(toUpdate)
	apisv1alpha1.ImageImported.Message(toUpdate, "started image importing")
	toUpdate.Status.AppliedURL = toUpdate.Spec.URL
	toUpdate.Status.Progress = 0

	if _, err := updateStatusRetryOnConflict(h.Images, h.ImageCache, toUpdate); err != nil {
		return err
	}

	go h.syncProgress(ctx, cancel, reader, image)

	mc, err := util.NewMinioClient(h.Options)
	if err != nil {
		return err
	}
	uploaded, err := mc.PutObjectWithContext(ctx, util.BucketName, image.Name, reader, fileSize, minio.PutObjectOptions{})
	if err != nil {
		return err
	}

	//Succeed
	currentImage, err := h.ImageCache.Get(image.Namespace, image.Name)
	if err != nil {
		return err
	}
	toUpdate = currentImage.DeepCopy()
	toUpdate.Status.Progress = 100
	toUpdate.Status.DownloadedBytes = uploaded
	apisv1alpha1.ImageImported.True(toUpdate)
	apisv1alpha1.ImageImported.Message(toUpdate, "completed image importing")
	toUpdate.Status.DownloadURL = fmt.Sprintf("%s/%s/%s", h.Options.ImageStorageEndpoint, util.BucketName, image.Name)
	if _, err := updateStatusRetryOnConflict(h.Images, h.ImageCache, toUpdate); err != nil {
		return err
	}
	logrus.Debugf("Successfully imported image in size %d from %s\n", uploaded, image.Spec.URL)
	return nil
}

func (h MinioImporter) syncProgress(ctx context.Context, cancel context.CancelFunc, reader *util.CountingReader, image *apisv1alpha1.VirtualMachineImage) {
	var count int64
	var progress int
	lastUpdate := time.Now()
	for {
		if count < reader.Current {
			currentImage, err := h.ImageCache.Get(image.Namespace, image.Name)
			if err != nil {
				logrus.Errorf("fail to get image syncing progress: %v", err)
			}

			if currentImage.Status.DownloadedBytes != reader.Current {
				toUpdate := currentImage.DeepCopy()
				if reader.Total > 0 {
					progress = int(float32(reader.Current) * 100 / float32(reader.Total))
					toUpdate.Status.Progress = progress
				} else {
					toUpdate.Status.Progress = -1
				}
				toUpdate.Status.DownloadedBytes = reader.Current
				if _, err := h.Images.Update(toUpdate); err != nil && !apierrors.IsConflict(err) {
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
