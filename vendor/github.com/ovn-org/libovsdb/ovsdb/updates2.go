package ovsdb

// TableUpdates2 is an object that maps from a table name to a
// TableUpdate2
type TableUpdates2 map[string]TableUpdate2

// TableUpdate2 is an object that maps from the row's UUID to a
// RowUpdate2
type TableUpdate2 map[string]*RowUpdate2

// RowUpdate2 represents a row update according to ovsdb-server.7
type RowUpdate2 struct {
	Initial *Row `json:"initial,omitempty"`
	Insert  *Row `json:"insert,omitempty"`
	Modify  *Row `json:"modify,omitempty"`
	Delete  *Row `json:"delete,omitempty"`
	Old     *Row `json:"-"`
	New     *Row `json:"-"`
}
