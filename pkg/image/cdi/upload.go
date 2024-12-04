package cdi

import (
	"net/http"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
)

type Uploader struct {
}

func GetUploader() backend.Uploader {
	return &Uploader{}
}

func (cu *Uploader) Do(_ *harvesterv1.VirtualMachineImage, _ *http.Request) error {

	return nil
}
