package informer

import (
	"reflect"
	"unsafe"
)

// UnsafeSet replaces the passed object's field value with the passed value.
func UnsafeSet(object any, field string, value any) {
	rs := reflect.ValueOf(object).Elem()
	rf := rs.FieldByName(field)
	wrf := reflect.NewAt(rf.Type(), unsafe.Pointer(rf.UnsafeAddr())).Elem()
	wrf.Set(reflect.ValueOf(value))
}

// UnsafeGet returns the value of the passed object's for the passed field.
func UnsafeGet(object any, field string) any {
	rs := reflect.ValueOf(object).Elem()
	rf := rs.FieldByName(field)
	wrf := reflect.NewAt(rf.Type(), unsafe.Pointer(rf.UnsafeAddr())).Elem()
	return wrf.Interface()
}
