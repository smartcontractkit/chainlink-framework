package metrics

import (
	"crypto/sha1" //nolint:gosec // sha1 used only for URL anonymisation, not security
	"fmt"
	"net/url"
	"strings"
)

// SanitizeRPCURL either strips user:passwd or replaces path and params with their sha1-hex, excluding leading / if present
func SanitizeRPCURL(raw string) string {
	// url.Parse requires a scheme to correctly populate Host.
	// When none is present, prepend a temporary one and strip it afterwards.
	// TrimPrefix below is always safe: no real scheme starts with fakeScheme.
	const fakeScheme = "x://"
	if !strings.Contains(raw, "://") {
		raw = fakeScheme + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "invalid_rpc_url"
	}

	if u.User != nil {
		// Strip credentials and leave everything else intact.
		u.User = nil
		return strings.TrimPrefix(u.String(), fakeScheme)
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
		return strings.TrimPrefix(u.String(), fakeScheme)
	}

	//nolint:gosec
	h := sha1.Sum([]byte(sensitive))
	u.Path = "/" + fmt.Sprintf("%x", h)
	u.RawQuery = ""
	return strings.TrimPrefix(u.String(), fakeScheme)
}
