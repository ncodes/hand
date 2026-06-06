package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckBearerAllowsRequestsWhenTokenIsNotConfigured(t *testing.T) {
	require.NoError(t, CheckBearer("", " "))
}

func TestCheckBearerAcceptsMatchingBearerToken(t *testing.T) {
	require.NoError(t, CheckBearer("Bearer secret-token", "secret-token"))
	require.NoError(t, CheckBearer("bearer secret-token", "secret-token"))
	require.NoError(t, CheckBearer(" Bearer secret-token ", "secret-token"))
}

func TestCheckBearerRejectsMissingMalformedAndInvalidTokens(t *testing.T) {
	require.ErrorIs(t, CheckBearer("", "secret-token"), ErrBearerTokenMissing)
	require.ErrorIs(t, CheckBearer("Basic secret-token", "secret-token"), ErrBearerTokenMissing)
	require.ErrorIs(t, CheckBearer("Bearer ", "secret-token"), ErrBearerTokenMissing)
	require.ErrorIs(t, CheckBearer("Bearer secret token", "secret-token"), ErrBearerTokenMissing)
	require.ErrorIs(t, CheckBearer("Bearer secret\ttoken", "secret-token"), ErrBearerTokenMissing)
	require.ErrorIs(t, CheckBearer("Bearer wrong-token", "secret-token"), ErrBearerTokenInvalid)
}
