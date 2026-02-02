package ovsdb

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// OvsMap is the JSON map structure used for OVSDB
// RFC 7047 uses the following notation for map as JSON doesn't support non-string keys for maps.
// A 2-element JSON array that represents a database map value.  The
// first element of the array must be the string "map", and the
// second element must be an array of zero or more <pair>s giving the
// values in the map.  All of the <pair>s must have the same key and
// value types.
type OvsMap struct {
	GoMap map[interface{}]interface{}
}

// MarshalJSON marshalls an OVSDB style Map to a byte array
func (o OvsMap) MarshalJSON() ([]byte, error) {
	if len(o.GoMap) > 0 {
		var ovsMap, innerMap []interface{}
		ovsMap = append(ovsMap, "map")
		for key, val := range o.GoMap {
			var mapSeg []interface{}
			mapSeg = append(mapSeg, key)
			mapSeg = append(mapSeg, val)
			innerMap = append(innerMap, mapSeg)
		}
		ovsMap = append(ovsMap, innerMap)
		return json.Marshal(ovsMap)
	}
	return []byte("[\"map\",[]]"), nil
}

// UnmarshalJSON unmarshals an OVSDB style Map from a byte array
func (o *OvsMap) UnmarshalJSON(b []byte) (err error) {
	var oMap []interface{}
	o.GoMap = make(map[interface{}]interface{})
	if err := json.Unmarshal(b, &oMap); err == nil && len(oMap) > 1 {
		innerSlice := oMap[1].([]interface{})
		for _, val := range innerSlice {
			f := val.([]interface{})
			var k interface{}
			switch f[0].(type) {
			case []interface{}:
				vSet := f[0].([]interface{})
				if len(vSet) != 2 || vSet[0] == "map" {
					return &json.UnmarshalTypeError{Value: reflect.ValueOf(oMap).String(), Type: reflect.TypeOf(*o)}
				}
				goSlice, err := ovsSliceToGoNotation(vSet)
				if err != nil {
					return err
				}
				k = goSlice
			default:
				k = f[0]
			}
			switch f[1].(type) {
			case []interface{}:
				vSet := f[1].([]interface{})
				if len(vSet) != 2 || vSet[0] == "map" {
					return &json.UnmarshalTypeError{Value: reflect.ValueOf(oMap).String(), Type: reflect.TypeOf(*o)}
				}
				goSlice, err := ovsSliceToGoNotation(vSet)
				if err != nil {
					return err
				}
				o.GoMap[k] = goSlice
			default:
				o.GoMap[k] = f[1]
			}
		}
	}
	return err
}

// NewOvsMap will return an OVSDB style map from a provided Golang Map
func NewOvsMap(goMap interface{}) (OvsMap, error) {
	v := reflect.ValueOf(goMap)
	if v.Kind() != reflect.Map {
		return OvsMap{}, fmt.Errorf("ovsmap supports only go map types")
	}

	genMap := make(map[interface{}]interface{})
	keys := v.MapKeys()
	for _, key := range keys {
		genMap[key.Interface()] = v.MapIndex(key).Interface()
	}
	return OvsMap{genMap}, nil
}
