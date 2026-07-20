package rpcauth

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	morphpb "github.com/wandxy/morph/internal/rpc/proto"
	"google.golang.org/grpc/metadata"
)

func TestValidator_ValidatesOneUseMethodBoundProof(t *testing.T) {
	credential := []byte("01234567890123456789012345678901")
	incoming := getIncomingProof(t, "/morph.Browser/Start", credential)
	validator := NewValidator(credential)

	require.NoError(t, validator.Validate(incoming, "/morph.Browser/Start", "request"))
	require.EqualError(
		t, validator.Validate(incoming, "/morph.Browser/Start", "request"), "RPC owner proof was already used",
	)

	wrongMethod := getIncomingProof(t, "/morph.Browser/Start", credential)
	require.EqualError(t, validator.Validate(wrongMethod, "/morph.Browser/Stop", "request"), "RPC owner proof is invalid")

	wrongRequest := getIncomingProof(t, "/morph.Browser/Start", credential)
	require.EqualError(
		t, validator.Validate(wrongRequest, "/morph.Browser/Start", "other"),
		"RPC owner proof request identity is invalid; verify the client and daemon versions match",
	)
}

func TestRequestDigest_UsesDeterministicProtobufEncoding(t *testing.T) {
	first := &morphpb.StartBrowserRequest{Profile: "default", OwnerSessionId: "session"}
	second := &morphpb.StartBrowserRequest{OwnerSessionId: "session", Profile: "default"}

	firstDigest, err := getRequestDigest(first)
	require.NoError(t, err)
	secondDigest, err := getRequestDigest(second)
	require.NoError(t, err)
	require.Equal(t, firstDigest, secondDigest)
}

func TestValidator_RejectsMissingMalformedExpiredAndFutureProofs(t *testing.T) {
	credential := []byte("01234567890123456789012345678901")
	validator := NewValidator(credential)
	require.EqualError(t, validator.Validate(context.Background(), "method", nil), "RPC owner proof is incomplete")

	requestDigest, err := getRequestDigest(nil)
	require.NoError(t, err)
	malformed := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		timestampKey, "not-a-time", nonceKey, "nonce", requestKey, requestDigest, signatureKey, "signature",
	))
	require.EqualError(t, validator.Validate(malformed, "method", nil), "RPC owner proof timestamp is invalid")

	proof := getIncomingProof(t, "method", credential)
	validator.now = func() time.Time { return time.Now().UTC().Add(2 * defaultProofWindow) }
	require.EqualError(t, validator.Validate(proof, "method", "request"), "RPC owner proof has expired")

	validator.now = func() time.Time { return time.Now().UTC().Add(-2 * defaultProofWindow) }
	require.EqualError(t, validator.Validate(proof, "method", "request"), "RPC owner proof has expired")
}

func TestValidator_RejectsUnavailableValidatorAndDuplicateMetadata(t *testing.T) {
	proof := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		timestampKey, "1", timestampKey, "2", nonceKey, "nonce", signatureKey, "signature",
	))
	require.EqualError(t, (*Validator)(nil).Validate(proof, "method", nil), "RPC owner authentication is unavailable")
	require.EqualError(t, NewValidator(nil).Validate(proof, "method", nil), "RPC owner authentication is unavailable")

	validator := NewValidator([]byte("credential"))
	require.EqualError(t, validator.Validate(proof, "method", nil), "RPC owner proof is incomplete")
}

func TestWithOutgoingProof_HandlesEmptyCredentialAndValidatesInput(t *testing.T) {
	ctx := context.Background()
	actual, err := WithOutgoingProof(ctx, "", nil, nil)
	require.NoError(t, err)
	require.Equal(t, ctx, actual)
	require.False(t, HasIncomingProof(ctx))

	_, err = WithOutgoingProof(ctx, " ", []byte("credential"), nil)
	require.EqualError(t, err, "RPC method is required")
	_, err = WithOutgoingProof(ctx, "method", []byte("credential"), make(chan int))
	require.EqualError(t, err, "RPC owner proof request identity is invalid")
	var emptyContext context.Context
	actual, err = WithOutgoingProof(emptyContext, "method", []byte("credential"), nil)
	require.NoError(t, err)
	require.NotNil(t, actual)
	require.False(t, HasIncomingProof(emptyContext))
}

func TestValidator_PrunesExpiredReplayEntries(t *testing.T) {
	validator := NewValidator([]byte("credential"))
	now := time.Now().UTC()
	validator.nonces["old"] = now.Add(-2 * validator.window)
	require.True(t, validator.reserveNonce("new", now))
	_, retained := validator.nonces["old"]
	require.False(t, retained)
	require.False(t, validator.reserveNonce("new", now))
}

func getIncomingProof(t *testing.T, method string, credential []byte) context.Context {
	t.Helper()
	outgoing, err := WithOutgoingProof(context.Background(), method, credential, "request")
	require.NoError(t, err)
	values, ok := metadata.FromOutgoingContext(outgoing)
	require.True(t, ok)
	return metadata.NewIncomingContext(context.Background(), values)
}

func TestSignature_IsStableAndURLSafe(t *testing.T) {
	first := getSignature([]byte("credential"), "method", "123", "nonce", "request")
	second := getSignature([]byte("credential"), "method", "123", "nonce", "request")
	require.Equal(t, first, second)
	require.NotContains(t, first, "=")
	decoded, err := base64.RawURLEncoding.DecodeString(first)
	require.NoError(t, err)
	require.Len(t, decoded, 32)
	require.False(t, strings.ContainsAny(first, "+/"))
}
