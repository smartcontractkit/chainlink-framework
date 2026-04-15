package metrics

import (
	"net/url"
	"strings"
)

const redactedPathToken = "REDACTED"

// sanitizeRPCURLForMetrics strips credentials and query, and replaces path segments that
// look like opaque API keys so values are safe for Prometheus labels and telemetry.
func sanitizeRPCURLForMetrics(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid_rpc_url"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""

	path := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
	if path == "" {
		u.Path = ""
		return u.String()
	}
	segs := strings.Split(path, "/")
	for i, seg := range segs {
		if segmentLooksSensitive(seg) {
			segs[i] = redactedPathToken
		}
	}
	u.Path = "/" + strings.Join(segs, "/")
	return u.String()
}

func segmentLooksSensitive(seg string) bool {
	const minLen = 20
	if len(seg) < minLen {
		return false
	}
	if !isOpaqueURLPathToken(seg) {
		return false
	}
	// Standard base64 uses +, /, and = (padding). If any appear in a long path segment, treat
	// the whole segment as credential material (base64url omits these but classic base64 does not).
	if strings.ContainsAny(seg, "+/=") {
		return true
	}
	hasDigit := false
	hasLower := false
	hasUpper := false
	for _, r := range seg {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		}
	}
	if hasDigit || (hasLower && hasUpper) {
		return true
	}
	if len(seg) >= 32 && isLowerHexString(seg) {
		return true
	}
	return false
}

// isOpaqueURLPathToken is true for segments made of typical API-key / base64 / base64url
// characters so we can apply redaction heuristics. Includes + and = for standard base64.
func isOpaqueURLPathToken(seg string) bool {
	for _, r := range seg {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '+' || r == '=' || r == '/' {
			continue
		}
		return false
	}
	return true
}

func isLowerHexString(s string) bool {
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}
