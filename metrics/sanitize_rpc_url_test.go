package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var toSanitizeCases = []struct {
	input string
	want  string
}{
	// simple path
	{"bsc-mainnet.core.chainstack.com/MjcwMDk3ZGFhMDA5NjJjMDM1", "bsc-mainnet.core.chainstack.com/b0b99e8b33b401b05251f91aec08b6a9581c86dd"},
	// less simple path
	{"172.16.156.14:8000/MmZmNTJmOWRiNzg0NTgxNDYyNzJjMTYzNDlmNGJ/iYWEwOTVmYWE0OQ/bsc/mainnet/", "172.16.156.14:8000/bd161d616d46b90a248de0f0a3ebc2daf2b8bb20"},
	// path with no / is excluded from sha
	{"anyblocks-01.mainnet.bnb.bdnodes.net?auth=MDcwMTgzODk3NzIyMjU4YzY2MTQzNGMyNTU2OWE2NGEzYjhlODM0NA", "anyblocks-01.mainnet.bnb.bdnodes.net/7c87697e63d8f9c049183bb4f8c171af40715b2b"},
	// path with leading / is included in sha
	{"anyblocks-02.mainnet.bnb.bdnodes.net/somepath/?auth=2Dc8bNAqCC0X74zZfi_4ra6XzuBY8lmXcTE1ic9EO5o", "anyblocks-02.mainnet.bnb.bdnodes.net/22c028ea2d53fd106e2bb93bc61d838ed6b01c19"},
	// strip creds keep path
	{"myLittleNop:YjY5MjAwOGJkMzBjNW@broadcast-mirror.fiews.io/?chain_id=56", "broadcast-mirror.fiews.io/?chain_id=56"},
	// even if no creds, sacrifice path for uniformity
	{"eu-bsc.rpc.linkriver.internal/rpc", "eu-bsc.rpc.linkriver.internal/e64b40f2bd5c8a9560773d16476a86ede7e7c1ba"},
	// keeps protocol too
	{"bsc-mainnet-proxy.internal.linkpool.io/ws", "bsc-mainnet-proxy.internal.linkpool.io/1457b75dc8c5500c0f1d4503cf801b60deb045a4"},
}

var alreadySanitizedCases = []string{
	"10.0.1.191:8545",
	"144.178.241.22:8545",
	"222.106.187.14:12001",
	"at2-bsc-main03.blockchain.fiews.net:8545",
	"berlioz.stakesystems.io:8745",
	"blockchains-1.shultzpro.com:8545",
	"dfw3-bsc-main01.blockchain.fiews.net:8545",
	"sylvester.stakesystems.io:8745",
	"bsc-dataseed.binance.org/",
	"chainlink-bsc.rpc.blxrbdn.com",
	"puissant-builder.48.club",
	"10.0.1.191:8546",
	"144.76.108.206:8546",
	"172.16.152.140:8546",
	"bsc-rpc-2.piertwo.prod:8546",
	"bsc.rpc.cinternal.com",
	"sylvester.stakesystems.io:8746",
	"bsc-rpc.o1.wtf",
}

var protocolParts = []string{
	"wss://",
	"https://",
	"http://",
	"",
}

func TestSanitizeRPCURL_RedactsSecrets(t *testing.T) {
	for _, prefix := range protocolParts {
		for _, tc := range toSanitizeCases {
			t.Run(tc.input, func(t *testing.T) {
				assert.Equal(t, prefix+tc.want, SanitizeRPCURL(prefix+tc.input))
			})
		}
	}
}

func TestSanitizeRPCURL_AlreadySanitized(t *testing.T) {
	for _, prefix := range protocolParts {
		for _, u := range alreadySanitizedCases {
			t.Run(u, func(t *testing.T) {
				assert.Equal(t, prefix+u, SanitizeRPCURL(prefix+u))
			})
		}
	}
}
