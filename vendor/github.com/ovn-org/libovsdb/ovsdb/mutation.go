package ovsdb

import (
	"encoding/json"
	"fmt"
)

type Mutator string

const (
	// MutateOperationDelete is the delete mutator
	MutateOperationDelete Mutator = "delete"
	// MutateOperationInsert is the insert mutator
	MutateOperationInsert Mutator = "insert"
	// MutateOperationAdd is the add mutator
	MutateOperationAdd Mutator = "+="
	// MutateOperationSubtract is the subtract mutator
	MutateOperationSubtract Mutator = "-="
	// MutateOperationMultiply is the multiply mutator
	MutateOperationMultiply Mutator = "*="
	// MutateOperationDivide is the divide mutator
	MutateOperationDivide Mutator = "/="
	// MutateOperationModulo is the modulo mutator
	MutateOperationModulo Mutator = "%="
)

// Mutation is described in RFC 7047: 5.1
type Mutation struct {
	Column  string
	Mutator Mutator
	Value   interface{}
}

// NewMutation returns a new mutation
func NewMutation(column string, mutator Mutator, value interface{}) *Mutation {
	return &Mutation{
		Column:  column,
		Mutator: mutator,
		Value:   value,
	}
}

// MarshalJSON marshals a mutation to a 3 element JSON array
func (m Mutation) MarshalJSON() ([]byte, error) {
	v := []interface{}{m.Column, m.Mutator, m.Value}
	return json.Marshal(v)
}

// UnmarshalJSON converts a 3 element JSON array to a Mutation
func (m *Mutation) UnmarshalJSON(b []byte) error {
	var v []interface{}
	err := json.Unmarshal(b, &v)
	if err != nil {
		return err
	}
	if len(v) != 3 {
		return fmt.Errorf("expected a 3 element json array. there are %d elements", len(v))
	}
	ok := false
	m.Column, ok = v[0].(string)
	if !ok {
		return fmt.Errorf("expected column name %v to be a valid string", v[0])
	}
	mutatorString, ok := v[1].(string)
	if !ok {
		return fmt.Errorf("expected mutator %v to be a valid string", v[1])
	}
	mutator := Mutator(mutatorString)
	switch mutator {
	case MutateOperationDelete,
		MutateOperationInsert,
		MutateOperationAdd,
		MutateOperationSubtract,
		MutateOperationMultiply,
		MutateOperationDivide,
		MutateOperationModulo:
		m.Mutator = mutator
	default:
		return fmt.Errorf("%s is not a valid mutator", mutator)
	}
	vv, err := ovsSliceToGoNotation(v[2])
	if err != nil {
		return err
	}
	m.Value = vv
	return nil
}
