package evm

import (
	"github.com/smartcontractkit/chainlink-framework/tools/evm-chain-bindings/pkg/utils"
	"github.com/smartcontractkit/chainlink/v2/core/chains/evm/assets"
	"github.com/smartcontractkit/chainlink/v2/core/services/relay/evm/types"
	"math/big"
)

func GenerateChainReaderChainWriterConfig(contracts map[string]CodeDetails) (types.ChainReaderConfig, types.ChainWriterConfig, error) {
	chainContractReaders := map[string]types.ChainContractReader{}
	chainWriterContractConfig := map[string]*types.ContractConfig{}

	for contractName, contractDetails := range contracts {
		chainReaderDefinitions := map[string]*types.ChainReaderDefinition{}
		chainWriterDefinitions := map[string]*types.ChainWriterDefinition{}

		for _, function := range contractDetails.Functions {
			if function.RequiresTransaction {
				//TODO BCFR-969 - Provide proper default values for ChainReader and ChainWriter config
				chainWriterDefinitions[utils.CapitalizeFirstLetter(function.Name)] = &types.ChainWriterDefinition{
					ChainSpecificName: function.SolidityName,
					Checker:           "simulate",
				}
			} else {
				//TODO BCFR-969 - Provide proper default values for ChainReader and ChainWriter config
				chainReaderDefinitions[utils.CapitalizeFirstLetter(function.Name)] = &types.ChainReaderDefinition{
					CacheEnabled:      false,
					ChainSpecificName: function.SolidityName,
				}
			}
		}

		contractReader := types.ChainContractReader{
			ContractABI: contractDetails.ABI,
			Configs:     chainReaderDefinitions,
		}

		contractConfig := types.ContractConfig{
			ContractABI: contractDetails.ABI,
			Configs:     chainWriterDefinitions,
		}

		chainContractReaders[utils.CapitalizeFirstLetter(contractName)] = contractReader
		chainWriterContractConfig[utils.CapitalizeFirstLetter(contractName)] = &contractConfig
	}

	chainReaderConfig := types.ChainReaderConfig{
		Contracts: chainContractReaders,
	}

	maxGasPrice := assets.NewWei(big.NewInt(10000000))
	chainWriterConfig := types.ChainWriterConfig{
		Contracts:   chainWriterContractConfig,
		MaxGasPrice: maxGasPrice,
	}

	return chainReaderConfig, chainWriterConfig, nil
}
