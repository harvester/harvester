package cdi

import (
	"net/http"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
)

type Downloader struct {
}

func GetDownloader() backend.Downloader {
	return &Downloader{}
}

func (cd *Downloader) Do(_ *harvesterv1.VirtualMachineImage, _ http.ResponseWriter, _ *http.Request) error {
	return nil
}
