package engineapi

import (
	"fmt"

	bimapi "github.com/longhorn/backing-image-manager/api"
	bimclient "github.com/longhorn/backing-image-manager/pkg/client"
)

type BackingImageDataSourceInfo struct {
	DiskUUID   string            `json:"diskUUID"`
	SourceType string            `json:"sourceType"`
	Parameters map[string]string `json:"parameters"`

	Name            string `json:"name"`
	UUID            string `json:"uuid"`
	FilePath        string `json:"filePath"`
	State           string `json:"state"`
	Size            int64  `json:"size"`
	Progress        int    `json:"progress"`
	ProcessedSize   int64  `json:"processedSize"`
	CurrentChecksum string `json:"currentChecksum"`
	Message         string `json:"message"`
}

type BackingImageDataSourceClient struct {
	client bimclient.DataSourceClient
}

func NewBackingImageDataSourceClient(ip string) *BackingImageDataSourceClient {
	return &BackingImageDataSourceClient{
		bimclient.DataSourceClient{
			Remote: fmt.Sprintf("%s:%d", ip, BackingImageDataSourceDefaultPort),
		},
	}
}

func (c *BackingImageDataSourceClient) parseDataSourceInfo(info *bimapi.DataSourceInfo) *BackingImageDataSourceInfo {
	if info == nil {
		return nil
	}
	return &BackingImageDataSourceInfo{
		DiskUUID:   info.DiskUUID,
		SourceType: info.SourceType,
		Parameters: info.Parameters,

		Name:            info.Name,
		UUID:            info.UUID,
		FilePath:        info.FilePath,
		State:           info.State,
		Size:            info.Size,
		ProcessedSize:   info.ProcessedSize,
		Progress:        info.Progress,
		CurrentChecksum: info.CurrentChecksum,
		Message:         info.Message,
	}
}

func (c *BackingImageDataSourceClient) Get() (*BackingImageDataSourceInfo, error) {
	dsInfo, err := c.client.Get()
	if err != nil {
		return nil, err
	}
	return c.parseDataSourceInfo(dsInfo), nil
}

func (c *BackingImageDataSourceClient) Transfer() error {
	return c.client.Transfer()
}
