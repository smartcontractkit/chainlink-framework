package metrics

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_sanitizeRPCURLForMetrics(t *testing.T) {
	t.Parallel()
	// Synthetic keys only (Ab1… / Ab1cd…): same hosts, path shapes, and segment lengths as
	// common production RPC URL patterns; no real credentials in this repo.
	const key22 = "Ab1cdEf2Gh3Ij4Kl5Mn6Op"
	const key32 = "Ab1cdEf2Gh3Ij4Kl5Mn6Op7Qr8St9Uv"
	const key32Underscore = "Ab1cdEf2Gh3Ij4Kl5Mn6Op7Q_8St9Uv"

	tests := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "alchemy arb-sepolia shape",
			in:   "wss://arb-sepolia.g.alchemy.com/v2/" + key32,
			out:  "wss://arb-sepolia.g.alchemy.com/v2/REDACTED",
		},
		{
			name: "alchemy plasma-testnet shape short key",
			in:   "wss://plasma-testnet.g.alchemy.com/v2/" + key22,
			out:  "wss://plasma-testnet.g.alchemy.com/v2/REDACTED",
		},
		{
			name: "linkpool unichain-testnet /ws/ key",
			in:   "wss://unichain-testnet-cll.public.linkpool.io/ws/" + key32,
			out:  "wss://unichain-testnet-cll.public.linkpool.io/ws/REDACTED",
		},
		{
			name: "alchemy base-sepolia key with underscore",
			in:   "wss://base-sepolia.g.alchemy.com/v2/" + key32Underscore,
			out:  "wss://base-sepolia.g.alchemy.com/v2/REDACTED",
		},
		{
			name: "linkpool zora-sepolia /1/ws/ key",
			in:   "wss://zora-sepolia-cll.public.linkpool.io/1/ws/" + key32,
			out:  "wss://zora-sepolia-cll.public.linkpool.io/1/ws/REDACTED",
		},
		{
			name: "alchemy zora-sepolia short key",
			in:   "wss://zora-sepolia.g.alchemy.com/v2/" + key22,
			out:  "wss://zora-sepolia.g.alchemy.com/v2/REDACTED",
		},
		{
			name: "alchemy avax-fuji shape",
			in:   "wss://avax-fuji.g.alchemy.com/v2/" + key32,
			out:  "wss://avax-fuji.g.alchemy.com/v2/REDACTED",
		},
		{
			name: "base64-like path segment with plus slash equals",
			in:   "wss://rpc.example/v1/AbCdEfGh+IjKlMnOp/QrStUvWxYz012345678==",
			out:  "wss://rpc.example/v1/REDACTED",
		},
		{
			name: "short segment with plus not redacted",
			in:   "wss://rpc.example/v1/short+key",
			out:  "wss://rpc.example/v1/short+key",
		},
		{
			name: "localhost unchanged",
			in:   "http://localhost:8545",
			out:  "http://localhost:8545",
		},
		{
			name: "strips userinfo and query",
			in:   "https://user:secret@rpc.example/v1/path?apiKey=supersecret",
			out:  "https://rpc.example/v1/path",
		},
		{
			name: "hex only long path segment",
			in:   "https://mainnet.infura.io/v3/abcdef0123456789abcdef0123456789",
			out:  "https://mainnet.infura.io/v3/REDACTED",
		},
		{
			name: "hyphenated chain slug not confused with key",
			in:   "wss://lb.drpc.live/polygon-zkevm-cardona",
			out:  "wss://lb.drpc.live/polygon-zkevm-cardona",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.out, sanitizeRPCURLForMetrics(tt.in))
		})
	}
}

// Test_sanitizeRPCURLForMetrics_envFixtures validates real RPC URLs without storing them in git.
// Run locally (only; never commit values):
//
//	RPC_URL_SANITIZE_FIXTURES='wss://...,wss://...' go test ./metrics/... -run Test_sanitizeRPCURLForMetrics_envFixtures
func Test_sanitizeRPCURLForMetrics_envFixtures(t *testing.T) {
	rawList := strings.TrimSpace(os.Getenv("RPC_URL_SANITIZE_FIXTURES"))
	if rawList == "" {
		t.Skip(`set RPC_URL_SANITIZE_FIXTURES to a comma-separated list of RPC URLs to validate redaction; do not commit secrets`)
	}

	for i, raw := range strings.Split(rawList, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		t.Run(fixtureName(raw, i), func(t *testing.T) {
			u, err := url.Parse(raw)
			require.NoError(t, err, "fixture URL must parse")

			sanitized := sanitizeRPCURLForMetrics(raw)

			path := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
			anySensitive := false
			for _, seg := range strings.Split(path, "/") {
				if seg == "" {
					continue
				}
				if segmentLooksSensitive(seg) {
					anySensitive = true
					require.NotContains(t, sanitized, seg,
						"sanitized output must not contain sensitive path segment")
				}
			}
			if anySensitive {
				require.Contains(t, sanitized, redactedPathToken,
					"expected REDACTED when URL contained sensitive path segments")
			}

			if u.User != nil {
				if user := u.User.Username(); user != "" {
					require.NotContains(t, sanitized, user, "userinfo must be stripped")
				}
				if pw, ok := u.User.Password(); ok && pw != "" {
					require.NotContains(t, sanitized, pw, "userinfo must be stripped")
				}
			}
			if u.RawQuery != "" {
				require.NotContains(t, sanitized, u.RawQuery, "query must be stripped")
			}
		})
	}
}

func fixtureName(raw string, idx int) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "n" + strconv.Itoa(idx)
	}
	if h := u.Hostname(); h != "" {
		return fmt.Sprintf("%d_%s", idx, h)
	}
	return strconv.Itoa(idx)
}
