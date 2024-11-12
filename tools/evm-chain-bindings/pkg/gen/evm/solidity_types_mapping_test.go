package evm

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSolidityPrimitiveTypeToGoType(t *testing.T) {
	fromToType := map[string]string{
		"uint8":        "uint8",
		"uint16":       "uint16",
		"uint32":       "uint32",
		"uint64":       "uint64",
		"uint192":      "*big.Int",
		"uint256":      "*big.Int",
		"int8":         "int8",
		"int16":        "int16",
		"int32":        "int32",
		"int64":        "int64",
		"int192":       "*big.Int",
		"int256":       "*big.Int",
		"bool":         "bool",
		"string":       "string",
		"address":      "common.Address",
		"address[]":    "[]common.Address",
		"bytes2":       "[2]uint8",
		"bytes":        "[]uint8",
		"bytes32":      "[32]uint8",
		"uint[32]":     "[32]uint",
		"uint8[32]":    "[32]uint8",
		"uint64[]":     "[]uint64",
		"uint[7]":      "[7]uint",
		"uint64[2993]": "[2993]uint64",
		"string[]":     "[]string",
	}
	for solidityType, expectedGoType := range fromToType {
		goType, err := abiTypeStringToGoType(solidityType)
		require.NoError(t, err)
		assert.Equal(t, expectedGoType, goType)
	}
	_, err := abiTypeStringToGoType("badType")
	require.Error(t, err, "badType is not a valid type")
	_, err = abiTypeStringToGoType("uint3")
	require.Error(t, err, "bad uint size")
	_, err = abiTypeStringToGoType("int3")
	require.Error(t, err, "bad int size")
	_, err = abiTypeStringToGoType("int512")
	require.Error(t, err, "large int size")
}
