package util

import (
	"fmt"
	"reflect"
)

type StructName string

type StructFields []StructField

// StructField contains information about a single field in a struct.
type StructField struct {
	Name  string
	Value interface{}
	Tag   string
}

func (sf *StructFields) Append(name StructName, value interface{}) {
	*sf = append(*sf, StructField{
		Name:  string(name),
		Value: value,
		Tag:   sf.ConvertTag(name),
	})
}

func (sf *StructFields) AppendCounted(structMap map[StructName]int) {
	for name, value := range structMap {
		sf.Append(name, value)
	}
}

func (sf *StructFields) ConvertTag(name StructName) string {
	return ConvertFirstCharToLower(fmt.Sprint(name))
}

// NewStruct creates a new struct based on the StructFields slice.
// Returns the new struct as an interface{}.
func (sf *StructFields) NewStruct() interface{} {
	var fields []reflect.StructField

	for _, f := range *sf {
		// Create a new struct field using reflection
		field := reflect.StructField{
			Name: f.Name,
			Type: reflect.TypeOf(f.Value),
			Tag:  reflect.StructTag(fmt.Sprintf(`json:"%s"`, f.Tag)),
		}

		// Append the struct field to the slice
		fields = append(fields, field)
	}

	// Create a new struct based on the fields slice.
	structOfFields := reflect.StructOf(fields)

	// Create a new instance of the struct and set its field values.
	structValue := reflect.New(structOfFields).Elem()
	for _, f := range *sf {
		structValue.FieldByName(f.Name).Set(reflect.ValueOf(f.Value))
	}
	return structValue.Interface()
}
