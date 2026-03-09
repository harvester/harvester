package model

import (
	"fmt"
	"reflect"

	"github.com/ovn-org/libovsdb/mapper"
	"github.com/ovn-org/libovsdb/ovsdb"
)

// ColumnKey addresses a column and optionally a key within a column
type ColumnKey struct {
	Column string
	Key    interface{}
}

// ClientIndex defines a client index by a set of columns
type ClientIndex struct {
	Columns []ColumnKey
}

// ClientDBModel contains the client information needed to build a DatabaseModel
type ClientDBModel struct {
	name    string
	types   map[string]reflect.Type
	indexes map[string][]ClientIndex
}

// NewModel returns a new instance of a model from a specific string
func (db ClientDBModel) newModel(table string) (Model, error) {
	mtype, ok := db.types[table]
	if !ok {
		return nil, fmt.Errorf("table %s not found in database model", string(table))
	}
	model := reflect.New(mtype.Elem())
	return model.Interface().(Model), nil
}

// Name returns the database name
func (db ClientDBModel) Name() string {
	return db.name
}

// Indexes returns the client indexes for a model
func (db ClientDBModel) Indexes(table string) []ClientIndex {
	if len(db.indexes) == 0 {
		return nil
	}
	if _, ok := db.indexes[table]; ok {
		return copyIndexes(db.indexes)[table]
	}
	return nil
}

// SetIndexes sets the client indexes. Client indexes are optional, similar to
// schema indexes and are only tracked in the specific client instances that are
// provided with this client model. A client index may point to multiple models
// as uniqueness is not enforced. They are defined per table and multiple
// indexes can be defined for a table. Each index consists of a set of columns.
// If the column is a map, specific keys of that map can be addressed for the
// index.
func (db *ClientDBModel) SetIndexes(indexes map[string][]ClientIndex) {
	db.indexes = copyIndexes(indexes)
}

// Validate validates the DatabaseModel against the input schema
// Returns all the errors detected
func (db ClientDBModel) validate(schema ovsdb.DatabaseSchema) []error {
	var errors []error
	if db.name != schema.Name {
		errors = append(errors, fmt.Errorf("database model name (%s) does not match schema (%s)",
			db.name, schema.Name))
	}

	infos := make(map[string]*mapper.Info, len(db.types))
	for tableName := range db.types {
		tableSchema := schema.Table(tableName)
		if tableSchema == nil {
			errors = append(errors, fmt.Errorf("database model contains a model for table %s that does not exist in schema", tableName))
			continue
		}
		model, err := db.newModel(tableName)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		info, err := mapper.NewInfo(tableName, tableSchema, model)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		infos[tableName] = info
	}

	for tableName, indexSets := range db.indexes {
		info, ok := infos[tableName]
		if !ok {
			errors = append(errors, fmt.Errorf("database model contains a client index for table %s that does not exist in schema", tableName))
			continue
		}
		for _, indexSet := range indexSets {
			for _, indexColumn := range indexSet.Columns {
				f, err := info.FieldByColumn(indexColumn.Column)
				if err != nil {
					errors = append(
						errors,
						fmt.Errorf("database model contains a client index for column %s that does not exist in table %s",
							indexColumn.Column,
							tableName))
					continue
				}
				if indexColumn.Key != nil && reflect.ValueOf(f).Kind() != reflect.Map {
					errors = append(
						errors,
						fmt.Errorf("database model contains a client index for key %s in column %s of table %s that is not a map",
							indexColumn.Key,
							indexColumn.Column,
							tableName))
					continue
				}
			}
		}
	}
	return errors
}

// NewClientDBModel constructs a ClientDBModel based on a database name and dictionary of models indexed by table name
func NewClientDBModel(name string, models map[string]Model) (ClientDBModel, error) {
	types := make(map[string]reflect.Type, len(models))
	for table, model := range models {
		modelType := reflect.TypeOf(model)
		if modelType.Kind() != reflect.Ptr || modelType.Elem().Kind() != reflect.Struct {
			return ClientDBModel{}, fmt.Errorf("model is expected to be a pointer to struct")
		}
		hasUUID := false
		for i := 0; i < modelType.Elem().NumField(); i++ {
			if field := modelType.Elem().Field(i); field.Tag.Get("ovsdb") == "_uuid" &&
				field.Type.Kind() == reflect.String {
				hasUUID = true
				break
			}
		}
		if !hasUUID {
			return ClientDBModel{}, fmt.Errorf("model is expected to have a string field called uuid")
		}

		types[table] = modelType
	}
	return ClientDBModel{
		types: types,
		name:  name,
	}, nil
}

func copyIndexes(src map[string][]ClientIndex) map[string][]ClientIndex {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string][]ClientIndex, len(src))
	for table, indexSets := range src {
		dst[table] = make([]ClientIndex, 0, len(indexSets))
		for _, indexSet := range indexSets {
			indexSetCopy := ClientIndex{
				Columns: make([]ColumnKey, len(indexSet.Columns)),
			}
			copy(indexSetCopy.Columns, indexSet.Columns)
			dst[table] = append(dst[table], indexSetCopy)
		}
	}
	return dst
}
