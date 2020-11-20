package mappers

import (
	"encoding/base64"
	"strings"

	"github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
	"github.com/rancher/mapper/values"
	"github.com/sirupsen/logrus"
)

type Base64 struct {
	Field            string
	IgnoreDefinition bool
	Separator        string
}

func (m Base64) FromInternal(data map[string]interface{}) {
	if v, ok := values.RemoveValue(data, strings.Split(m.Field, m.getSep())...); ok {
		str := convert.ToString(v)
		if str == "" {
			return
		}

		newData, err := base64.StdEncoding.DecodeString(str)
		if err != nil {
			logrus.Errorf("failed to base64 decode data")
		}

		values.PutValue(data, string(newData), strings.Split(m.Field, m.getSep())...)
	}
}

func (m Base64) ToInternal(data map[string]interface{}) error {
	if v, ok := values.RemoveValue(data, strings.Split(m.Field, m.getSep())...); ok {
		str := convert.ToString(v)
		if str == "" {
			return nil
		}

		newData := base64.StdEncoding.EncodeToString([]byte(str))
		values.PutValue(data, newData, strings.Split(m.Field, m.getSep())...)
	}

	return nil
}

func (m Base64) ModifySchema(s *mapper.Schema, schemas *mapper.Schemas) error {
	if !m.IgnoreDefinition {
		if err := ValidateField(m.Field, s); err != nil {
			return err
		}
	}

	return nil
}

func (m Base64) getSep() string {
	if m.Separator == "" {
		return "/"
	}
	return m.Separator
}
