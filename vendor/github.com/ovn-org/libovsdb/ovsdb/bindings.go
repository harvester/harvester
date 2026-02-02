package ovsdb

import (
	"fmt"
	"reflect"
)

var (
	intType  = reflect.TypeOf(0)
	realType = reflect.TypeOf(0.0)
	boolType = reflect.TypeOf(true)
	strType  = reflect.TypeOf("")
)

// ErrWrongType describes typing error
type ErrWrongType struct {
	from     string
	expected string
	got      interface{}
}

func (e *ErrWrongType) Error() string {
	return fmt.Sprintf("Wrong Type (%s): expected %s but got %+v (%s)",
		e.from, e.expected, e.got, reflect.TypeOf(e.got))
}

// NewErrWrongType creates a new ErrWrongType
func NewErrWrongType(from, expected string, got interface{}) error {
	return &ErrWrongType{
		from:     from,
		expected: expected,
		got:      got,
	}
}

// NativeTypeFromAtomic returns the native type that can hold a value of an
// AtomicType
func NativeTypeFromAtomic(basicType string) reflect.Type {
	switch basicType {
	case TypeInteger:
		return intType
	case TypeReal:
		return realType
	case TypeBoolean:
		return boolType
	case TypeString:
		return strType
	case TypeUUID:
		return strType
	default:
		panic("Unknown basic type %s basicType")
	}
}

// NativeType returns the reflect.Type that can hold the value of a column
// OVS Type to Native Type convertions:
//
//	OVS sets -> go slices or a go native type depending on the key
//	OVS uuid -> go strings
//	OVS map  -> go map
//	OVS enum -> go native type depending on the type of the enum key
func NativeType(column *ColumnSchema) reflect.Type {
	switch column.Type {
	case TypeInteger, TypeReal, TypeBoolean, TypeUUID, TypeString:
		return NativeTypeFromAtomic(column.Type)
	case TypeEnum:
		return NativeTypeFromAtomic(column.TypeObj.Key.Type)
	case TypeMap:
		keyType := NativeTypeFromAtomic(column.TypeObj.Key.Type)
		valueType := NativeTypeFromAtomic(column.TypeObj.Value.Type)
		return reflect.MapOf(keyType, valueType)
	case TypeSet:
		keyType := NativeTypeFromAtomic(column.TypeObj.Key.Type)
		// optional type
		if column.TypeObj.Min() == 0 && column.TypeObj.Max() == 1 {
			return reflect.PtrTo(keyType)
		}
		// non-optional type with max 1
		if column.TypeObj.Min() == 1 && column.TypeObj.Max() == 1 {
			return keyType
		}
		return reflect.SliceOf(keyType)
	default:
		panic(fmt.Errorf("unknown extended type %s", column.Type))
	}
}

// OvsToNativeAtomic returns the native type of the basic ovs type
func OvsToNativeAtomic(basicType string, ovsElem interface{}) (interface{}, error) {
	switch basicType {
	case TypeReal, TypeString, TypeBoolean:
		naType := NativeTypeFromAtomic(basicType)
		if reflect.TypeOf(ovsElem) != naType {
			return nil, NewErrWrongType("OvsToNativeAtomic", naType.String(), ovsElem)
		}
		return ovsElem, nil
	case TypeInteger:
		naType := NativeTypeFromAtomic(basicType)
		// Default decoding of numbers is float64, convert them to int
		if !reflect.TypeOf(ovsElem).ConvertibleTo(naType) {
			return nil, NewErrWrongType("OvsToNativeAtomic", fmt.Sprintf("Convertible to %s", naType), ovsElem)
		}
		return reflect.ValueOf(ovsElem).Convert(naType).Interface(), nil
	case TypeUUID:
		uuid, ok := ovsElem.(UUID)
		if !ok {
			return nil, NewErrWrongType("OvsToNativeAtomic", "UUID", ovsElem)
		}
		return uuid.GoUUID, nil
	default:
		panic(fmt.Errorf("unknown atomic type %s", basicType))
	}
}

func OvsToNativeSlice(baseType string, ovsElem interface{}) (interface{}, error) {
	naType := NativeTypeFromAtomic(baseType)
	var nativeSet reflect.Value
	switch ovsSet := ovsElem.(type) {
	case OvsSet:
		nativeSet = reflect.MakeSlice(reflect.SliceOf(naType), 0, len(ovsSet.GoSet))
		for _, v := range ovsSet.GoSet {
			nv, err := OvsToNativeAtomic(baseType, v)
			if err != nil {
				return nil, err
			}
			nativeSet = reflect.Append(nativeSet, reflect.ValueOf(nv))
		}

	default:
		nativeSet = reflect.MakeSlice(reflect.SliceOf(naType), 0, 1)
		nv, err := OvsToNativeAtomic(baseType, ovsElem)
		if err != nil {
			return nil, err
		}

		nativeSet = reflect.Append(nativeSet, reflect.ValueOf(nv))
	}
	return nativeSet.Interface(), nil
}

// OvsToNative transforms an ovs type to native one based on the column type information
func OvsToNative(column *ColumnSchema, ovsElem interface{}) (interface{}, error) {
	switch column.Type {
	case TypeReal, TypeString, TypeBoolean, TypeInteger, TypeUUID:
		return OvsToNativeAtomic(column.Type, ovsElem)
	case TypeEnum:
		return OvsToNativeAtomic(column.TypeObj.Key.Type, ovsElem)
	case TypeSet:
		naType := NativeType(column)
		// The inner slice is []interface{}
		// We need to convert it to the real type os slice
		switch naType.Kind() {
		case reflect.Ptr:
			switch ovsSet := ovsElem.(type) {
			case OvsSet:
				if len(ovsSet.GoSet) > 1 {
					return nil, fmt.Errorf("expected a slice of len =< 1, but got a slice with %d elements", len(ovsSet.GoSet))
				}
				if len(ovsSet.GoSet) == 0 {
					return reflect.Zero(naType).Interface(), nil
				}
				native, err := OvsToNativeAtomic(column.TypeObj.Key.Type, ovsSet.GoSet[0])
				if err != nil {
					return nil, err
				}
				pv := reflect.New(naType.Elem())
				pv.Elem().Set(reflect.ValueOf(native))
				return pv.Interface(), nil
			default:
				native, err := OvsToNativeAtomic(column.TypeObj.Key.Type, ovsElem)
				if err != nil {
					return nil, err
				}
				pv := reflect.New(naType.Elem())
				pv.Elem().Set(reflect.ValueOf(native))
				return pv.Interface(), nil
			}
		case reflect.Slice:
			return OvsToNativeSlice(column.TypeObj.Key.Type, ovsElem)
		default:
			return nil, fmt.Errorf("native type was not slice or pointer. got %d", naType.Kind())
		}
	case TypeMap:
		naType := NativeType(column)
		ovsMap, ok := ovsElem.(OvsMap)
		if !ok {
			return nil, NewErrWrongType("OvsToNative", "OvsMap", ovsElem)
		}
		// The inner slice is map[interface]interface{}
		// We need to convert it to the real type os slice
		nativeMap := reflect.MakeMapWithSize(naType, len(ovsMap.GoMap))
		for k, v := range ovsMap.GoMap {
			nk, err := OvsToNativeAtomic(column.TypeObj.Key.Type, k)
			if err != nil {
				return nil, err
			}
			nv, err := OvsToNativeAtomic(column.TypeObj.Value.Type, v)
			if err != nil {
				return nil, err
			}
			nativeMap.SetMapIndex(reflect.ValueOf(nk), reflect.ValueOf(nv))
		}
		return nativeMap.Interface(), nil
	default:
		panic(fmt.Sprintf("Unknown Type: %v", column.Type))
	}
}

// NativeToOvsAtomic returns the OVS type of the atomic native value
func NativeToOvsAtomic(basicType string, nativeElem interface{}) (interface{}, error) {
	naType := NativeTypeFromAtomic(basicType)
	if reflect.TypeOf(nativeElem) != naType {
		return nil, NewErrWrongType("NativeToOvsAtomic", naType.String(), nativeElem)
	}
	switch basicType {
	case TypeUUID:
		return UUID{GoUUID: nativeElem.(string)}, nil
	default:
		return nativeElem, nil
	}
}

// NativeToOvs transforms an native type to a ovs type based on the column type information
func NativeToOvs(column *ColumnSchema, rawElem interface{}) (interface{}, error) {
	naType := NativeType(column)
	if t := reflect.TypeOf(rawElem); t != naType {
		return nil, NewErrWrongType("NativeToOvs", naType.String(), rawElem)
	}

	switch column.Type {
	case TypeInteger, TypeReal, TypeString, TypeBoolean, TypeEnum:
		return rawElem, nil
	case TypeUUID:
		return UUID{GoUUID: rawElem.(string)}, nil
	case TypeSet:
		var ovsSet OvsSet
		if column.TypeObj.Key.Type == TypeUUID {
			ovsSlice := []interface{}{}
			if _, ok := rawElem.([]string); ok {
				for _, v := range rawElem.([]string) {
					uuid := UUID{GoUUID: v}
					ovsSlice = append(ovsSlice, uuid)
				}
			} else if _, ok := rawElem.(*string); ok {
				v := rawElem.(*string)
				if v != nil {
					uuid := UUID{GoUUID: *v}
					ovsSlice = append(ovsSlice, uuid)
				}
			} else {
				return nil, fmt.Errorf("uuid slice was neither []string or *string")
			}
			ovsSet = OvsSet{GoSet: ovsSlice}

		} else {
			var err error
			ovsSet, err = NewOvsSet(rawElem)
			if err != nil {
				return nil, err
			}
		}
		return ovsSet, nil
	case TypeMap:
		nativeMapVal := reflect.ValueOf(rawElem)
		ovsMap := make(map[interface{}]interface{}, nativeMapVal.Len())
		for _, key := range nativeMapVal.MapKeys() {
			ovsKey, err := NativeToOvsAtomic(column.TypeObj.Key.Type, key.Interface())
			if err != nil {
				return nil, err
			}
			ovsVal, err := NativeToOvsAtomic(column.TypeObj.Value.Type, nativeMapVal.MapIndex(key).Interface())
			if err != nil {
				return nil, err
			}
			ovsMap[ovsKey] = ovsVal
		}
		return OvsMap{GoMap: ovsMap}, nil

	default:
		panic(fmt.Sprintf("Unknown Type: %v", column.Type))
	}
}

// IsDefaultValue checks if a provided native element corresponds to the default value of its
// designated column type
func IsDefaultValue(column *ColumnSchema, nativeElem interface{}) bool {
	switch column.Type {
	case TypeEnum:
		return isDefaultBaseValue(nativeElem, column.TypeObj.Key.Type)
	default:
		return isDefaultBaseValue(nativeElem, column.Type)
	}
}

// ValidateMutationAtomic checks if the mutation is valid for a specific AtomicType
func validateMutationAtomic(atype string, mutator Mutator, value interface{}) error {
	nType := NativeTypeFromAtomic(atype)
	if reflect.TypeOf(value) != nType {
		return NewErrWrongType(fmt.Sprintf("Mutation of atomic type %s", atype), nType.String(), value)
	}

	switch atype {
	case TypeUUID, TypeString, TypeBoolean:
		return fmt.Errorf("atomictype %s does not support mutation", atype)
	case TypeReal:
		switch mutator {
		case MutateOperationAdd, MutateOperationSubtract, MutateOperationMultiply, MutateOperationDivide:
			return nil
		default:
			return fmt.Errorf("wrong mutator for real type %s", mutator)
		}
	case TypeInteger:
		switch mutator {
		case MutateOperationAdd, MutateOperationSubtract, MutateOperationMultiply, MutateOperationDivide, MutateOperationModulo:
			return nil
		default:
			return fmt.Errorf("wrong mutator for integer type: %s", mutator)
		}
	default:
		panic("Unsupported Atomic Type")
	}
}

// ValidateMutation checks if the mutation value and mutator string area appropriate
// for a given column based on the rules specified RFC7047
func ValidateMutation(column *ColumnSchema, mutator Mutator, value interface{}) error {
	if !column.Mutable() {
		return fmt.Errorf("column is not mutable")
	}
	switch column.Type {
	case TypeSet:
		switch mutator {
		case MutateOperationInsert, MutateOperationDelete:
			// RFC7047 says a <set> may be an <atom> with a single
			// element. Check if we can store this value in our column
			if reflect.TypeOf(value).Kind() != reflect.Slice {
				if NativeType(column) != reflect.SliceOf(reflect.TypeOf(value)) {
					return NewErrWrongType(fmt.Sprintf("Mutation %s of single value in to column %s", mutator, column),
						NativeType(column).String(), reflect.SliceOf(reflect.TypeOf(value)).String())
				}
				return nil
			}
			if NativeType(column) != reflect.TypeOf(value) {
				return NewErrWrongType(fmt.Sprintf("Mutation %s of column %s", mutator, column),
					NativeType(column).String(), value)
			}
			return nil
		default:
			return validateMutationAtomic(column.TypeObj.Key.Type, mutator, value)
		}
	case TypeMap:
		switch mutator {
		case MutateOperationInsert:
			// Value must be a map of the same kind
			if reflect.TypeOf(value) != NativeType(column) {
				return NewErrWrongType(fmt.Sprintf("Mutation %s of column %s", mutator, column),
					NativeType(column).String(), value)
			}
			return nil
		case MutateOperationDelete:
			// Value must be a map of the same kind or a set of keys to delete
			if reflect.TypeOf(value) != NativeType(column) &&
				reflect.TypeOf(value) != reflect.SliceOf(NativeTypeFromAtomic(column.TypeObj.Key.Type)) {
				return NewErrWrongType(fmt.Sprintf("Mutation %s of column %s", mutator, column),
					"compatible map type", value)
			}
			return nil

		default:
			return fmt.Errorf("wrong mutator for map type: %s", mutator)
		}
	case TypeEnum:
		// RFC does not clarify what to do with enums.
		return fmt.Errorf("enums do not support mutation")
	default:
		return validateMutationAtomic(column.Type, mutator, value)
	}
}

func ValidateCondition(column *ColumnSchema, function ConditionFunction, nativeValue interface{}) error {
	if NativeType(column) != reflect.TypeOf(nativeValue) {
		return NewErrWrongType(fmt.Sprintf("Condition for column %s", column),
			NativeType(column).String(), nativeValue)
	}

	switch column.Type {
	case TypeSet, TypeMap, TypeBoolean, TypeString, TypeUUID:
		switch function {
		case ConditionEqual, ConditionNotEqual, ConditionIncludes, ConditionExcludes:
			return nil
		default:
			return fmt.Errorf("wrong condition function %s for type: %s", function, column.Type)
		}
	case TypeInteger, TypeReal:
		// All functions are valid
		return nil
	default:
		panic("Unsupported Type")
	}
}

func isDefaultBaseValue(elem interface{}, etype ExtendedType) bool {
	value := reflect.ValueOf(elem)
	if !value.IsValid() {
		return true
	}
	if reflect.TypeOf(elem).Kind() == reflect.Ptr {
		return reflect.ValueOf(elem).IsZero()
	}
	switch etype {
	case TypeUUID:
		return elem.(string) == "00000000-0000-0000-0000-000000000000" || elem.(string) == ""
	case TypeMap, TypeSet:
		if value.Kind() == reflect.Array {
			return value.Len() == 0
		}
		return value.IsNil() || value.Len() == 0
	case TypeString:
		return elem.(string) == ""
	case TypeInteger:
		return elem.(int) == 0
	case TypeReal:
		return elem.(float64) == 0
	default:
		return false
	}
}
