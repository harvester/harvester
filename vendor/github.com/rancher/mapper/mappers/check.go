package mappers

import (
	"fmt"

	types "github.com/rancher/mapper"
)

func ValidateFields(schemas *types.Schema, fields ...string) error {
	for _, f := range fields {
		if err := ValidateField(f, schemas); err != nil {
			return err
		}
	}

	return nil
}

func ValidateField(field string, schema *types.Schema) error {
	if _, ok := schema.ResourceFields[field]; !ok {
		return fmt.Errorf("field %s missing on schema %s", field, schema.ID)
	}

	return nil
}
