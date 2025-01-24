package engineapi

import (
	"context"

	"github.com/pkg/errors"

	imapi "github.com/longhorn/longhorn-instance-manager/pkg/api"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (p *Proxy) SPDKBackingImageCreate(name, backingImageUUID, diskUUID, checksum, fromAddress, srcDiskUUID string, size uint64) (*imapi.BackingImage, error) {
	return p.grpcClient.SPDKBackingImageCreate(name, backingImageUUID, diskUUID, checksum, fromAddress, srcDiskUUID, size)
}

func (p *Proxy) SPDKBackingImageDelete(name, diskUUID string) error {
	return p.grpcClient.SPDKBackingImageDelete(name, diskUUID)
}

func (p *Proxy) SPDKBackingImageGet(name, diskUUID string) (*imapi.BackingImage, error) {
	return p.grpcClient.SPDKBackingImageGet(name, diskUUID)
}

func (p *Proxy) SPDKBackingImageList() (map[string]longhorn.BackingImageV2CopyInfo, error) {
	result := map[string]longhorn.BackingImageV2CopyInfo{}

	v2BackingImages, err := p.grpcClient.SPDKBackingImageList()
	if err != nil {
		return nil, err
	}

	for name, backingImage := range v2BackingImages {
		result[name] = *parseBackingImage(backingImage)
	}

	return result, nil
}

func (p *Proxy) SPDKBackingImageWatch(ctx context.Context) (*imapi.BackingImageStream, error) {
	return p.grpcClient.SPDKBackingImageWatch(ctx)
}

func parseBackingImage(bi *imapi.BackingImage) *longhorn.BackingImageV2CopyInfo {
	return &longhorn.BackingImageV2CopyInfo{
		Name:     bi.Name,
		UUID:     bi.BackingImageUUID,
		DiskUUID: bi.DiskUUID,
		Size:     int64(bi.Size),

		Progress:        bi.Status.Progress,
		State:           longhorn.BackingImageState(bi.Status.State),
		CurrentChecksum: bi.Status.CurrentChecksum,
		Message:         bi.Status.ErrorMsg,
	}
}

func (e *EngineBinary) SPDKBackingImageCreate(name, backingImageUUID, diskUUID, checksum, fromAddress, srcDiskUUID string, size uint64) (*imapi.BackingImage, error) {
	return nil, errors.New(ErrNotImplement)
}

func (e *EngineBinary) SPDKBackingImageDelete(name, diskUUID string) error {
	return errors.New(ErrNotImplement)
}

func (e *EngineBinary) SPDKBackingImageGet(name, diskUUID string) (*imapi.BackingImage, error) {
	return nil, errors.New(ErrNotImplement)
}

func (e *EngineBinary) SPDKBackingImageList() (map[string]longhorn.BackingImageV2CopyInfo, error) {
	return nil, errors.New(ErrNotImplement)
}

func (e *EngineBinary) SPDKBackingImageWatch(ctx context.Context) (*imapi.BackingImageStream, error) {
	return nil, errors.New(ErrNotImplement)
}
