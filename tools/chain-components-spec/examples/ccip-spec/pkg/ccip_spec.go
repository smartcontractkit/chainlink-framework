package spec

import (
	"math/big"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"
	"github.com/smartcontractkit/chainlink-framework/tools/chain-components-spec/pkg/spec"
)

type LatestRoundData struct {
	RoundId         *big.Int
	Answer          *big.Int
	StartedAt       *big.Int
	UpdatedAt       *big.Int
	AnsweredInRound *big.Int
}

type TimestampedUnixBig struct {
	Value     *big.Int
	Timestamp uint32
}

type (
	UnknownAddress []byte
	ChainSelector  uint
	SeqNum         uint64
	Bytes          []byte
	Bytes32        [32]byte
)

type RampMessageHeader struct {
	OnRamp              UnknownAddress
	SourceChainSelector ChainSelector
	DestChainSelector   ChainSelector
	SequenceNumber      SeqNum
	Nonce               uint64
	MessageID           Bytes32
	MsgHash             Bytes32
}

type CCIPMessage struct {
	FeeTokenAmount *big.Int
	FeeValueJuels  *big.Int
	Sender         UnknownAddress
	Data           Bytes
	Receiver       UnknownAddress
	ExtraArgs      Bytes
	FeeToken       UnknownAddress
	Header         RampMessageHeader
}

type CCIPMessageSent struct {
	Message           CCIPMessage
	DestChainSelector ChainSelector
}

func CCIPSpec() spec.ChainComponentsSpec {
	return spec.ChainComponentsSpec{
		SmartContracts: []spec.SmartContract{
			{
				ReadEntities: []spec.ReadEntity{
					{
						Identifier: "LatestRoundData",
						Type: spec.Type{
							GoType: reflect.TypeOf(LatestRoundData{}),
						},
						QuerySupport: spec.QuerySupport{
							IsQueryble: false,
						},
					},
					{
						Identifier: "TokenPrice",
						Type: spec.Type{
							GoType: reflect.TypeOf(TimestampedUnixBig{}),
						},
						CustomFilter: spec.Inputs{
							spec.Type{
								Name:   "address",
								GoType: reflect.TypeOf(""),
							},
						},
						QuerySupport: spec.QuerySupport{
							IsQueryble: false,
						},
					},
					{
						Identifier: "CCIPMesageSent",
						Type: spec.Type{
							GoType: reflect.TypeOf(CCIPMessageSent{}),
						},
						QuerySupport: spec.QuerySupport{
							IsQueryble:      true,
							ConfidenceLevel: ptr(primitives.Finalized),
						},
					},
				},
				WriteOperations: []spec.WriteOperation{
					{
						Identifier: "Commit",
						Inputs: []spec.Type{{
							Name:   "reportContext",
							GoType: reflect.TypeOf([3]Bytes32{}),
						}, {
							Name:   "report",
							GoType: reflect.TypeOf(Bytes{}),
						}, {
							Name:   "rs",
							GoType: reflect.TypeOf(Bytes{}),
						}, {
							Name:   "ss",
							GoType: reflect.TypeOf(Bytes{}),
						}, {
							Name:   "rawVs",
							GoType: reflect.TypeOf(Bytes32{}),
						}},
					},
					{
						Identifier: "Execute",
						Inputs: []spec.Type{{
							Name:   "reportContext",
							GoType: reflect.TypeOf([3]Bytes32{}),
						}, {
							Name:    "report",
							GoType:  reflect.TypeOf(Bytes{}),
							Encoded: true,
						}},
					},
				},
			},
		},
	}
}

// Ptr returns a pointer to the provided value of any type.
func ptr[T any](value T) *T {
	return &value
}
