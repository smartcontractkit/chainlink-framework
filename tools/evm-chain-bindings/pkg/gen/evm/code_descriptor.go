package evm

import (
	"fmt"
	"log"
	"strings"

	"github.com/smartcontractkit/chainlink-framework/tools/evm-chain-bindings/pkg/utils"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type Field struct {
	Name         string
	SolidityName string
	Type         GoType
}

type GoType struct {
	IsStruct      bool
	StructDetails *Struct
	PrimitiveType string
}

func (g GoType) Print() string {
	if g.IsStruct {
		return g.StructDetails.Name
	}
	return g.PrimitiveType
}

type Struct struct {
	Name   string
	Fields []Field
}

type Function struct {
	Name                string
	SolidityName        string
	Input               *Struct
	Output              *GoType
	RequiresTransaction bool
	IsPayable           bool
	IsQueryKey          bool
}

type CodeDetails struct {
	ABI                string
	Functions          []Function
	Structs            *map[string]*Struct
	ContractStructName string
	ContractName       string
}

func solidityStructToGoStruct(abiType abi.Type, structs *map[string]*Struct, owner string, nameProvider func() string) (*Struct, error) {
	if abiType.TupleType == nil {
		return nil, nil
	}

	var structName string
	if abiType.TupleRawName != "" {
		name, err := solidityNameToGoName(abiType.TupleRawName, owner)
		if err != nil {
			return nil, err
		}
		structName = name
	} else {
		structName = nameProvider()
	}

	requiredStruct, exists := (*structs)[structName]
	if exists {
		return requiredStruct, nil
	}

	fields := []Field{}
	for i := 0; i < len(abiType.TupleElems); i++ {
		fieldAbiType := abiType.TupleElems[i]
		fieldAbiName := abiType.TupleRawNames[i]
		goFieldName, err := solidityNameToGoName(fieldAbiName, owner)
		if err != nil {
			return nil, err
		}

		owner := fmt.Sprintf("%s.%s", owner, structName)

		structNameProvider := func() string {
			return fmt.Sprintf("%s%s%d", structName, "Inner", i)
		}

		goType, err := abiTypeToGoType(*fieldAbiType, structs, owner, structNameProvider)
		fields = append(fields, Field{
			Name:         goFieldName,
			Type:         goType,
			SolidityName: fieldAbiName,
		})
	}

	s := Struct{
		Name:   structName,
		Fields: fields,
	}
	(*structs)[structName] = &s
	return &s, nil
}

func getParam(arguments abi.Arguments, structs *map[string]*Struct, contractName string, structName string) (*Struct, error) {
	if len(arguments) == 0 {
		return nil, nil
	}
	fields := []Field{}
	owner := fmt.Sprintf("%s.%s", contractName, contractName)
	for _, argument := range arguments {
		GoName, err := solidityNameToGoName(argument.Name, owner)
		nameProvider := func() string {
			return structName
		}
		goType, err := abiTypeToGoType(argument.Type, structs, owner, nameProvider)
		if err != nil {
			return nil, err
		}
		fields = append(fields, Field{
			Name:         GoName,
			Type:         goType,
			SolidityName: argument.Name,
		})
	}
	// If there's only one parameter of type struct do not wrap it in another struct.
	if len(fields) == 1 && fields[0].Type.IsStruct {
		return fields[0].Type.StructDetails, nil
	}
	s := Struct{
		Fields: fields,
		Name:   structName,
	}
	(*structs)[structName] = &s
	return &s, nil
}

func abiTypeToGoType(abiType abi.Type, structs *map[string]*Struct, owner string, nameProvider func() string) (GoType, error) {
	goStruct, err := solidityStructToGoStruct(abiType, structs, owner, nameProvider)
	if err != nil {
		return GoType{}, err
	}
	goType, err := SolidityPrimitiveTypeToGoType(abiType)
	if err != nil {
		return GoType{}, err
	}
	return GoType{
		IsStruct:      abiType.TupleType != nil,
		StructDetails: goStruct,
		PrimitiveType: goType,
	}, nil
}

func ConvertABIToCodeDetails(contractName string, abiJSON string) (CodeDetails, error) {
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		log.Fatalf("Failed to parse ABI: %v", err)
	}

	codeDetails := CodeDetails{
		ABI:                abiJSON,
		Functions:          make([]Function, 0),
		ContractStructName: contractName,
		ContractName:       contractName,
	}

	structs := &map[string]*Struct{}
	err = generateMethodsFunctions(contractName, parsedABI, structs, &codeDetails)
	if err != nil {
		return CodeDetails{}, err
	}

	//TODO BCFR-968 - Add support for reading data generated from events
	//details, err2, done := generateEventFunctions(contractName, parsedABI, structs, &codeDetails)
	//if done {
	//	return details, err2
	//}

	codeDetails.Structs = structs
	return codeDetails, nil
}

func generateMethodsFunctions(contractName string, parsedABI abi.ABI, structs *map[string]*Struct, codeDetails *CodeDetails) error {
	// Iterate over all the methods in the ABI
	for methodName, method := range parsedABI.Methods {

		// TODO fix this, logic may not be the same
		owner := createConstructOwnerId(contractName, methodName)
		fixedMethodName, err := solidityNameToGoName(methodName, owner)
		if err != nil {
			return err
		}
		// Initialize a Function struct for this method
		inputParam, err := getParam(method.Inputs, structs, owner, fixedMethodName+"Input")
		if err != nil {
			return err
		}
		outputParam, err := getParam(method.Outputs, structs, owner, fixedMethodName+"Output")
		if err != nil {
			return err
		}
		requiresTransaction, isPayable := requiresTransactionAndPayable(method.StateMutability)
		functionDetails := Function{
			Name:                fixedMethodName,
			SolidityName:        methodName,
			Input:               inputParam,
			Output:              convertToPrimitiveIfNeeded(outputParam),
			RequiresTransaction: requiresTransaction,
			IsPayable:           isPayable,
		}

		// Append the function details to the contract
		codeDetails.Functions = append(codeDetails.Functions, functionDetails)
	}
	return nil
}

func createConstructOwnerId(contractName string, methodName string) string {
	return fmt.Sprintf("%s.%s", contractName, methodName)
}

func generateEventFunctions(contractName string, parsedABI abi.ABI, structs *map[string]*Struct, codeDetails *CodeDetails) (CodeDetails, error, bool) {
	// For each event we will create structs for the data and another for the topics to use as filters.
	// Then create functions to retrieve last event using the topic struct as filter and returning the event struct.
	for eventName, event := range parsedABI.Events {
		owner := createConstructOwnerId(contractName, eventName)
		fixedEventName, err := solidityNameToGoName(eventName, owner)
		if err != nil {
			return CodeDetails{}, err, true
		}

		// Each event should have a struct
		eventStruct, err := getParam(event.Inputs, structs, eventName, fixedEventName)
		if err != nil {
			return CodeDetails{}, err, true
		}

		atLeastOneFieldIndexed := false
		for _, f := range event.Inputs {
			atLeastOneFieldIndexed = atLeastOneFieldIndexed || f.Indexed
		}

		var topicFilterStruct *Struct = nil
		if atLeastOneFieldIndexed {
			topicStructName := eventStruct.Name + "TopicsFilter"
			topicArguments := abi.Arguments{}
			for _, arg := range event.Inputs {
				if arg.Indexed {
					topicArguments = append(topicArguments, arg)
				}
			}

			topicFilterStruct, err = getParam(topicArguments, structs, contractName, topicStructName)
			pointerFields := []Field{}
			for _, field := range topicFilterStruct.Fields {
				pointerFields = append(pointerFields, Field{
					SolidityName: field.SolidityName,
					Name:         field.Name,
					Type: GoType{
						IsStruct:      field.Type.IsStruct,
						StructDetails: field.Type.StructDetails,
						PrimitiveType: "*" + field.Type.PrimitiveType,
					},
				})
			}

			(*structs)[topicFilterStruct.Name] = &Struct{
				Fields: pointerFields,
				Name:   topicFilterStruct.Name,
			}

			if err != nil {
				return CodeDetails{}, nil, true
			}
		}

		//We generate:
		//- GetLast[EventName](TopicFilers with each indexed event parameter) (EventStruct)
		//- Query[EventName](Expressions, potentially more strong typed) (EventStruct)
		getLastEventFunction := Function{
			Name:                "GetLast" + fixedEventName,
			SolidityName:        eventName,
			Input:               topicFilterStruct,
			Output:              &GoType{IsStruct: true, StructDetails: eventStruct},
			RequiresTransaction: false,
			IsPayable:           false,
		}

		codeDetails.Functions = append(codeDetails.Functions, getLastEventFunction)

		queryEventFunction := Function{
			Name:                "Query" + fixedEventName,
			SolidityName:        eventName,
			Input:               nil,
			Output:              &GoType{PrimitiveType: "[]" + eventStruct.Name},
			RequiresTransaction: false,
			IsPayable:           false,
			IsQueryKey:          true,
		}

		codeDetails.Functions = append(codeDetails.Functions, queryEventFunction)
	}
	return CodeDetails{}, nil, false
}

func convertToPrimitiveIfNeeded(param *Struct) *GoType {
	if param == nil {
		return nil
	}
	if len(param.Fields) == 1 {
		field := param.Fields[0]
		if !field.Type.IsStruct {
			return &field.Type
		}
	}
	return &GoType{
		IsStruct:      true,
		StructDetails: param,
	}
}

// isReadOnly takes the stateMutability string and returns true if the method is read-only, false if it requires a transaction
func requiresTransactionAndPayable(stateMutability string) (bool, bool) {
	requiresTransaction := false
	isPayable := false
	switch stateMutability {
	case "view", "pure":
		requiresTransaction = false
	case "payable":
		isPayable = true
		requiresTransaction = true
	case "nonpayable":
		requiresTransaction = true
	}
	return requiresTransaction, isPayable
}

func solidityNameToGoName(name string, owner string) (string, error) {
	if !utils.IsValidIdentifier(name) {
		return "", fmt.Errorf("invalid identifier for parameter: %s in %s, only digits and letters and underscore are allows", name, owner)
	}
	// Case of a single primitive return type.
	if name == "" {
		name = "Value"
	}
	if paramNeedsRenaming(name) {
		return getExpectedParameterName(name), nil
	}
	return name, nil
}

func getExpectedParameterName(paramName string) string {
	return utils.CapitalizeFirstLetter(utils.RemoveUnderscore(paramName))
}

func paramNeedsRenaming(paramName string) bool {
	return utils.StartsWithLowerCase(paramName) || utils.ContainsUnderscore(paramName)
}
