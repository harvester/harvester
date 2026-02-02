package ovsdb

import (
	"fmt"
)

// ExpandNamedUUIDs replaces named UUIDs in columns that contain UUID types
// throughout the operation. The caller must ensure each input operation has
// a valid UUID, which may be replaced if a previous operation created a
// matching named UUID mapping. Returns the updated operations or an error.
func ExpandNamedUUIDs(ops []Operation, schema *DatabaseSchema) ([]Operation, error) {
	uuidMap := make(map[string]string)

	// Pass 1: replace the named UUID with a real UUID for each operation and
	// build the substitution map
	for i := range ops {
		op := &ops[i]
		if op.Op != OperationInsert {
			// Only Insert operations can specify a Named UUID
			continue
		}

		if err := ValidateUUID(op.UUID); err != nil {
			return nil, fmt.Errorf("operation UUID %q invalid: %v", op.UUID, err)
		}

		if op.UUIDName != "" {
			if uuid, ok := uuidMap[op.UUIDName]; ok {
				if op.UUID != "" && op.UUID != uuid {
					return nil, fmt.Errorf("named UUID %q maps to UUID %q but found existing UUID %q",
						op.UUIDName, uuid, op.UUID)
				}
				// If there's already a mapping for this named UUID use it
				op.UUID = uuid
			} else {
				uuidMap[op.UUIDName] = op.UUID
			}
			op.UUIDName = ""
		}
	}

	// Pass 2: replace named UUIDs in operation fields with the real UUID
	for i := range ops {
		op := &ops[i]
		tableSchema := schema.Table(op.Table)
		if tableSchema == nil {
			return nil, fmt.Errorf("table %q not found in schema %q", op.Table, schema.Name)
		}

		for i, condition := range op.Where {
			newVal, err := expandColumnNamedUUIDs(tableSchema, op.Table, condition.Column, condition.Value, uuidMap)
			if err != nil {
				return nil, err
			}
			op.Where[i].Value = newVal
		}
		for i, mutation := range op.Mutations {
			newVal, err := expandColumnNamedUUIDs(tableSchema, op.Table, mutation.Column, mutation.Value, uuidMap)
			if err != nil {
				return nil, err
			}
			op.Mutations[i].Value = newVal
		}
		for _, row := range op.Rows {
			for k, v := range row {
				newVal, err := expandColumnNamedUUIDs(tableSchema, op.Table, k, v, uuidMap)
				if err != nil {
					return nil, err
				}
				row[k] = newVal
			}
		}
		for k, v := range op.Row {
			newVal, err := expandColumnNamedUUIDs(tableSchema, op.Table, k, v, uuidMap)
			if err != nil {
				return nil, err
			}
			op.Row[k] = newVal
		}
	}

	return ops, nil
}

func expandColumnNamedUUIDs(tableSchema *TableSchema, tableName, columnName string, value interface{}, uuidMap map[string]string) (interface{}, error) {
	column := tableSchema.Column(columnName)
	if column == nil {
		return nil, fmt.Errorf("column %q not found in table %q", columnName, tableName)
	}
	return expandNamedUUID(column, value, uuidMap), nil
}

func expandNamedUUID(column *ColumnSchema, value interface{}, namedUUIDs map[string]string) interface{} {
	var keyType, valType ExtendedType

	switch column.Type {
	case TypeUUID:
		keyType = column.Type
	case TypeSet:
		keyType = column.TypeObj.Key.Type
	case TypeMap:
		keyType = column.TypeObj.Key.Type
		valType = column.TypeObj.Value.Type
	}

	if valType == TypeUUID {
		if m, ok := value.(OvsMap); ok {
			for k, v := range m.GoMap {
				if newUUID, ok := expandNamedUUIDAtomic(keyType, k, namedUUIDs); ok {
					m.GoMap[newUUID] = m.GoMap[k]
					delete(m.GoMap, k)
					k = newUUID
				}
				if newUUID, ok := expandNamedUUIDAtomic(valType, v, namedUUIDs); ok {
					m.GoMap[k] = newUUID
				}
			}
		}
	} else if keyType == TypeUUID {
		if ovsSet, ok := value.(OvsSet); ok {
			for i, s := range ovsSet.GoSet {
				if newUUID, ok := expandNamedUUIDAtomic(keyType, s, namedUUIDs); ok {
					ovsSet.GoSet[i] = newUUID
				}
			}
			return value
		} else if strSet, ok := value.([]string); ok {
			for i, s := range strSet {
				if newUUID, ok := expandNamedUUIDAtomic(keyType, s, namedUUIDs); ok {
					strSet[i] = newUUID.(string)
				}
			}
			return value
		} else if uuidSet, ok := value.([]UUID); ok {
			for i, s := range uuidSet {
				if newUUID, ok := expandNamedUUIDAtomic(keyType, s, namedUUIDs); ok {
					uuidSet[i] = newUUID.(UUID)
				}
			}
			return value
		}

		if newUUID, ok := expandNamedUUIDAtomic(keyType, value, namedUUIDs); ok {
			return newUUID
		}
	}

	// No expansion required; return original value
	return value
}

func expandNamedUUIDAtomic(valueType ExtendedType, value interface{}, namedUUIDs map[string]string) (interface{}, bool) {
	if valueType == TypeUUID {
		if uuid, ok := value.(UUID); ok {
			if newUUID, ok := namedUUIDs[uuid.GoUUID]; ok {
				return UUID{GoUUID: newUUID}, true
			}
		} else if uuid, ok := value.(string); ok {
			if newUUID, ok := namedUUIDs[uuid]; ok {
				return newUUID, true
			}
		}
	}
	return value, false
}
