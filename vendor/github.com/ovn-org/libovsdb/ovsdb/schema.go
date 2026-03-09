package ovsdb

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"strings"
)

// DatabaseSchema is a database schema according to RFC7047
type DatabaseSchema struct {
	Name          string                 `json:"name"`
	Version       string                 `json:"version"`
	Tables        map[string]TableSchema `json:"tables"`
	allTablesRoot *bool
}

// UUIDColumn is a static column that represents the _uuid column, common to all tables
var UUIDColumn = ColumnSchema{
	Type: TypeUUID,
}

// Table returns a TableSchema Schema for a given table and column name
func (schema DatabaseSchema) Table(tableName string) *TableSchema {
	if table, ok := schema.Tables[tableName]; ok {
		return &table
	}
	return nil
}

// IsRoot whether a table is root or not
func (schema DatabaseSchema) IsRoot(tableName string) (bool, error) {
	t := schema.Table(tableName)
	if t == nil {
		return false, fmt.Errorf("Table %s not in schame", tableName)
	}
	if t.IsRoot {
		return true, nil
	}
	// As per RFC7047, for compatibility with schemas created before
	// "isRoot" was introduced, if "isRoot" is omitted or false in every
	// <table-schema> in a given <database-schema>, then every table is part
	// of the root set.
	if schema.allTablesRoot == nil {
		allTablesRoot := true
		for _, tSchema := range schema.Tables {
			if tSchema.IsRoot {
				allTablesRoot = false
				break
			}
		}
		schema.allTablesRoot = &allTablesRoot
	}
	return *schema.allTablesRoot, nil
}

// Print will print the contents of the DatabaseSchema
func (schema DatabaseSchema) Print(w io.Writer) {
	fmt.Fprintf(w, "%s, (%s)\n", schema.Name, schema.Version)
	for table, tableSchema := range schema.Tables {
		fmt.Fprintf(w, "\t %s", table)
		if len(tableSchema.Indexes) > 0 {
			fmt.Fprintf(w, "(%v)\n", tableSchema.Indexes)
		} else {
			fmt.Fprintf(w, "\n")
		}
		for column, columnSchema := range tableSchema.Columns {
			fmt.Fprintf(w, "\t\t %s => %s\n", column, columnSchema)
		}
	}
}

// SchemaFromFile returns a DatabaseSchema from a file
func SchemaFromFile(f *os.File) (DatabaseSchema, error) {
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return DatabaseSchema{}, err
	}
	var schema DatabaseSchema
	err = json.Unmarshal(data, &schema)
	if err != nil {
		return DatabaseSchema{}, err
	}
	return schema, nil
}

// ValidateOperations performs basic validation for operations against a DatabaseSchema
func (schema DatabaseSchema) ValidateOperations(operations ...Operation) bool {
	for _, op := range operations {
		switch op.Op {
		case OperationAbort, OperationAssert, OperationComment, OperationCommit, OperationWait:
			continue
		case OperationInsert, OperationSelect, OperationUpdate, OperationMutate, OperationDelete:
			table, ok := schema.Tables[op.Table]
			if ok {
				for column := range op.Row {
					if _, ok := table.Columns[column]; !ok {
						if column != "_uuid" && column != "_version" {
							return false
						}
					}
				}
				for _, row := range op.Rows {
					for column := range row {
						if _, ok := table.Columns[column]; !ok {
							if column != "_uuid" && column != "_version" {
								return false
							}
						}
					}
				}
				for _, column := range op.Columns {
					if _, ok := table.Columns[column]; !ok {
						if column != "_uuid" && column != "_version" {
							return false
						}
					}
				}
			} else {
				return false
			}
		}
	}
	return true
}

// TableSchema is a table schema according to RFC7047
type TableSchema struct {
	Columns map[string]*ColumnSchema `json:"columns"`
	Indexes [][]string               `json:"indexes,omitempty"`
	IsRoot  bool                     `json:"isRoot,omitempty"`
}

// Column returns the Column object for a specific column name
func (t TableSchema) Column(columnName string) *ColumnSchema {
	if columnName == "_uuid" {
		return &UUIDColumn
	}
	if column, ok := t.Columns[columnName]; ok {
		return column
	}
	return nil
}

/*RFC7047 defines some atomic-types (e.g: integer, string, etc). However, the Column's type
can also hold other more complex types such as set, enum and map. The way to determine the type
depends on internal, not directly marshallable fields. Therefore, in order to simplify the usage
of this library, we define an ExtendedType that includes all possible column types (including
atomic fields).
*/

// ExtendedType includes atomic types as defined in the RFC plus Enum, Map and Set
type ExtendedType = string

// RefType is used to define the possible RefTypes
type RefType = string

// unlimited is not constant as we can't take the address of int constants
var (
	// Unlimited is used to express unlimited "Max"
	Unlimited = -1
)

const (
	unlimitedString = "unlimited"
	//Strong RefType
	Strong RefType = "strong"
	//Weak RefType
	Weak RefType = "weak"

	//ExtendedType associated with Atomic Types

	//TypeInteger is equivalent to 'int'
	TypeInteger ExtendedType = "integer"
	//TypeReal is equivalent to 'float64'
	TypeReal ExtendedType = "real"
	//TypeBoolean is equivalent to 'bool'
	TypeBoolean ExtendedType = "boolean"
	//TypeString is equivalent to 'string'
	TypeString ExtendedType = "string"
	//TypeUUID is equivalent to 'libovsdb.UUID'
	TypeUUID ExtendedType = "uuid"

	//Extended Types used to summarize the internal type of the field.

	//TypeEnum is an enumerator of type defined by Key.Type
	TypeEnum ExtendedType = "enum"
	//TypeMap is a map whose type depend on Key.Type and Value.Type
	TypeMap ExtendedType = "map"
	//TypeSet is a set whose type depend on Key.Type
	TypeSet ExtendedType = "set"
)

// BaseType is a base-type structure as per RFC7047
type BaseType struct {
	Type       string
	Enum       []interface{}
	minReal    *float64
	maxReal    *float64
	minInteger *int
	maxInteger *int
	minLength  *int
	maxLength  *int
	refTable   *string
	refType    *RefType
}

func (b *BaseType) simpleAtomic() bool {
	return isAtomicType(b.Type) && b.Enum == nil && b.minReal == nil && b.maxReal == nil && b.minInteger == nil && b.maxInteger == nil && b.minLength == nil && b.maxLength == nil && b.refTable == nil && b.refType == nil
}

// MinReal returns the minimum real value
// RFC7047 does not define a default, but we assume this to be
// the smallest non zero value a float64 could hold
func (b *BaseType) MinReal() (float64, error) {
	if b.Type != TypeReal {
		return 0, fmt.Errorf("%s is not a real", b.Type)
	}
	if b.minReal != nil {
		return *b.minReal, nil
	}
	return math.SmallestNonzeroFloat64, nil
}

// MaxReal returns the maximum real value
// RFC7047 does not define a default, but this would be the maximum
// value held by a float64
func (b *BaseType) MaxReal() (float64, error) {
	if b.Type != TypeReal {
		return 0, fmt.Errorf("%s is not a real", b.Type)
	}
	if b.maxReal != nil {
		return *b.maxReal, nil
	}
	return math.MaxFloat64, nil
}

// MinInteger returns the minimum integer value
// RFC7047 specifies the minimum to be -2^63
func (b *BaseType) MinInteger() (int, error) {
	if b.Type != TypeInteger {
		return 0, fmt.Errorf("%s is not an integer", b.Type)
	}
	if b.minInteger != nil {
		return *b.minInteger, nil
	}
	return int(math.Pow(-2, 63)), nil
}

// MaxInteger returns the minimum integer value
// RFC7047 specifies the minimum to be 2^63-1
func (b *BaseType) MaxInteger() (int, error) {
	if b.Type != TypeInteger {
		return 0, fmt.Errorf("%s is not an integer", b.Type)
	}
	if b.maxInteger != nil {
		return *b.maxInteger, nil
	}
	return int(math.Pow(2, 63)) - 1, nil
}

// MinLength returns the minimum string length
// RFC7047 doesn't specify a default, but we assume
// that it must be >= 0
func (b *BaseType) MinLength() (int, error) {
	if b.Type != TypeString {
		return 0, fmt.Errorf("%s is not an string", b.Type)
	}
	if b.minLength != nil {
		return *b.minLength, nil
	}
	return 0, nil
}

// MaxLength returns the maximum string length
// RFC7047 doesn't specify a default, but we assume
// that it must 2^63-1
func (b *BaseType) MaxLength() (int, error) {
	if b.Type != TypeString {
		return 0, fmt.Errorf("%s is not an string", b.Type)
	}
	if b.maxLength != nil {
		return *b.maxLength, nil
	}
	return int(math.Pow(2, 63)) - 1, nil
}

// RefTable returns the table to which a UUID type refers
// It will return an empty string if not set
func (b *BaseType) RefTable() (string, error) {
	if b.Type != TypeUUID {
		return "", fmt.Errorf("%s is not a uuid", b.Type)
	}
	if b.refTable != nil {
		return *b.refTable, nil
	}
	return "", nil
}

// RefType returns the reference type for a UUID field
// RFC7047 infers the RefType is strong if omitted
func (b *BaseType) RefType() (RefType, error) {
	if b.Type != TypeUUID {
		return "", fmt.Errorf("%s is not a uuid", b.Type)
	}
	if b.refType != nil {
		return *b.refType, nil
	}
	return Strong, nil
}

// UnmarshalJSON unmarshals a json-formatted base type
func (b *BaseType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if isAtomicType(s) {
			b.Type = s
		} else {
			return fmt.Errorf("non atomic type %s in <base-type>", s)
		}
		return nil
	}
	// temporary type to avoid recursive call to unmarshal
	var bt struct {
		Type       string      `json:"type"`
		Enum       interface{} `json:"enum,omitempty"`
		MinReal    *float64    `json:"minReal,omitempty"`
		MaxReal    *float64    `json:"maxReal,omitempty"`
		MinInteger *int        `json:"minInteger,omitempty"`
		MaxInteger *int        `json:"maxInteger,omitempty"`
		MinLength  *int        `json:"minLength,omitempty"`
		MaxLength  *int        `json:"maxLength,omitempty"`
		RefTable   *string     `json:"refTable,omitempty"`
		RefType    *RefType    `json:"refType,omitempty"`
	}
	err := json.Unmarshal(data, &bt)
	if err != nil {
		return err
	}

	if bt.Enum != nil {
		// 'enum' is a list or a single element representing a list of exactly one element
		switch bt.Enum.(type) {
		case []interface{}:
			// it's an OvsSet
			oSet := bt.Enum.([]interface{})
			innerSet := oSet[1].([]interface{})
			b.Enum = make([]interface{}, len(innerSet))
			copy(b.Enum, innerSet)
		default:
			b.Enum = []interface{}{bt.Enum}
		}
	}
	b.Type = bt.Type
	b.minReal = bt.MinReal
	b.maxReal = bt.MaxReal
	b.minInteger = bt.MinInteger
	b.maxInteger = bt.MaxInteger
	b.minLength = bt.MaxLength
	b.maxLength = bt.MaxLength
	b.refTable = bt.RefTable
	b.refType = bt.RefType
	return nil
}

// MarshalJSON marshals a base type to JSON
func (b BaseType) MarshalJSON() ([]byte, error) {
	j := struct {
		Type       string   `json:"type,omitempty"`
		Enum       *OvsSet  `json:"enum,omitempty"`
		MinReal    *float64 `json:"minReal,omitempty"`
		MaxReal    *float64 `json:"maxReal,omitempty"`
		MinInteger *int     `json:"minInteger,omitempty"`
		MaxInteger *int     `json:"maxInteger,omitempty"`
		MinLength  *int     `json:"minLength,omitempty"`
		MaxLength  *int     `json:"maxLength,omitempty"`
		RefTable   *string  `json:"refTable,omitempty"`
		RefType    *RefType `json:"refType,omitempty"`
	}{
		Type:       b.Type,
		MinReal:    b.minReal,
		MaxReal:    b.maxReal,
		MinInteger: b.minInteger,
		MaxInteger: b.maxInteger,
		MinLength:  b.maxLength,
		MaxLength:  b.maxLength,
		RefTable:   b.refTable,
		RefType:    b.refType,
	}
	if len(b.Enum) > 0 {
		set, err := NewOvsSet(b.Enum)
		if err != nil {
			return nil, err
		}
		j.Enum = &set
	}
	return json.Marshal(j)
}

// ColumnType is a type object as per RFC7047
// "key": <base-type>                 required
// "value": <base-type>               optional
// "min": <integer>                   optional (default: 1)
// "max": <integer> or "unlimited"    optional (default: 1)
type ColumnType struct {
	Key   *BaseType
	Value *BaseType
	min   *int
	max   *int
}

// Max returns the maximum value of a ColumnType. -1 is Unlimited
func (c *ColumnType) Max() int {
	if c.max == nil {
		return 1
	}
	return *c.max
}

// Min returns the minimum value of a ColumnType
func (c *ColumnType) Min() int {
	if c.min == nil {
		return 1
	}
	return *c.min
}

// UnmarshalJSON unmarshals a json-formatted column type
func (c *ColumnType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if isAtomicType(s) {
			c.Key = &BaseType{Type: s}
		} else {
			return fmt.Errorf("non atomic type %s in <type>", s)
		}
		return nil
	}
	var colType struct {
		Key   *BaseType   `json:"key"`
		Value *BaseType   `json:"value"`
		Min   *int        `json:"min"`
		Max   interface{} `json:"max"`
	}
	err := json.Unmarshal(data, &colType)
	if err != nil {
		return err
	}
	c.Key = colType.Key
	c.Value = colType.Value
	c.min = colType.Min
	switch v := colType.Max.(type) {
	case string:
		if v == unlimitedString {
			c.max = &Unlimited
		} else {
			return fmt.Errorf("unexpected string value in max field")
		}
	case float64:
		i := int(v)
		c.max = &i
	default:
		c.max = nil
	}
	return nil
}

// MarshalJSON marshalls a column type to JSON
func (c ColumnType) MarshalJSON() ([]byte, error) {
	if c.Value == nil && c.max == nil && c.min == nil && c.Key.simpleAtomic() {
		return json.Marshal(c.Key.Type)
	}
	if c.Max() == Unlimited {
		colType := struct {
			Key   *BaseType `json:"key"`
			Value *BaseType `json:"value,omitempty"`
			Min   *int      `json:"min,omitempty"`
			Max   string    `json:"max,omitempty"`
		}{
			Key:   c.Key,
			Value: c.Value,
			Min:   c.min,
			Max:   unlimitedString,
		}
		return json.Marshal(&colType)
	}
	colType := struct {
		Key   *BaseType `json:"key"`
		Value *BaseType `json:"value,omitempty"`
		Min   *int      `json:"min,omitempty"`
		Max   *int      `json:"max,omitempty"`
	}{
		Key:   c.Key,
		Value: c.Value,
		Min:   c.min,
		Max:   c.max,
	}
	return json.Marshal(&colType)
}

// ColumnSchema is a column schema according to RFC7047
type ColumnSchema struct {
	// According to RFC7047, "type" field can be, either an <atomic-type>
	// Or a ColumnType defined below. To try to simplify the usage, the
	// json message will be parsed manually and Type will indicate the "extended"
	// type. Depending on its value, more information may be available in TypeObj.
	// E.g: If Type == TypeEnum, TypeObj.Key.Enum contains the possible values
	Type      ExtendedType
	TypeObj   *ColumnType
	ephemeral *bool
	mutable   *bool
}

// Mutable returns whether a column is mutable
func (c *ColumnSchema) Mutable() bool {
	if c.mutable != nil {
		return *c.mutable
	}
	// default true
	return true
}

// Ephemeral returns whether a column is ephemeral
func (c *ColumnSchema) Ephemeral() bool {
	if c.ephemeral != nil {
		return *c.ephemeral
	}
	// default false
	return false
}

// UnmarshalJSON unmarshals a json-formatted column
func (c *ColumnSchema) UnmarshalJSON(data []byte) error {
	// ColumnJSON represents the known json values for a Column
	var colJSON struct {
		Type      *ColumnType `json:"type"`
		Ephemeral *bool       `json:"ephemeral,omitempty"`
		Mutable   *bool       `json:"mutable,omitempty"`
	}

	// Unmarshal known keys
	if err := json.Unmarshal(data, &colJSON); err != nil {
		return fmt.Errorf("cannot parse column object %s", err)
	}

	c.ephemeral = colJSON.Ephemeral
	c.mutable = colJSON.Mutable
	c.TypeObj = colJSON.Type

	// Infer the ExtendedType from the TypeObj
	if c.TypeObj.Value != nil {
		c.Type = TypeMap
	} else if c.TypeObj.Min() != 1 || c.TypeObj.Max() != 1 {
		c.Type = TypeSet
	} else if len(c.TypeObj.Key.Enum) > 0 {
		c.Type = TypeEnum
	} else {
		c.Type = c.TypeObj.Key.Type
	}
	return nil
}

// MarshalJSON marshalls a column schema to JSON
func (c ColumnSchema) MarshalJSON() ([]byte, error) {
	type colJSON struct {
		Type      *ColumnType `json:"type"`
		Ephemeral *bool       `json:"ephemeral,omitempty"`
		Mutable   *bool       `json:"mutable,omitempty"`
	}
	column := colJSON{
		Type:      c.TypeObj,
		Ephemeral: c.ephemeral,
		Mutable:   c.mutable,
	}
	return json.Marshal(column)
}

// String returns a string representation of the (native) column type
func (c *ColumnSchema) String() string {
	var flags []string
	var flagStr string
	var typeStr string
	if c.Ephemeral() {
		flags = append(flags, "E")
	}
	if c.Mutable() {
		flags = append(flags, "M")
	}
	if len(flags) > 0 {
		flagStr = fmt.Sprintf("[%s]", strings.Join(flags, ","))
	}

	switch c.Type {
	case TypeInteger, TypeReal, TypeBoolean, TypeString:
		typeStr = string(c.Type)
	case TypeUUID:
		if c.TypeObj != nil && c.TypeObj.Key != nil {
			// ignore err as we've already asserted this is a uuid
			reftable, _ := c.TypeObj.Key.RefTable()
			reftype := ""
			if s, err := c.TypeObj.Key.RefType(); err != nil {
				reftype = s
			}
			typeStr = fmt.Sprintf("uuid [%s (%s)]", reftable, reftype)
		} else {
			typeStr = "uuid"
		}

	case TypeEnum:
		typeStr = fmt.Sprintf("enum (type: %s): %v", c.TypeObj.Key.Type, c.TypeObj.Key.Enum)
	case TypeMap:
		typeStr = fmt.Sprintf("[%s]%s", c.TypeObj.Key.Type, c.TypeObj.Value.Type)
	case TypeSet:
		var keyStr string
		if c.TypeObj.Key.Type == TypeUUID {
			// ignore err as we've already asserted this is a uuid
			reftable, _ := c.TypeObj.Key.RefTable()
			reftype, _ := c.TypeObj.Key.RefType()
			keyStr = fmt.Sprintf(" [%s (%s)]", reftable, reftype)
		} else {
			keyStr = string(c.TypeObj.Key.Type)
		}
		typeStr = fmt.Sprintf("[]%s (min: %d, max: %d)", keyStr, c.TypeObj.Min(), c.TypeObj.Max())
	default:
		panic(fmt.Sprintf("Unsupported type %s", c.Type))
	}

	return strings.Join([]string{typeStr, flagStr}, " ")
}

func isAtomicType(atype string) bool {
	switch atype {
	case TypeInteger, TypeReal, TypeBoolean, TypeString, TypeUUID:
		return true
	default:
		return false
	}
}
