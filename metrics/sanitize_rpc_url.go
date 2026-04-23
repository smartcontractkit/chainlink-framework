package metrics

import (
	"crypto/sha1" //nolint:gosec // sha1 used only for URL anonymisation, not security
	"fmt"
	"net/url"
	"strings"
)

// SanitizeRPCURL either strips user:passwd or replaces path and params with their sha1-hex, excluding leading / if present
func SanitizeRPCURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid_rpc_url"
	}

	if u.User != nil {
		// Strip credentials and leave everything else intact.
		u.User = nil
		return u.String()
	}

	// Build the sensitive portion: path (without leading /) plus optional query.
	sensitive := strings.TrimPrefix(u.Path, "/")
	if u.RawQuery != "" {
		if sensitive != "" {
			sensitive += "?" + u.RawQuery
		} else {
			sensitive = u.RawQuery
		}
	}

	if sensitive == "" {
		// Nothing to redact.
		return u.String()
	}

	//nolint:gosec
	h := sha1.Sum([]byte(sensitive))
	u.Path = "/" + fmt.Sprintf("%x", h)
	u.RawQuery = ""
	return u.String()
}
