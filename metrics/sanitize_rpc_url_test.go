package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeRPCURL_RedactsSecrets(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// simple path
		{"https://bsc-mainnet.core.chainstack.com/MjcwMDk3ZGFhMDA5NjJjMDM1", "https://bsc-mainnet.core.chainstack.com/b0b99e8b33b401b05251f91aec08b6a9581c86dd"},
		// less simple path
		{"http://172.16.156.14:8000/MmZmNTJmOWRiNzg0NTgxNDYyNzJjMTYzNDlmNGJ/iYWEwOTVmYWE0OQ/bsc/mainnet/", "http://172.16.156.14:8000/bd161d616d46b90a248de0f0a3ebc2daf2b8bb20"},
		// path with no / is excluded from sha
		{"https://anyblocks-01.mainnet.bnb.bdnodes.net?auth=MDcwMTgzODk3NzIyMjU4YzY2MTQzNGMyNTU2OWE2NGEzYjhlODM0NA", "https://anyblocks-01.mainnet.bnb.bdnodes.net/7c87697e63d8f9c049183bb4f8c171af40715b2b"},
		// path with leading / is included in sha
		{"https://anyblocks-02.mainnet.bnb.bdnodes.net/somepath/?auth=2Dc8bNAqCC0X74zZfi_4ra6XzuBY8lmXcTE1ic9EO5o", "https://anyblocks-02.mainnet.bnb.bdnodes.net/22c028ea2d53fd106e2bb93bc61d838ed6b01c19"},
		// strip creds keep path
		{"https://myLittleNop:YjY5MjAwOGJkMzBjNW@broadcast-mirror.fiews.io/?chain_id=56", "https://broadcast-mirror.fiews.io/?chain_id=56"},
		// even if no creds, sacrifice path for uniformity
		{"https://eu-bsc.rpc.linkriver.internal/rpc", "https://eu-bsc.rpc.linkriver.internal/e64b40f2bd5c8a9560773d16476a86ede7e7c1ba"},
		// keeps protocol too
		{"wss://bsc-mainnet-proxy.internal.linkpool.io/ws", "wss://bsc-mainnet-proxy.internal.linkpool.io/1457b75dc8c5500c0f1d4503cf801b60deb045a4"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, SanitizeRPCURL(tc.input))
		})
	}
}

func TestSanitizeRPCURL_AlreadySanitized(t *testing.T) {
	urls := []string{
		"http://10.0.1.191:8545",
		"http://144.178.241.22:8545",
		"http://222.106.187.14:12001",
		"http://at2-bsc-main03.blockchain.fiews.net:8545",
		"http://berlioz.stakesystems.io:8745",
		"http://blockchains-1.shultzpro.com:8545",
		"http://dfw3-bsc-main01.blockchain.fiews.net:8545",
		"http://sylvester.stakesystems.io:8745",
		"https://bsc-dataseed.binance.org/",
		"https://chainlink-bsc.rpc.blxrbdn.com",
		"https://puissant-builder.48.club",
		"ws://10.0.1.191:8546",
		"ws://144.76.108.206:8546",
		"ws://172.16.152.140:8546",
		"ws://bsc-rpc-2.piertwo.prod:8546",
		"ws://bsc.rpc.cinternal.com",
		"ws://sylvester.stakesystems.io:8746",
		"wss://bsc-rpc.o1.wtf",
	}

	for _, u := range urls {
		t.Run(u, func(t *testing.T) {
			assert.Equal(t, u, SanitizeRPCURL(u))
		})
	}
}
