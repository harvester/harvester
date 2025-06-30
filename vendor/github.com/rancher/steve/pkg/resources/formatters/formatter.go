package formatters

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/wrangler/v3/pkg/data"
	"github.com/sirupsen/logrus"
	"helm.sh/helm/v3/pkg/release"
	rspb "k8s.io/helm/pkg/proto/hapi/release"
)

var (
	ErrNotHelmRelease = errors.New("not helm release") // error for when it's not a helm release
	magicGzip         = []byte{0x1f, 0x8b, 0x08}       // gzip magic header
)

func HandleHelmData(request *types.APIRequest, resource *types.RawResource) {
	objData := resource.APIObject.Data()
	if q := request.Query.Get("includeHelmData"); q == "true" {
		var helmReleaseData string
		if resource.Type == "secret" {
			b, err := base64.StdEncoding.DecodeString(objData.String("data", "release"))
			if err != nil {
				return
			}
			helmReleaseData = string(b)
		} else {
			helmReleaseData = objData.String("data", "release")
		}
		if objData.String("metadata", "labels", "owner") == "helm" {
			rl, err := decodeHelm3(helmReleaseData)
			if err != nil {
				logrus.Errorf("Failed to decode helm3 release data: %v", err)
				return
			}
			objData.SetNested(rl, "data", "release")
		}
		if objData.String("metadata", "labels", "OWNER") == "TILLER" {
			rl, err := decodeHelm2(helmReleaseData)
			if err != nil {
				logrus.Errorf("Failed to decode helm2 release data: %v", err)
				return
			}
			objData.SetNested(rl, "data", "release")
		}

	} else {
		DropHelmData(objData)
	}
}

func Pod(request *types.APIRequest, resource *types.RawResource) {
	data := resource.APIObject.Data()
	fields := data.StringSlice("metadata", "fields")
	if len(fields) > 2 {
		data.SetNested(convert.LowerTitle(fields[2]), "metadata", "state", "name")
	}
}

// decodeHelm3 receives a helm3 release data string, decodes the string data using the standard base64 library
// and unmarshals the data into release.Release struct to return it.
func decodeHelm3(data string) (*release.Release, error) {
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}

	// Data is too small to be helm 3 release object
	if len(b) <= 3 {
		return nil, ErrNotHelmRelease
	}

	// For backwards compatibility with releases that were stored before
	// compression was introduced we skip decompression if the
	// gzip magic header is not found
	if bytes.Equal(b[0:3], magicGzip) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		b2, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		b = b2
	}

	var rls release.Release
	// unmarshal release object bytes
	if err := json.Unmarshal(b, &rls); err != nil {
		return nil, err
	}
	return &rls, nil
}

// decodeHelm2 receives a helm2 release data and returns the corresponding helm2 release proto struct
func decodeHelm2(data string) (*rspb.Release, error) {
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}

	// For backwards compatibility with releases that were stored before
	// compression was introduced we skip decompression if the
	// gzip magic header is not found
	if bytes.Equal(b[0:3], magicGzip) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		b2, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		b = b2
	}

	var rls rspb.Release
	// unmarshal protobuf bytes
	if err := proto.Unmarshal(b, &rls); err != nil {
		return nil, err
	}
	return &rls, nil
}

func DropHelmData(data data.Object) {
	if data.String("metadata", "labels", "owner") == "helm" ||
		data.String("metadata", "labels", "OWNER") == "TILLER" {
		if data.String("data", "release") != "" {
			delete(data.Map("data"), "release")
		}
	}
}
