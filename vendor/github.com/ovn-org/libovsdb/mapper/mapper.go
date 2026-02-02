package mapper

import (
	"fmt"
	"reflect"

	"github.com/ovn-org/libovsdb/ovsdb"
)

// Mapper offers functions to interact with libovsdb through user-provided native structs.
// The way to specify what field of the struct goes
// to what column in the database id through field a field tag.
// The tag used is "ovsdb" and has the following structure
// 'ovsdb:"${COLUMN_NAME}"'
//	where COLUMN_NAME is the name of the column and must match the schema
//
//Example:
//  type MyObj struct {
//  	Name string `ovsdb:"name"`
//  }
type Mapper struct {
	Schema ovsdb.DatabaseSchema
}

// ErrMapper describes an error in an Mapper type
type ErrMapper struct {
	objType   string
	field     string
	fieldType string
	fieldTag  string
	reason    string
}

func (e *ErrMapper) Error() string {
	return fmt.Sprintf("Mapper Error. Object type %s contains field %s (%s) ovs tag %s: %s",
		e.objType, e.field, e.fieldType, e.fieldTag, e.reason)
}

// NewMapper returns a new mapper
func NewMapper(schema ovsdb.DatabaseSchema) Mapper {
	return Mapper{
		Schema: schema,
	}
}

// GetRowData transforms a Row to a struct based on its tags
// The result object must be given as pointer to an object with the right tags
func (m Mapper) GetRowData(row *ovsdb.Row, result *Info) error {
	if row == nil {
		return nil
	}
	return m.getData(*row, result)
}

// getData transforms a map[string]interface{} containing OvS types (e.g: a ResultRow
// has this format) to orm struct
// The result object must be given as pointer to an object with the right tags
func (m Mapper) getData(ovsData ovsdb.Row, result *Info) error {
	for name, column := range result.Metadata.TableSchema.Columns {
		if !result.hasColumn(name) {
			// If provided struct does not have a field to hold this value, skip it
			continue
		}

		ovsElem, ok := ovsData[name]
		if !ok {
			// Ignore missing columns
			continue
		}

		nativeElem, err := ovsdb.OvsToNative(column, ovsElem)
		if err != nil {
			return fmt.Errorf("table %s, column %s: failed to extract native element: %s",
				result.Metadata.TableName, name, err.Error())
		}

		if err := result.SetField(name, nativeElem); err != nil {
			return err
		}
	}
	return nil
}

// NewRow transforms an orm struct to a map[string] interface{} that can be used as libovsdb.Row
// By default, default or null values are skipped. This behavior can be modified by specifying
// a list of fields (pointers to fields in the struct) to be added to the row
func (m Mapper) NewRow(data *Info, fields ...interface{}) (ovsdb.Row, error) {
	columns := make(map[string]*ovsdb.ColumnSchema)
	for k, v := range data.Metadata.TableSchema.Columns {
		columns[k] = v
	}
	columns["_uuid"] = &ovsdb.UUIDColumn
	ovsRow := make(map[string]interface{}, len(columns))
	for name, column := range columns {
		nativeElem, err := data.FieldByColumn(name)
		if err != nil {
			// If provided struct does not have a field to hold this value, skip it
			continue
		}

		// add specific fields
		if len(fields) > 0 {
			found := false
			for _, f := range fields {
				col, err := data.ColumnByPtr(f)
				if err != nil {
					return nil, err
				}
				if col == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if len(fields) == 0 && ovsdb.IsDefaultValue(column, nativeElem) {
			continue
		}
		ovsElem, err := ovsdb.NativeToOvs(column, nativeElem)
		if err != nil {
			return nil, fmt.Errorf("table %s, column %s: failed to generate ovs element. %s", data.Metadata.TableName, name, err.Error())
		}
		ovsRow[name] = ovsElem
	}
	return ovsRow, nil
}

// NewEqualityCondition returns a list of equality conditions that match a given object
// A list of valid columns that shall be used as a index can be provided.
// If none are provided, we will try to use object's field that matches the '_uuid' ovsdb tag
// If it does not exist or is null (""), then we will traverse all of the table indexes and
// use the first index (list of simultaneously unique columns) for which the provided mapper
// object has valid data. The order in which they are traversed matches the order defined
// in the schema.
// By `valid data` we mean non-default data.
func (m Mapper) NewEqualityCondition(data *Info, fields ...interface{}) ([]ovsdb.Condition, error) {
	var conditions []ovsdb.Condition
	var condIndex [][]string

	// If index is provided, use it. If not, obtain the valid indexes from the mapper info
	if len(fields) > 0 {
		providedIndex := []string{}
		for i := range fields {
			if col, err := data.ColumnByPtr(fields[i]); err == nil {
				providedIndex = append(providedIndex, col)
			} else {
				return nil, err
			}
		}
		condIndex = append(condIndex, providedIndex)
	} else {
		var err error
		condIndex, err = data.getValidIndexes()
		if err != nil {
			return nil, err
		}
	}

	if len(condIndex) == 0 {
		return nil, fmt.Errorf("failed to find a valid index")
	}

	// Pick the first valid index
	for _, col := range condIndex[0] {
		field, err := data.FieldByColumn(col)
		if err != nil {
			return nil, err
		}

		column := data.Metadata.TableSchema.Column(col)
		if column == nil {
			return nil, fmt.Errorf("column %s not found", col)
		}
		ovsVal, err := ovsdb.NativeToOvs(column, field)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, ovsdb.NewCondition(col, ovsdb.ConditionEqual, ovsVal))
	}
	return conditions, nil
}

// EqualFields compares two mapped objects.
// The indexes to use for comparison are, the _uuid, the table indexes and the columns that correspond
// to the mapped fields pointed to by 'fields'. They must be pointers to fields on the first mapped element (i.e: one)
func (m Mapper) EqualFields(one, other *Info, fields ...interface{}) (bool, error) {
	indexes := []string{}
	for _, f := range fields {
		col, err := one.ColumnByPtr(f)
		if err != nil {
			return false, err
		}
		indexes = append(indexes, col)
	}
	return m.equalIndexes(one, other, indexes...)
}

// NewCondition returns a ovsdb.Condition based on the model
func (m Mapper) NewCondition(data *Info, field interface{}, function ovsdb.ConditionFunction, value interface{}) (*ovsdb.Condition, error) {
	column, err := data.ColumnByPtr(field)
	if err != nil {
		return nil, err
	}

	// Check that the condition is valid
	columnSchema := data.Metadata.TableSchema.Column(column)
	if columnSchema == nil {
		return nil, fmt.Errorf("column %s not found", column)
	}
	if err := ovsdb.ValidateCondition(columnSchema, function, value); err != nil {
		return nil, err
	}

	ovsValue, err := ovsdb.NativeToOvs(columnSchema, value)
	if err != nil {
		return nil, err
	}

	ovsdbCondition := ovsdb.NewCondition(column, function, ovsValue)

	return &ovsdbCondition, nil

}

// NewMutation creates a RFC7047 mutation object based on an ORM object and the mutation fields (in native format)
// It takes care of field validation against the column type
func (m Mapper) NewMutation(data *Info, column string, mutator ovsdb.Mutator, value interface{}) (*ovsdb.Mutation, error) {
	// Check the column exists in the object
	if !data.hasColumn(column) {
		return nil, fmt.Errorf("mutation contains column %s that does not exist in object %v", column, data)
	}
	// Check that the mutation is valid
	columnSchema := data.Metadata.TableSchema.Column(column)
	if columnSchema == nil {
		return nil, fmt.Errorf("column %s not found", column)
	}
	if err := ovsdb.ValidateMutation(columnSchema, mutator, value); err != nil {
		return nil, err
	}

	var ovsValue interface{}
	var err error
	// Usually a mutation value is of the same type of the value being mutated
	// except for delete mutation of maps where it can also be a list of same type of
	// keys (rfc7047 5.1). Handle this special case here.
	if mutator == "delete" && columnSchema.Type == ovsdb.TypeMap && reflect.TypeOf(value).Kind() != reflect.Map {
		// It's OK to cast the value to a list of elements because validation has passed
		ovsSet, err := ovsdb.NewOvsSet(value)
		if err != nil {
			return nil, err
		}
		ovsValue = ovsSet
	} else {
		ovsValue, err = ovsdb.NativeToOvs(columnSchema, value)
		if err != nil {
			return nil, err
		}
	}

	return &ovsdb.Mutation{Column: column, Mutator: mutator, Value: ovsValue}, nil
}

// equalIndexes returns whether both models are equal from the DB point of view
// Two objects are considered equal if any of the following conditions is true
// They have a field tagged with column name '_uuid' and their values match
// For any of the indexes defined in the Table Schema, the values all of its columns are simultaneously equal
// (as per RFC7047)
// The values of all of the optional indexes passed as variadic parameter to this function are equal.
func (m Mapper) equalIndexes(one, other *Info, indexes ...string) (bool, error) {
	match := false

	oneIndexes, err := one.getValidIndexes()
	if err != nil {
		return false, err
	}

	otherIndexes, err := other.getValidIndexes()
	if err != nil {
		return false, err
	}

	oneIndexes = append(oneIndexes, indexes)
	otherIndexes = append(otherIndexes, indexes)

	for _, lidx := range oneIndexes {
		for _, ridx := range otherIndexes {
			if reflect.DeepEqual(ridx, lidx) {
				// All columns in an index must be simultaneously equal
				for _, col := range lidx {
					if !one.hasColumn(col) || !other.hasColumn(col) {
						break
					}
					lfield, err := one.FieldByColumn(col)
					if err != nil {
						return false, err
					}
					rfield, err := other.FieldByColumn(col)
					if err != nil {
						return false, err
					}
					if reflect.DeepEqual(lfield, rfield) {
						match = true
					} else {
						match = false
						break
					}
				}
				if match {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
