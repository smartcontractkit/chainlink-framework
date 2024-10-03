package evm

import (
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
)

var arrayPattern *regexp.Regexp = regexp.MustCompile(`^([a-zA-Z0-9]+)\[(\d+)?\]$`)
var intPattern *regexp.Regexp = regexp.MustCompile(`^(u?int)(8|16|24|32|40|48|56|64|72|80|88|96|104|112|120|128|136|144|152|160|168|176|184|192|200|208|216|224|232|240|248|256)?$`)

func SolidityPrimitiveTypeToGoType(abiType abi.Type) (string, error) {
	if abiType.TupleType != nil {
		return "", nil
	}
	return abiTypeStringToGoType(abiType.String())
}

func abiTypeStringToGoType(abiType string) (string, error) {
	//TODO BCFR-979 - Add support for all solidity primitive types
	switch abiType {
	case "bool":
		return reflect.TypeOf(false).String(), nil
	case "string":
		return reflect.TypeOf("").String(), nil
	//TODO BCFR-980 - Decouple chain agnostic bindings from geth address
	case "address":
		return reflect.TypeOf(common.Address{}).String(), nil
	case "address[]":
		return reflect.TypeOf([]common.Address{}).String(), nil
	case "bytes2":
		return reflect.TypeOf([2]uint8{}).String(), nil
	case "bytes":
		return reflect.TypeOf([]byte{}).String(), nil
	case "bytes32":
		return reflect.TypeOf([32]byte{}).String(), nil
	default:
		switch {
		case isMatch(arrayPattern, abiType):
			return getArrayType(abiType)
		case isMatch(intPattern, abiType):
			return getNumericType(abiType)
		default:
			return "", fmt.Errorf("unknown abiType: %s", abiType)
		}
	}
}

func isMatch(regexp *regexp.Regexp, input string) bool {
	match := regexp.FindStringSubmatch(input)
	return len(match) > 0
}

func getArrayType(input string) (string, error) {
	matches := arrayPattern.FindStringSubmatch(input)
	if len(matches) == 0 {
		return "", fmt.Errorf("input string does not match the pattern")
	}
	basicType := matches[1]
	var arraySize string
	if matches[2] == "" {
		arraySize = ""
	} else {
		arraySize = matches[2]
	}
	basicTypeGoType, err := abiTypeStringToGoType(basicType)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("[%s]%s", arraySize, basicTypeGoType), nil
}

func getNumericType(input string) (string, error) {
	matches := intPattern.FindStringSubmatch(input)
	if len(matches) == 0 {
		return "", fmt.Errorf("input does not match the pattern")
	}
	typePrefix := matches[1]
	if matches[2] == "" {
		return typePrefix, nil
	}
	number, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", fmt.Errorf("invalid number: %v", err)
	}
	if number <= 64 {
		return input, nil
	} else {
		return reflect.TypeOf(&big.Int{}).String(), nil
	}
}
