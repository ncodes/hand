package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
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
	secret := strings.TrimSpace(v.Secret)
	if secret == "" {
		return ErrSigningSecretRequired
	}
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return ErrSignatureMissing
	}
	timestamp = strings.TrimSpace(timestamp)
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
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(secret)))
	mac.Write([]byte(fmt.Sprintf("%s:%s:", signatureVersion, strings.TrimSpace(timestamp))))
	mac.Write(body)
	return signatureVersion + "=" + hex.EncodeToString(mac.Sum(nil))
}
