package spec

import (
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"
)

type ChainComponentsSpec struct {
	SmartContracts []SmartContract
}

type SmartContract struct {
	ReadEntities    []ReadEntity
	WriteOperations []WriteOperation
}

type QuerySupport struct {
	IsQueryble      bool
	ConfidenceLevel *primitives.ConfidenceLevel
}

type ReadEntity struct {
	Identifier   string
	Type         Type
	CustomFilter Inputs
	QuerySupport QuerySupport
}

type Type struct {
	Name    string
	GoType  reflect.Type
	Encoded bool
}

type Inputs []Type

type WriteOperation struct {
	Identifier string
	Inputs     Inputs
}
