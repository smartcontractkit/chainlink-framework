package multinode

import (
	"context"
	"fmt"
	"testing"

	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestSanitizeRPCError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "https url with path",
			err:  fmt.Errorf(`Post "https://test-cl-03.test.xyz/dOnOtLeAkAnyOfThis/doNotLeak/test/mainnet/": %w`, context.DeadlineExceeded),
			want: `Post "[REDACTED URL]": context deadline exceeded`,
		},
		{
			name: "http url with path",
			err:  fmt.Errorf(`Post "http://secret.com/doNotLeak/": %w`, context.DeadlineExceeded),
			want: `Post "[REDACTED URL]": context deadline exceeded`,
		},
		{
			name: "ws url with path",
			err:  fmt.Errorf(`dial "ws://secret.com/doNotLeak/": %w`, context.DeadlineExceeded),
			want: `dial "[REDACTED URL]": context deadline exceeded`,
		},
		{
			name: "wss url with path",
			err:  fmt.Errorf(`dial "wss://secret.com/doNotLeak/": %w`, context.DeadlineExceeded),
			want: `dial "[REDACTED URL]": context deadline exceeded`,
		},
		{
			name: "https url with user info",
			err:  fmt.Errorf(`Post "https://user:pass@secret.com/": %w`, context.DeadlineExceeded),
			want: `Post "[REDACTED URL]": context deadline exceeded`,
		},
		{
			name: "multiple urls",
			err:  fmt.Errorf(`primary http://secret.com/doNotLeak/ fallback wss://secret.com/doNotLeak/ failed: %w`, context.DeadlineExceeded),
			want: `primary [REDACTED URL] fallback [REDACTED URL] failed: context deadline exceeded`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sanitized := SanitizeRPCError(tt.err)

			require.EqualError(t, sanitized, tt.want)
			require.NotContains(t, sanitized.Error(), "doNotLeak")
			require.NotContains(t, sanitized.Error(), "user:pass")
			require.NotContains(t, sanitized.Error(), "secret.com")
			require.ErrorIs(t, sanitized, context.DeadlineExceeded)
			require.Equal(t, tt.err, pkgerrors.Cause(sanitized))
		})
	}
}

func TestSanitizeRPCError_Nil(t *testing.T) {
	t.Parallel()
	require.NoError(t, SanitizeRPCError(nil))
}
