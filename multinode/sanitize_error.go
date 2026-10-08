package multinode

import "regexp"

// sanitizedError redacts the message of an error while preserving the original for chain traversal.
type sanitizedError struct {
	err error  // original error, retained for chain traversal/classification
	msg string // redacted, safe-to-display message
}

func (e sanitizedError) Error() string {
	return e.msg
}

// Unwrap and Cause expose the original error so both the standard library
// (errors.Is/errors.As) and pkg/errors (pkgerrors.Cause) can still traverse to
// the underlying RPC error. This is required for error classification — e.g.
// extracting the JSON-RPC error via pkgerrors.Cause(err) — and for sentinel
// matching such as context.DeadlineExceeded.
//
// Only the Error() string is redacted. That is the value logged and returned to
// workflow users, which is where the provider URL/API key was leaking.
func (e sanitizedError) Unwrap() error {
	return e.err
}

func (e sanitizedError) Cause() error {
	return e.err
}

// rpcURLRegexp matches full HTTP(S)/WS(S) URLs so provider URLs, paths, query params, credentials, and API keys can be redacted from errors.
var rpcURLRegexp = regexp.MustCompile(`(?i)(?:https?|wss?)://[^\s"']+`)

// SanitizeRPCError returns an error whose message has all URLs replaced with "[REDACTED URL]".
// The original error remains reachable via errors.Is/errors.As and pkgerrors.Cause.
// Returns nil if err is nil.
func SanitizeRPCError(err error) error {
	if err == nil {
		return nil
	}
	return sanitizedError{
		err: err,
		msg: rpcURLRegexp.ReplaceAllString(err.Error(), "[REDACTED URL]"),
	}
}
