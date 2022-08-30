package api

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/longhorn/backing-image-manager/pkg/rpc"
)

type BackingImage struct {
	Name             string `json:"name"`
	UUID             string `json:"uuid"`
	Size             int64  `json:"size"`
	ExpectedChecksum string `json:"expectedChecksum"`

	Status BackingImageStatus `json:"status"`
}

type BackingImageStatus struct {
	State                string `json:"state"`
	CurrentChecksum      string `json:"currentChecksum"`
	SendingReference     int    `json:"sendingReference"`
	ErrorMsg             string `json:"errorMsg"`
	SenderManagerAddress string `json:"senderManagerAddress"`
	Progress             int    `json:"progress"`
}

func RPCToBackingImage(obj *rpc.BackingImageResponse) *BackingImage {
	return &BackingImage{
		Name:             obj.Spec.Name,
		UUID:             obj.Spec.Uuid,
		Size:             obj.Spec.Size,
		ExpectedChecksum: obj.Spec.Checksum,

		Status: BackingImageStatus{
			State:                obj.Status.State,
			CurrentChecksum:      obj.Status.Checksum,
			SendingReference:     int(obj.Status.SendingReference),
			ErrorMsg:             obj.Status.ErrorMsg,
			SenderManagerAddress: obj.Status.SenderManagerAddress,
			Progress:             int(obj.Status.Progress),
		},
	}
}

func RPCToBackingImageList(obj *rpc.ListResponse) map[string]*BackingImage {
	ret := map[string]*BackingImage{}
	for name, bi := range obj.BackingImages {
		ret[name] = RPCToBackingImage(bi)
	}
	return ret
}

type BackingImageStream struct {
	conn      *grpc.ClientConn
	ctxCancel context.CancelFunc
	stream    rpc.BackingImageManagerService_WatchClient
}

func NewBackingImageStream(conn *grpc.ClientConn, ctxCancel context.CancelFunc, stream rpc.BackingImageManagerService_WatchClient) *BackingImageStream {
	return &BackingImageStream{
		conn,
		ctxCancel,
		stream,
	}
}

func (s *BackingImageStream) Close() error {
	s.ctxCancel()
	if err := s.conn.Close(); err != nil {
		return errors.Wrapf(err, "error closing backing image watcher gRPC connection")
	}
	return nil
}

func (s *BackingImageStream) Recv() error {
	_, err := s.stream.Recv()
	return err
}

type DataSourceInfo struct {
	SourceType string            `json:"sourceType"`
	Parameters map[string]string `json:"parameters"`

	Name string `json:"name"`

	FileInfo
}

type FileInfo struct {
	DiskUUID         string `json:"diskUUID"`
	ExpectedChecksum string `json:"expectedChecksum"`

	FilePath         string `json:"filePath"`
	UUID             string `json:"uuid"`
	Size             int64  `json:"size"`
	State            string `json:"state"`
	Progress         int    `json:"progress"`
	ProcessedSize    int64  `json:"processedSize"`
	ModificationTime string `json:"modificationTime"`
	CurrentChecksum  string `json:"currentChecksum"`
	Message          string `json:"message"`
	SendingReference int    `json:"sendingReference"`
}

func (in *DataSourceInfo) DeepCopy() *DataSourceInfo {
	out := &DataSourceInfo{
		SourceType: in.SourceType,
		Parameters: make(map[string]string, len(in.Parameters)),

		Name: in.Name,

		FileInfo: in.FileInfo,
	}
	for k, v := range in.Parameters {
		out.Parameters[k] = v
	}
	return out
}
