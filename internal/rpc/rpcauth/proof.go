package rpcauth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

const (
	timestampKey       = "x-morph-owner-timestamp"
	nonceKey           = "x-morph-owner-nonce"
	signatureKey       = "x-morph-owner-signature"
	requestKey         = "x-morph-owner-request"
	defaultProofWindow = 30 * time.Second
)

type Validator struct {
	credential []byte
	now        func() time.Time
	window     time.Duration
	mu         sync.Mutex
	nonces     map[string]time.Time
}

func NewValidator(credential []byte) *Validator {
	return &Validator{
		credential: append([]byte(nil), credential...),
		now:        func() time.Time { return time.Now().UTC() },
		window:     defaultProofWindow,
		nonces:     make(map[string]time.Time),
	}
}

func WithOutgoingProof(
	ctx context.Context,
	method string,
	credential []byte,
	request any,
) (context.Context, error) {
	if len(credential) == 0 {
		return ctx, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return nil, errors.New("RPC method is required")
	}
	nonceBytes := make([]byte, 18)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	requestDigest, err := getRequestDigest(request)
	if err != nil {
		return nil, err
	}
	signature := getSignature(credential, method, timestamp, nonce, requestDigest)

	return metadata.AppendToOutgoingContext(
		ctx,
		timestampKey,
		timestamp,
		nonceKey,
		nonce,
		requestKey,
		requestDigest,
		signatureKey,
		signature,
	), nil
}

func HasIncomingProof(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	values := metadata.ValueFromIncomingContext(ctx, signatureKey)
	return len(values) > 0
}

func (v *Validator) Validate(ctx context.Context, method string, request any) error {
	if v == nil || len(v.credential) == 0 {
		return errors.New("RPC owner authentication is unavailable")
	}
	if ctx == nil {
		return errors.New("RPC owner proof is required")
	}
	timestamp, err := getSingleMetadataValue(ctx, timestampKey)
	if err != nil {
		return err
	}
	nonce, err := getSingleMetadataValue(ctx, nonceKey)
	if err != nil {
		return err
	}
	signature, err := getSingleMetadataValue(ctx, signatureKey)
	if err != nil {
		return err
	}
	requestDigest, err := getSingleMetadataValue(ctx, requestKey)
	if err != nil {
		return err
	}
	expectedRequestDigest, err := getRequestDigest(request)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(expectedRequestDigest), []byte(requestDigest)) {
		return errors.New("RPC owner proof request identity is invalid; verify the client and daemon versions match")
	}
	unixTime, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("RPC owner proof timestamp is invalid")
	}
	now := v.now()
	createdAt := time.Unix(unixTime, 0).UTC()
	if createdAt.Before(now.Add(-v.window)) || createdAt.After(now.Add(v.window)) {
		return errors.New("RPC owner proof has expired")
	}
	expected := getSignature(v.credential, strings.TrimSpace(method), timestamp, nonce, requestDigest)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return errors.New("RPC owner proof is invalid")
	}
	if !v.reserveNonce(nonce, now) {
		return errors.New("RPC owner proof was already used")
	}

	return nil
}

func (v *Validator) reserveNonce(nonce string, now time.Time) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	cutoff := now.Add(-v.window)
	for value, createdAt := range v.nonces {
		if createdAt.Before(cutoff) {
			delete(v.nonces, value)
		}
	}
	if _, exists := v.nonces[nonce]; exists {
		return false
	}
	v.nonces[nonce] = now

	return true
}

func getSignature(credential []byte, method, timestamp, nonce, requestDigest string) string {
	mac := hmac.New(sha256.New, credential)
	_, _ = mac.Write([]byte(method))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(nonce))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(requestDigest))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func getRequestDigest(request any) (string, error) {
	var encoded []byte
	var err error
	if message, ok := request.(proto.Message); ok {
		encoded, err = (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	} else {
		encoded, err = json.Marshal(request)
	}
	if err != nil {
		return "", errors.New("RPC owner proof request identity is invalid")
	}
	digest := sha256.Sum256(encoded)

	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func getSingleMetadataValue(ctx context.Context, key string) (string, error) {
	values := metadata.ValueFromIncomingContext(ctx, key)
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return "", errors.New("RPC owner proof is incomplete")
	}

	return strings.TrimSpace(values[0]), nil
}
