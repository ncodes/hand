package slack

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSignatureVerifier_CheckAcceptsValidSignature(t *testing.T) {
	now := time.Unix(1710000000, 0)
	body := []byte(`{"type":"event_callback"}`)
	signature := SignRequest("secret", "1710000000", body)

	err := SignatureVerifier{
		Secret: " secret ",
		Now:    func() time.Time { return now },
	}.Check("1710000000", signature, body)

	require.NoError(t, err)
}

func TestSignatureVerifier_CheckRejectsInvalidRequests(t *testing.T) {
	now := time.Unix(1710000000, 0)
	body := []byte(`{"type":"event_callback"}`)
	signature := SignRequest("secret", "1710000000", body)

	tests := []struct {
		name      string
		verifier  SignatureVerifier
		timestamp string
		signature string
		want      error
	}{
		{
			name:      "secret required",
			verifier:  SignatureVerifier{Now: func() time.Time { return now }},
			timestamp: "1710000000",
			signature: signature,
			want:      ErrSigningSecretRequired,
		},
		{
			name:      "signature required",
			verifier:  SignatureVerifier{Secret: "secret", Now: func() time.Time { return now }},
			timestamp: "1710000000",
			want:      ErrSignatureMissing,
		},
		{
			name:      "timestamp required",
			verifier:  SignatureVerifier{Secret: "secret", Now: func() time.Time { return now }},
			signature: signature,
			want:      ErrTimestampMissing,
		},
		{
			name:      "timestamp invalid",
			verifier:  SignatureVerifier{Secret: "secret", Now: func() time.Time { return now }},
			timestamp: "nope",
			signature: signature,
			want:      ErrTimestampInvalid,
		},
		{
			name:      "timestamp stale",
			verifier:  SignatureVerifier{Secret: "secret", Now: func() time.Time { return now.Add(10 * time.Minute) }},
			timestamp: "1710000000",
			signature: signature,
			want:      ErrTimestampStale,
		},
		{
			name:      "signature mismatch",
			verifier:  SignatureVerifier{Secret: "secret", Now: func() time.Time { return now }},
			timestamp: "1710000000",
			signature: SignRequest("other", "1710000000", body),
			want:      ErrSignatureMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.verifier.Check(tt.timestamp, tt.signature, body)

			require.ErrorIs(t, err, tt.want)
		})
	}
}
