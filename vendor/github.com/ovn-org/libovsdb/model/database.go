package model

import (
	"fmt"
	"reflect"

	"github.com/ovn-org/libovsdb/mapper"
	"github.com/ovn-org/libovsdb/ovsdb"
)

// A DatabaseModel represents libovsdb's metadata about the database.
// It's the result of combining the client's ClientDBModel and the server's Schema
type DatabaseModel struct {
	client   ClientDBModel
	Schema   ovsdb.DatabaseSchema
	Mapper   mapper.Mapper
	metadata map[reflect.Type]mapper.Metadata
}

// NewDatabaseModel returns a new DatabaseModel
func NewDatabaseModel(schema ovsdb.DatabaseSchema, client ClientDBModel) (DatabaseModel, []error) {
	dbModel := &DatabaseModel{
		Schema: schema,
		client: client,
	}
	errs := client.validate(schema)
	if len(errs) > 0 {
		return DatabaseModel{}, errs
	}
	dbModel.Mapper = mapper.NewMapper(schema)
	var metadata map[reflect.Type]mapper.Metadata
	metadata, errs = generateModelInfo(schema, client.types)
	if len(errs) > 0 {
		return DatabaseModel{}, errs
	}
	dbModel.metadata = metadata
	return *dbModel, nil
}

// NewPartialDatabaseModel returns a DatabaseModel what does not have a schema yet
func NewPartialDatabaseModel(client ClientDBModel) DatabaseModel {
	return DatabaseModel{
		client: client,
	}
}

// Valid returns whether the DatabaseModel is fully functional
func (db DatabaseModel) Valid() bool {
	return !reflect.DeepEqual(db.Schema, ovsdb.DatabaseSchema{})
}

// Client returns the DatabaseModel's client dbModel
func (db DatabaseModel) Client() ClientDBModel {
	return db.client
}

// NewModel returns a new instance of a model from a specific string
func (db DatabaseModel) NewModel(table string) (Model, error) {
	mtype, ok := db.client.types[table]
	if !ok {
		return nil, fmt.Errorf("table %s not found in database model", string(table))
	}
	model := reflect.New(mtype.Elem())
	return model.Interface().(Model), nil
}

// Types returns the DatabaseModel Types
// the DatabaseModel types is a map of reflect.Types indexed by string
// The reflect.Type is a pointer to a struct that contains 'ovs' tags
// as described above. Such pointer to struct also implements the Model interface
func (db DatabaseModel) Types() map[string]reflect.Type {
	return db.client.types
}

// FindTable returns the string associated with a reflect.Type or ""
func (db DatabaseModel) FindTable(mType reflect.Type) string {
	for table, tType := range db.client.types {
		if tType == mType {
			return table
		}
	}
	return ""
}

// generateModelMetadata creates metadata objects from all models included in the
// database and caches them for future re-use
func generateModelInfo(dbSchema ovsdb.DatabaseSchema, modelTypes map[string]reflect.Type) (map[reflect.Type]mapper.Metadata, []error) {
	errors := []error{}
	metadata := make(map[reflect.Type]mapper.Metadata, len(modelTypes))
	for tableName, tType := range modelTypes {
		tableSchema := dbSchema.Table(tableName)
		if tableSchema == nil {
			errors = append(errors, fmt.Errorf("database Model contains model for table %s which is not present in schema", tableName))
			continue
		}

		obj := reflect.New(tType.Elem()).Interface().(Model)
		info, err := mapper.NewInfo(tableName, tableSchema, obj)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		metadata[tType] = info.Metadata
	}
	return metadata, errors
}

// NewModelInfo returns a mapper.Info object based on a provided model
func (db DatabaseModel) NewModelInfo(obj interface{}) (*mapper.Info, error) {
	meta, ok := db.metadata[reflect.TypeOf(obj)]
	if !ok {
		return nil, ovsdb.NewErrWrongType("NewModelInfo", "type that is part of the DatabaseModel", obj)
	}
	return &mapper.Info{
		Obj:      obj,
		Metadata: meta,
	}, nil
}
