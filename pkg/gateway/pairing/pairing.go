package pairing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/wandxy/morph/pkg/str"
)

const (
	DefaultPeriodSeconds = 30
	DefaultSkew          = 1
	DefaultDigits        = otp.DigitsEight
	DefaultRequestTTL    = time.Hour
	DefaultPendingLimit  = 100
)

var (
	ErrSecretRequired = errors.New("gateway pairing secret is required")
	ErrAmbiguousCode  = errors.New("gateway pairing code matches multiple pending requests")
	ErrPendingLimit   = errors.New("gateway pairing pending request limit reached")
)

type Identity struct {
	Source      string
	SenderID    string
	DisplayName string
	Metadata    map[string]string
}

type PendingRequest struct {
	CreatedAt   time.Time
	LastSeenAt  time.Time
	ExpiresAt   time.Time
	Source      string
	SenderID    string
	DisplayName string
	Metadata    map[string]string
}

type ApprovedSender struct {
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Source      string
	SenderID    string
	DisplayName string
	Metadata    map[string]string
}

type Challenge struct {
	Request PendingRequest
	Code    string
}

type Store interface {
	SaveGatewayPairingRequest(context.Context, PendingRequest) error
	GetGatewayPairingRequest(context.Context, string, string) (PendingRequest, bool, error)
	ListGatewayPairingRequests(context.Context, string) ([]PendingRequest, error)
	DeleteGatewayPairingRequest(context.Context, string, string) error
	ClearGatewayPairingRequests(context.Context, string) error
	SaveGatewayPairedSender(context.Context, ApprovedSender) error
	GetGatewayPairedSender(context.Context, string, string) (ApprovedSender, bool, error)
	ListGatewayPairedSenders(context.Context, string) ([]ApprovedSender, error)
	DeleteGatewayPairedSender(context.Context, string, string) error
}

type Manager struct {
	store        Store
	secret       string
	period       uint
	skew         uint
	digits       otp.Digits
	requestTTL   time.Duration
	pendingLimit int
	now          func() time.Time
	verifyCode   func(string, string, string, time.Time) (bool, error)
}

type Options struct {
	Store        Store
	Secret       string
	Period       time.Duration
	Skew         uint
	Digits       otp.Digits
	RequestTTL   time.Duration
	PendingLimit int
	Now          func() time.Time
}

func NewManager(opts Options) *Manager {
	stringValue1 := str.String(opts.Secret)
	manager := &Manager{
		store:        opts.Store,
		secret:       stringValue1.Trim(),
		period:       DefaultPeriodSeconds,
		skew:         DefaultSkew,
		digits:       DefaultDigits,
		requestTTL:   DefaultRequestTTL,
		pendingLimit: DefaultPendingLimit,
		now:          func() time.Time { return time.Now().UTC() },
	}
	if opts.Period > 0 {
		manager.period = uint(opts.Period.Seconds())
	}
	if opts.Skew != 0 {
		manager.skew = opts.Skew
	}
	if opts.Digits != 0 {
		manager.digits = opts.Digits
	}
	if opts.RequestTTL > 0 {
		manager.requestTTL = opts.RequestTTL
	}
	if opts.PendingLimit > 0 {
		manager.pendingLimit = opts.PendingLimit
	}
	if opts.Now != nil {
		manager.now = opts.Now
	}

	return manager
}

func (m *Manager) Request(ctx context.Context, identity Identity) (Challenge, error) {
	if err := m.checkReady(); err != nil {
		return Challenge{}, err
	}

	identity = normalizeIdentity(identity)
	if identity.Source == "" {
		return Challenge{}, errors.New("gateway pairing source is required")
	}
	if identity.SenderID == "" {
		return Challenge{}, errors.New("gateway pairing sender id is required")
	}

	now := m.now().UTC()
	existing, ok, err := m.store.GetGatewayPairingRequest(ctx, identity.Source, identity.SenderID)
	if err != nil {
		return Challenge{}, err
	}
	if ok && existing.ExpiresAt.After(now) {
		existing.LastSeenAt = now
		existing.DisplayName = identity.DisplayName
		existing.Metadata = cloneMap(identity.Metadata)
		if err := m.store.SaveGatewayPairingRequest(ctx, existing); err != nil {
			return Challenge{}, err
		}
		code, err := m.Code(identity.Source, identity.SenderID, now)
		return Challenge{Request: existing, Code: code}, err
	}

	requests, err := m.store.ListGatewayPairingRequests(ctx, identity.Source)
	if err != nil {
		return Challenge{}, err
	}
	active := 0
	for _, request := range requests {
		if request.ExpiresAt.After(now) {
			active++
		}
	}
	if active >= m.pendingLimit {
		return Challenge{}, ErrPendingLimit
	}

	request := PendingRequest{
		Source:      identity.Source,
		SenderID:    identity.SenderID,
		DisplayName: identity.DisplayName,
		Metadata:    cloneMap(identity.Metadata),
		CreatedAt:   now,
		LastSeenAt:  now,
		ExpiresAt:   now.Add(m.requestTTL),
	}
	if err := m.store.SaveGatewayPairingRequest(ctx, request); err != nil {
		return Challenge{}, err
	}
	code, err := m.Code(identity.Source, identity.SenderID, now)
	return Challenge{Request: request, Code: code}, err
}

func (m *Manager) Code(source string, senderID string, at time.Time) (string, error) {
	if err := m.checkReady(); err != nil {
		return "", err
	}
	stringValue2 := str.String(senderID)
	secret := deriveTOTPSecret(m.secret, normalizeSource(source), stringValue2.Trim())
	return totp.GenerateCodeCustom(secret, at.UTC(), m.validateOpts())
}

func (m *Manager) Verify(source string, senderID string, code string, at time.Time) (bool, error) {
	if err := m.checkReady(); err != nil {
		return false, err
	}
	if m.verifyCode != nil {
		return m.verifyCode(source, senderID, code, at)
	}
	stringValue3 := str.String(senderID)
	secret := deriveTOTPSecret(m.secret, normalizeSource(source), stringValue3.Trim())
	stringValue4 := str.String(code)
	return totp.ValidateCustom(stringValue4.Trim(), secret, at.UTC(), m.validateOpts())
}

func (m *Manager) Approve(ctx context.Context, source string, code string) (ApprovedSender, bool, error) {
	if err := m.checkReady(); err != nil {
		return ApprovedSender{}, false, err
	}

	source = normalizeSource(source)
	stringValue5 := str.String(code)
	code = stringValue5.Trim()
	if source == "" {
		return ApprovedSender{}, false, errors.New("gateway pairing source is required")
	}
	if code == "" {
		return ApprovedSender{}, false, nil
	}

	now := m.now().UTC()
	requests, err := m.store.ListGatewayPairingRequests(ctx, source)
	if err != nil {
		return ApprovedSender{}, false, err
	}

	var matches []PendingRequest
	for _, request := range requests {
		if !request.ExpiresAt.After(now) {
			continue
		}
		ok, err := m.Verify(source, request.SenderID, code, now)
		if err != nil {
			return ApprovedSender{}, false, err
		}
		if ok {
			matches = append(matches, request)
		}
	}
	if len(matches) == 0 {
		return ApprovedSender{}, false, nil
	}
	if len(matches) > 1 {
		return ApprovedSender{}, false, ErrAmbiguousCode
	}

	request := matches[0]
	approved := ApprovedSender{
		Source:      request.Source,
		SenderID:    request.SenderID,
		DisplayName: request.DisplayName,
		Metadata:    cloneMap(request.Metadata),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := m.store.SaveGatewayPairedSender(ctx, approved); err != nil {
		return ApprovedSender{}, false, err
	}
	if err := m.store.DeleteGatewayPairingRequest(ctx, source, request.SenderID); err != nil {
		return ApprovedSender{}, false, err
	}

	return approved, true, nil
}

func (m *Manager) Revoke(ctx context.Context, source string, senderID string) error {
	if m == nil || m.store == nil {
		return errors.New("gateway pairing store is required")
	}
	stringValue6 := str.String(senderID)
	return m.store.DeleteGatewayPairedSender(ctx, normalizeSource(source), stringValue6.Trim())
}

func (m *Manager) IsApproved(ctx context.Context, source string, senderID string) (bool, error) {
	if m == nil || m.store == nil {
		return false, errors.New("gateway pairing store is required")
	}
	stringValue7 := str.String(senderID)
	_, ok, err := m.store.GetGatewayPairedSender(ctx, normalizeSource(source), stringValue7.Trim())
	return ok, err
}

func (m *Manager) validateOpts() totp.ValidateOpts {
	return totp.ValidateOpts{
		Period:    m.period,
		Skew:      m.skew,
		Digits:    m.digits,
		Algorithm: otp.AlgorithmSHA1,
	}
}

func (m *Manager) checkReady() error {
	if m == nil || m.store == nil {
		return errors.New("gateway pairing store is required")
	}
	stringValue8 := str.String(m.secret)
	if stringValue8.Trim() == "" {
		return ErrSecretRequired
	}

	return nil
}

func normalizeIdentity(identity Identity) Identity {
	stringValue9 := str.String(identity.SenderID)
	stringValue10 := str.String(identity.DisplayName)
	return Identity{
		Source:      normalizeSource(identity.Source),
		SenderID:    stringValue9.Trim(),
		DisplayName: stringValue10.Trim(),
		Metadata:    cloneMap(identity.Metadata),
	}
}

func normalizeSource(source string) string {
	stringValue11 := str.String(source)
	return stringValue11.Normalized()
}

func deriveTOTPSecret(secret string, source string, senderID string) string {
	stringValue12 := str.String(secret)
	mac := hmac.New(sha256.New, []byte(stringValue12.Trim()))
	mac.Write([]byte(source))
	mac.Write([]byte{0})
	stringValue13 := str.String(senderID)
	mac.Write([]byte(stringValue13.Trim()))
	sum := mac.Sum(nil)

	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum)
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	clone := make(map[string]string, len(values))
	for key, value := range values {
		stringValue14 := str.String(key)
		key = stringValue14.Trim()
		if key == "" {
			continue
		}
		stringValue15 := str.String(value)
		clone[key] = stringValue15.Trim()
	}
	if len(clone) == 0 {
		return nil
	}

	return clone
}
