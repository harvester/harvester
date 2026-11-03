package formatters

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	// helm v2 is long since deprecated
	// Unlike helm v3, it uses Protobuf encoding, so we can't use generic decoding without the message descriptors.
	// The relevant types were vendored, since they won't change anymore. We should consider dropping support for Helm v2.
	rspb "github.com/rancher/steve/pkg/resources/formatters/internal/legacytypes/helmv2api"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/norman/types/convert"
	rescommon "github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/wrangler/v3/pkg/data"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/duration"
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

func Pod(_ *types.APIRequest, resource *types.RawResource) {
	objData := resource.APIObject.Data()
	fields := objData.StringSlice("metadata", "fields")
	if len(fields) > 2 {
		objData.SetNested(convert.LowerTitle(fields[2]), "metadata", "state", "name")
	}

	if resource.Schema == nil {
		return
	}

	cols := rescommon.GetColumnDefinitions(resource.Schema)
	for _, col := range cols {
		if col.Name != "Restarts" {
			continue
		}

		index := rescommon.GetIndexValueFromString(col.Field)
		if index == -1 {
			continue
		}

		rawFieldsRaw, ok := data.GetValue(objData, "metadata", "fields")
		if !ok {
			continue
		}

		rawFields, ok := rawFieldsRaw.([]interface{})
		if !ok || index >= len(rawFields) {
			continue
		}

		valMap, ok := rawFields[index].(map[string]interface{})
		if !ok {
			continue
		}

		rawFields[index] = FormatRestarts(valMap)
		objData.SetNested(rawFields, "metadata", "fields")
	}
}

// FormatRestarts formats a restart map as a display string.
func FormatRestarts(valMap map[string]interface{}) string {
	var count int64
	var timestamp int64

	if c, ok := valMap["count"]; ok {
		switch v := c.(type) {
		case int64:
			count = v
		case int:
			count = int64(v)
		case float64:
			count = int64(v)
		}
	}

	if t, ok := valMap["timestamp"]; ok {
		switch v := t.(type) {
		case int64:
			timestamp = v
		case int:
			timestamp = int64(v)
		case float64:
			timestamp = int64(v)
		}
	}

	if timestamp == 0 {
		return fmt.Sprintf("%d", count)
	}

	t := time.UnixMilli(timestamp)
	dur := time.Since(t)
	humanDur := duration.HumanDuration(dur)
	return fmt.Sprintf("%d (%s ago)", count, humanDur)
}

// decodeHelm3 receives a helm3 release data string, decodes the string data using the standard base64 library
// and unmarshals the data into release.Release struct to return it.
func decodeHelm3(data string) (any, error) {
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}

	// Data is too small to be helm 3 release object
	if len(b) <= 3 {
		return nil, ErrNotHelmRelease
	}
	var r io.Reader = bytes.NewReader(b)

	// For backwards compatibility with releases that were stored before
	// compression was introduced we skip decompression if the
	// gzip magic header is not found
	if bytes.Equal(b[0:3], magicGzip) {
		gzr, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		defer gzr.Close()
		r = gzr
	}

	var rls json.RawMessage
	// unmarshal JSON release payload
	if err := json.NewDecoder(r).Decode(&rls); err != nil {
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
		defer r.Close()

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
