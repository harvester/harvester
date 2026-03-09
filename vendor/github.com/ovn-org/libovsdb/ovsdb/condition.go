package ovsdb

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type ConditionFunction string
type WaitCondition string

const (
	// ConditionLessThan is the less than condition
	ConditionLessThan ConditionFunction = "<"
	// ConditionLessThanOrEqual is the less than or equal condition
	ConditionLessThanOrEqual ConditionFunction = "<="
	// ConditionEqual is the equal condition
	ConditionEqual ConditionFunction = "=="
	// ConditionNotEqual is the not equal condition
	ConditionNotEqual ConditionFunction = "!="
	// ConditionGreaterThan is the greater than condition
	ConditionGreaterThan ConditionFunction = ">"
	// ConditionGreaterThanOrEqual is the greater than or equal condition
	ConditionGreaterThanOrEqual ConditionFunction = ">="
	// ConditionIncludes is the includes condition
	ConditionIncludes ConditionFunction = "includes"
	// ConditionExcludes is the excludes condition
	ConditionExcludes ConditionFunction = "excludes"

	// WaitConditionEqual is the equal condition
	WaitConditionEqual WaitCondition = "=="
	// WaitConditionNotEqual is the not equal condition
	WaitConditionNotEqual WaitCondition = "!="
)

// Condition is described in RFC 7047: 5.1
type Condition struct {
	Column   string
	Function ConditionFunction
	Value    interface{}
}

func (c Condition) String() string {
	return fmt.Sprintf("where column %s %s %v", c.Column, c.Function, c.Value)
}

// NewCondition returns a new condition
func NewCondition(column string, function ConditionFunction, value interface{}) Condition {
	return Condition{
		Column:   column,
		Function: function,
		Value:    value,
	}
}

// MarshalJSON marshals a condition to a 3 element JSON array
func (c Condition) MarshalJSON() ([]byte, error) {
	v := []interface{}{c.Column, c.Function, c.Value}
	return json.Marshal(v)
}

// UnmarshalJSON converts a 3 element JSON array to a Condition
func (c *Condition) UnmarshalJSON(b []byte) error {
	var v []interface{}
	err := json.Unmarshal(b, &v)
	if err != nil {
		return err
	}
	if len(v) != 3 {
		return fmt.Errorf("expected a 3 element json array. there are %d elements", len(v))
	}
	c.Column = v[0].(string)
	function := ConditionFunction(v[1].(string))
	switch function {
	case ConditionEqual,
		ConditionNotEqual,
		ConditionIncludes,
		ConditionExcludes,
		ConditionGreaterThan,
		ConditionGreaterThanOrEqual,
		ConditionLessThan,
		ConditionLessThanOrEqual:
		c.Function = function
	default:
		return fmt.Errorf("%s is not a valid function", function)
	}
	vv, err := ovsSliceToGoNotation(v[2])
	if err != nil {
		return err
	}
	c.Value = vv
	return nil
}

// Evaluate will evaluate the condition on the two provided values
// The conditions operately differently depending on the type of
// the provided values. The behavior is as described in RFC7047
func (c ConditionFunction) Evaluate(a interface{}, b interface{}) (bool, error) {
	x := reflect.ValueOf(a)
	y := reflect.ValueOf(b)
	if x.Kind() != y.Kind() {
		return false, fmt.Errorf("comparison between %s and %s not supported", x.Kind(), y.Kind())
	}
	switch c {
	case ConditionEqual:
		return reflect.DeepEqual(a, b), nil
	case ConditionNotEqual:
		return !reflect.DeepEqual(a, b), nil
	case ConditionIncludes:
		switch x.Kind() {
		case reflect.Slice:
			return sliceContains(x, y), nil
		case reflect.Map:
			return mapContains(x, y), nil
		case reflect.Int, reflect.Float64, reflect.Bool, reflect.String:
			return reflect.DeepEqual(a, b), nil
		default:
			return false, fmt.Errorf("condition not supported on %s", x.Kind())
		}
	case ConditionExcludes:
		switch x.Kind() {
		case reflect.Slice:
			return !sliceContains(x, y), nil
		case reflect.Map:
			return !mapContains(x, y), nil
		case reflect.Int, reflect.Float64, reflect.Bool, reflect.String:
			return !reflect.DeepEqual(a, b), nil
		default:
			return false, fmt.Errorf("condition not supported on %s", x.Kind())
		}
	case ConditionGreaterThan:
		switch x.Kind() {
		case reflect.Int:
			return x.Int() > y.Int(), nil
		case reflect.Float64:
			return x.Float() > y.Float(), nil
		case reflect.Bool, reflect.String, reflect.Slice, reflect.Map:
		default:
			return false, fmt.Errorf("condition not supported on %s", x.Kind())
		}
	case ConditionGreaterThanOrEqual:
		switch x.Kind() {
		case reflect.Int:
			return x.Int() >= y.Int(), nil
		case reflect.Float64:
			return x.Float() >= y.Float(), nil
		case reflect.Bool, reflect.String, reflect.Slice, reflect.Map:
		default:
			return false, fmt.Errorf("condition not supported on %s", x.Kind())
		}
	case ConditionLessThan:
		switch x.Kind() {
		case reflect.Int:
			return x.Int() < y.Int(), nil
		case reflect.Float64:
			return x.Float() < y.Float(), nil
		case reflect.Bool, reflect.String, reflect.Slice, reflect.Map:
		default:
			return false, fmt.Errorf("condition not supported on %s", x.Kind())
		}
	case ConditionLessThanOrEqual:
		switch x.Kind() {
		case reflect.Int:
			return x.Int() <= y.Int(), nil
		case reflect.Float64:
			return x.Float() <= y.Float(), nil
		case reflect.Bool, reflect.String, reflect.Slice, reflect.Map:
		default:
			return false, fmt.Errorf("condition not supported on %s", x.Kind())
		}
	default:
		return false, fmt.Errorf("unsupported condition function %s", c)
	}
	// we should never get here
	return false, fmt.Errorf("unreachable condition")
}

func sliceContains(x, y reflect.Value) bool {
	for i := 0; i < y.Len(); i++ {
		found := false
		vy := y.Index(i)
		for j := 0; j < x.Len(); j++ {
			vx := x.Index(j)
			if vy.Kind() == reflect.Interface {
				if vy.Elem() == vx.Elem() {
					found = true
					break
				}
			} else {
				if vy.Interface() == vx.Interface() {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func mapContains(x, y reflect.Value) bool {
	iter := y.MapRange()
	for iter.Next() {
		k := iter.Key()
		v := iter.Value()
		vx := x.MapIndex(k)
		if !vx.IsValid() {
			return false
		}
		if v.Kind() != reflect.Interface {
			if v.Interface() != vx.Interface() {
				return false
			}
		} else {
			if v.Elem() != vx.Elem() {
				return false
			}
		}
	}
	return true
}
