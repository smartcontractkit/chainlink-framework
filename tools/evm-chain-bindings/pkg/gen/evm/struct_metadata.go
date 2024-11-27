package evm

import (
	"fmt"
	"reflect"
	"strings"
)

// Takes an instance of an struct and it will generate a map with metadata around the struct
// and it's content that can be used to generate go code with a declaration of a variable with the same value.
func GenerateValueDescriptor(value interface{}) map[string]interface{} {
	return valueDescriptor(value, false)
}

func valueDescriptor(value interface{}, isPointer bool) map[string]interface{} {
	reflectValue := reflect.ValueOf(value)
	reflectType := reflect.TypeOf(value)
	var valueStr string
	isStruct := false
	isArray := false
	isMap := false
	var mapValues map[interface{}]interface{}
	var arrayValues []interface{}
	var keyType, valueType string
	var fields map[string]interface{}
	var typeStr = reflectType.String()
	//TODO review, we are dereferencing and nothing mroe
	if reflectValue.Kind() == reflect.Ptr {
		reflectValue = reflectValue.Elem()
		reflectType = reflect.TypeOf(reflectValue.Interface())
		typeStr = strings.TrimPrefix(typeStr, "*")
		typeStr = "&" + typeStr
	}
	switch reflectValue.Kind() {
	case reflect.String:
		valueStr = fmt.Sprintf("%q", reflectValue.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		valueStr = fmt.Sprintf("%d", reflectValue.Int())
	case reflect.Float32, reflect.Float64:
		valueStr = fmt.Sprintf("%f", reflectValue.Float())
	case reflect.Bool:
		valueStr = fmt.Sprintf("%t", reflectValue.Bool())
	case reflect.Struct:
		isStruct = true
		fields = make(map[string]interface{}, reflectType.NumField())
		for i := 0; i < reflectType.NumField(); i++ {
			field := reflectType.Field(i)
			name := field.Name
			reflectFieldValue := reflectValue.Field(i)
			if field.IsExported() {
				skip := false
				fileIsPointer := false
				if reflectFieldValue.Kind() == reflect.Ptr {
					fileIsPointer = true
					if reflectFieldValue.IsNil() {
						skip = true
					} else {
						reflectFieldValue = reflectFieldValue.Elem()
					}
				}
				if !skip && reflectFieldValue.Interface() != nil {
					valueDefinition := valueDescriptor(reflectFieldValue.Interface(), fileIsPointer)
					if len(valueDefinition) > 0 {
						fields[name] = valueDefinition
					}
				}
			}
		}
	case reflect.Map:
		isMap = true
		mapValues = make(map[interface{}]interface{})
		for _, key := range reflectValue.MapKeys() {
			keyValue := reflectValue.MapIndex(key)
			fileIsPointer := false
			if keyValue.Kind() == reflect.Ptr {
				fileIsPointer = true
				keyValue = keyValue.Elem()
			}
			valueDefinition := valueDescriptor(keyValue.Interface(), fileIsPointer)
			if len(valueDefinition) > 0 {
				mapValues[key.Interface()] = valueDefinition
			}
		}
		keyType = reflectValue.Type().Key().String()
		valueType = reflectValue.Type().Elem().String()
	case reflect.Array, reflect.Slice:
		isArray = true
		arrayValues = make([]interface{}, reflectValue.Len())
		for i := 0; i < reflectValue.Len(); i++ {
			reflectArrayValue := reflectValue.Index(i)
			fileIsPointer := false
			if reflectArrayValue.Kind() == reflect.Ptr {
				fileIsPointer = true
				reflectArrayValue = reflectArrayValue.Elem()
			}
			valueDefinition := valueDescriptor(reflectArrayValue.Interface(), fileIsPointer)
			if len(valueDefinition) > 0 {
				arrayValues[i] = valueDefinition
			}
		}
	default:
		valueStr = fmt.Sprintf("%v", reflectValue.Interface())
	}

	valueDefinition := map[string]interface{}{
		"GoType":      typeStr,
		"Value":       valueStr,
		"IsStruct":    isStruct,
		"IsMap":       isMap,
		"IsArray":     isArray,
		"KeyType":     keyType,
		"ValueType":   valueType,
		"MapValues":   mapValues,
		"Fields":      fields,
		"ArrayValues": arrayValues,
		"IsPointer":   isPointer,
	}
	if isEmptyMap(isMap, mapValues) || isEmptyArray(isArray, arrayValues) || isEmptyStruct(isStruct, fields) {
		return map[string]interface{}{}
	}
	return valueDefinition
}

func isEmptyStruct(isStruct bool, fields map[string]interface{}) bool {
	return isStruct && len(fields) == 0
}

func isEmptyArray(isArray bool, arrayValues []interface{}) bool {
	return isArray && len(arrayValues) == 0
}

func isEmptyMap(isMap bool, mapValues map[interface{}]interface{}) bool {
	return isMap && len(mapValues) == 0
}
