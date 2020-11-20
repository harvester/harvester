package mappers

import (
	types "github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
	"github.com/sirupsen/logrus"
)

type MaybeStringer interface {
	MaybeString() interface{}
}

type StringerFactory func() MaybeStringer
type ToObject func(interface{}) (interface{}, error)

type ObjectsToSlice struct {
	Field     string
	NewObject StringerFactory
	ToObject  ToObject
}

func (p ObjectsToSlice) FromInternal(data map[string]interface{}) {
	if data == nil {
		return
	}

	objs, ok := data[p.Field]
	if !ok {
		return
	}

	var result []interface{}
	for _, obj := range convert.ToMapSlice(objs) {
		target := p.NewObject()
		if err := convert.ToObj(obj, target); err != nil {
			logrus.Errorf("Failed to unmarshal slice to object: %v", err)
			continue
		}

		ret := target.MaybeString()
		if slc, ok := ret.([]string); ok {
			for _, v := range slc {
				result = append(result, v)
			}
		} else {
			result = append(result, ret)
		}
	}

	if len(result) == 0 {
		delete(data, p.Field)
	} else {
		data[p.Field] = result
	}
}

func (p ObjectsToSlice) ToInternal(data map[string]interface{}) error {
	if data == nil {
		return nil
	}

	d, ok := data[p.Field]
	if !ok {
		return nil
	}

	if str, ok := d.(string); ok {
		d = []interface{}{str}
	}

	slc, ok := d.([]interface{})
	if !ok {
		return nil
	}

	var newSlc []interface{}

	for _, obj := range slc {
		n, err := convert.ToNumber(obj)
		if err == nil && n > 0 {
			obj = convert.ToString(n)
		}
		newObj, err := p.ToObject(obj)
		if err != nil {
			return err
		}

		if mapSlice, isMapSlice := newObj.([]map[string]interface{}); isMapSlice {
			for _, v := range mapSlice {
				newSlc = append(newSlc, v)
			}
		} else {
			if _, isMap := newObj.(map[string]interface{}); !isMap {
				newObj, err = convert.EncodeToMap(newObj)
			}

			newSlc = append(newSlc, newObj)
		}
	}

	data[p.Field] = newSlc
	return nil
}

func (p ObjectsToSlice) ModifySchema(schema *types.Schema, schemas *types.Schemas) error {
	return ValidateField(p.Field, schema)
}
