package ovsdb

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// OvsSet is an OVSDB style set
// RFC 7047 has a weird (but understandable) notation for set as described as :
// Either an <atom>, representing a set with exactly one element, or
// a 2-element JSON array that represents a database set value.  The
// first element of the array must be the string "set", and the
// second element must be an array of zero or more <atom>s giving the
// values in the set.  All of the <atom>s must have the same type.
type OvsSet struct {
	GoSet []interface{}
}

// NewOvsSet creates a new OVSDB style set from a Go interface (object)
func NewOvsSet(obj interface{}) (OvsSet, error) {
	ovsSet := make([]interface{}, 0)
	var v reflect.Value
	if reflect.TypeOf(obj).Kind() == reflect.Ptr {
		v = reflect.ValueOf(obj).Elem()
		if v.Kind() == reflect.Invalid {
			// must be a nil pointer, so just return an empty set
			return OvsSet{ovsSet}, nil
		}
	} else {
		v = reflect.ValueOf(obj)
	}

	switch v.Kind() {
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			ovsSet = append(ovsSet, v.Index(i).Interface())
		}
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.Bool:
		ovsSet = append(ovsSet, v.Interface())
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(UUID{}) {
			ovsSet = append(ovsSet, v.Interface())
		} else {
			return OvsSet{}, fmt.Errorf("ovsset supports only go slice/string/numbers/uuid or pointers to those types")
		}
	default:
		return OvsSet{}, fmt.Errorf("ovsset supports only go slice/string/numbers/uuid or pointers to those types")
	}
	return OvsSet{ovsSet}, nil
}

// MarshalJSON wil marshal an OVSDB style Set in to a JSON byte array
func (o OvsSet) MarshalJSON() ([]byte, error) {
	switch l := len(o.GoSet); {
	case l == 1:
		return json.Marshal(o.GoSet[0])
	case l > 0:
		var oSet []interface{}
		oSet = append(oSet, "set")
		oSet = append(oSet, o.GoSet)
		return json.Marshal(oSet)
	}
	return []byte("[\"set\",[]]"), nil
}

// UnmarshalJSON will unmarshal a JSON byte array to an OVSDB style Set
func (o *OvsSet) UnmarshalJSON(b []byte) (err error) {
	o.GoSet = make([]interface{}, 0)
	addToSet := func(o *OvsSet, v interface{}) error {
		goVal, err := ovsSliceToGoNotation(v)
		if err == nil {
			o.GoSet = append(o.GoSet, goVal)
		}
		return err
	}

	var inter interface{}
	if err = json.Unmarshal(b, &inter); err != nil {
		return err
	}
	switch inter.(type) {
	case []interface{}:
		var oSet []interface{}
		oSet = inter.([]interface{})
		// it's a single uuid object
		if len(oSet) == 2 && (oSet[0] == "uuid" || oSet[0] == "named-uuid") {
			return addToSet(o, UUID{GoUUID: oSet[1].(string)})
		}
		if oSet[0] != "set" {
			// it is a slice, but is not a set
			return &json.UnmarshalTypeError{Value: reflect.ValueOf(inter).String(), Type: reflect.TypeOf(*o)}
		}
		innerSet := oSet[1].([]interface{})
		for _, val := range innerSet {
			err := addToSet(o, val)
			if err != nil {
				return err
			}
		}
		return err
	default:
		// it is a single object
		return addToSet(o, inter)
	}
}
