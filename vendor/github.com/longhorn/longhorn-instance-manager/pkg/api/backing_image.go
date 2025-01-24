package api

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"

	rpc "github.com/longhorn/types/pkg/generated/imrpc"
)

type BackingImage struct {
	Name             string `json:"name"`
	BackingImageUUID string `json:"backing_image_uuid"`
	DiskUUID         string `json:"disk_uuid"`
	Size             uint64 `json:"size"`
	ExpectedChecksum string `json:"expected_checksum"`

	Status BackingImageStatus `json:"status"`
}

type BackingImageStatus struct {
	Progress        int    `json:"progress"`
	State           string `json:"state"`
	CurrentChecksum string `json:"currentChecksum"`
	ErrorMsg        string `json:"errorMsg"`
}

func RPCToBackingImage(obj *rpc.SPDKBackingImageResponse) (*BackingImage, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot convert nil SPDKBackingImageResponse")
	}
	if obj.Spec == nil {
		return nil, fmt.Errorf("backing image spec is nil")
	}
	bi := &BackingImage{
		Name:             obj.Spec.Name,
		BackingImageUUID: obj.Spec.BackingImageUuid,
		DiskUUID:         obj.Spec.DiskUuid,
		Size:             obj.Spec.Size,
		ExpectedChecksum: obj.Spec.Checksum,

		Status: BackingImageStatus{
			Progress:        int(obj.Status.Progress),
			State:           obj.Status.State,
			CurrentChecksum: obj.Status.Checksum,
			ErrorMsg:        obj.Status.ErrorMsg,
		},
	}

	return bi, nil
}

func RPCToBackingImageList(obj *rpc.SPDKBackingImageListResponse) map[string]*BackingImage {
	ret := map[string]*BackingImage{}
	for name, bi := range obj.BackingImages {
		res, err := RPCToBackingImage(bi)
		if err != nil {
			logrus.WithError(err).Warnf("failed to convert backing image %v", name)
		}
		ret[name] = res
	}
	return ret
}

type BackingImageStream struct {
	stream rpc.ProxyEngineService_SPDKBackingImageWatchClient
}

func NewBackingImageStream(stream rpc.ProxyEngineService_SPDKBackingImageWatchClient) *BackingImageStream {
	return &BackingImageStream{
		stream,
	}
}

func (s *BackingImageStream) Recv() (*emptypb.Empty, error) {
	return s.stream.Recv()
}
