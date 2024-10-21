package settings

import (
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
)

type Image struct {
	Repository      string            `json:"repository"`
	Tag             interface{}       `json:"tag"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy"`
}

func (i Image) ImageName() string {
	tag := i.GetTag()

	if tag == "" || i.Repository == "" {
		return ""
	}

	return fmt.Sprintf("%s:%s", i.Repository, tag)
}

// GetTag gets correct format tag from Chart.Values
// The tag might be "v12.2", 12.2 and "12.2" etc.
// So we need to convert it in the case.
func (i Image) GetTag() string {
	var tag string

	if i.Tag == nil {
		return tag
	}

	switch t := i.Tag.(type) {
	case string:
		tag = t
	case int, int32, int64:
		tag = fmt.Sprintf("%d", t)
	case float64:
		dec := decimal.NewFromFloat(t)
		tag = dec.String()
	case float32:
		dec := decimal.NewFromFloat32(t)
		tag = dec.String()
	}

	return tag
}

func GetImage(setting Setting) *Image {
	imageStr := setting.Get()
	if imageStr == "{}" || imageStr == "" {
		return nil
	}

	var image Image
	err := json.Unmarshal([]byte(imageStr), &image)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"setting": setting.Name,
			"image":   imageStr,
		}).Errorf("fail to parse image")
		return nil
	}
	return &image
}
