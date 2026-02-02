package ovsdb

// TableUpdates is an object that maps from a table name to a
// TableUpdate
type TableUpdates map[string]TableUpdate

// TableUpdate is an object that maps from the row's UUID to a
// RowUpdate
type TableUpdate map[string]*RowUpdate

// RowUpdate represents a row update according to RFC7047
type RowUpdate struct {
	New *Row `json:"new,omitempty"`
	Old *Row `json:"old,omitempty"`
}

// Insert returns true if this is an update for an insert operation
func (r RowUpdate) Insert() bool {
	return r.New != nil && r.Old == nil
}

// Modify returns true if this is an update for a modify operation
func (r RowUpdate) Modify() bool {
	return r.New != nil && r.Old != nil
}

// Delete returns true if this is an update for a delete operation
func (r RowUpdate) Delete() bool {
	return r.New == nil && r.Old != nil
}

func (r *RowUpdate) FromRowUpdate2(ru2 RowUpdate2) {
	r.Old = ru2.Old
	r.New = ru2.New
}
