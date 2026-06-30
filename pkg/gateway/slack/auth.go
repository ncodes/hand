package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/wandxy/morph/pkg/stringx"
)

const (
	SignatureHeader           = "X-Slack-Signature"
	TimestampHeader           = "X-Slack-Request-Timestamp"
	signatureVersion          = "v0"
	defaultSignatureTolerance = 5 * time.Minute
)

var (
	ErrSigningSecretRequired = errors.New("slack signing secret is required")
	ErrSignatureMissing      = errors.New("slack signature is required")
	ErrTimestampMissing      = errors.New("slack request timestamp is required")
	ErrTimestampInvalid      = errors.New("slack request timestamp is invalid")
	ErrTimestampStale        = errors.New("slack request timestamp is stale")
	ErrSignatureMismatch     = errors.New("slack signature mismatch")
)

type SignatureVerifier struct {
	Secret    string
	Now       func() time.Time
	Tolerance time.Duration
}

func (v SignatureVerifier) Check(timestamp string, signature string, body []byte) error {
	secret := stringx.String(v.Secret).Trim()
	if secret == "" {
		return ErrSigningSecretRequired
	}
	signature = stringx.String(signature).Trim()
	if signature == "" {
		return ErrSignatureMissing
	}
	timestamp = stringx.String(timestamp).Trim()
	if timestamp == "" {
		return ErrTimestampMissing
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrTimestampInvalid
	}

	now := time.Now().UTC()
	if v.Now != nil {
		now = v.Now().UTC()
	}
	tolerance := v.Tolerance
	if tolerance <= 0 {
		tolerance = defaultSignatureTolerance
	}
	requestTime := time.Unix(seconds, 0).UTC()
	if now.Sub(requestTime) > tolerance || requestTime.Sub(now) > tolerance {
		return ErrTimestampStale
	}

	expected := SignRequest(secret, timestamp, body)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return ErrSignatureMismatch
	}

	return nil
}

func SignRequest(secret string, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(stringx.String(secret).Trim()))
	mac.Write([]byte(fmt.Sprintf("%s:%s:", signatureVersion, stringx.String(timestamp).Trim())))
	mac.Write(body)
	return signatureVersion + "=" + hex.EncodeToString(mac.Sum(nil))
}
