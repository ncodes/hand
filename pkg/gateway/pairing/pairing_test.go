package pairing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/stretchr/testify/require"
)

func TestManager_RequestCreatesTOTPChallengeAndApprovePairsSender(t *testing.T) {
	store := newMemoryStore()
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	manager := NewManager(Options{Store: store, Secret: "secret", Now: func() time.Time { return now }})

	challenge, err := manager.Request(context.Background(), Identity{
		Source:      " Telegram ",
		SenderID:    " 123 ",
		DisplayName: " Ada ",
		Metadata:    map[string]string{" chat ": " private ", " ": "ignored"},
	})

	require.NoError(t, err)
	require.Len(t, challenge.Code, 8)
	require.Equal(t, "telegram", challenge.Request.Source)
	require.Equal(t, "123", challenge.Request.SenderID)
	require.Equal(t, map[string]string{"chat": "private"}, challenge.Request.Metadata)
	require.Equal(t, now.Add(DefaultRequestTTL), challenge.Request.ExpiresAt)

	sender, ok, err := manager.Approve(context.Background(), "telegram", challenge.Code)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, ApprovedSender{
		Source:      "telegram",
		SenderID:    "123",
		DisplayName: "Ada",
		Metadata:    map[string]string{"chat": "private"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}, sender)

	_, pending, err := store.GetGatewayPairingRequest(context.Background(), "telegram", "123")
	require.NoError(t, err)
	require.False(t, pending)
	approved, err := manager.IsApproved(context.Background(), "telegram", "123")
	require.NoError(t, err)
	require.True(t, approved)
}

func TestManager_NewManagerAppliesOptionOverrides(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	manager := NewManager(Options{
		Store:        newMemoryStore(),
		Secret:       " secret ",
		Period:       time.Minute,
		Skew:         2,
		Digits:       otp.DigitsSix,
		RequestTTL:   2 * time.Hour,
		PendingLimit: 7,
		Now:          func() time.Time { return now },
	})

	require.Equal(t, "secret", manager.secret)
	require.Equal(t, uint(60), manager.period)
	require.Equal(t, uint(2), manager.skew)
	require.Equal(t, otp.DigitsSix, manager.digits)
	require.Equal(t, 2*time.Hour, manager.requestTTL)
	require.Equal(t, 7, manager.pendingLimit)
	require.Equal(t, now, manager.now())
}

func TestManager_RevokeDeletesApprovedSender(t *testing.T) {
	store := newMemoryStore()
	manager := NewManager(Options{Store: store, Secret: "secret"})
	require.NoError(t, store.SaveGatewayPairedSender(context.Background(), ApprovedSender{
		Source:   "telegram",
		SenderID: "123",
	}))

	require.NoError(t, manager.Revoke(context.Background(), " Telegram ", " 123 "))

	approved, err := manager.IsApproved(context.Background(), "telegram", "123")
	require.NoError(t, err)
	require.False(t, approved)
}

func TestManager_RequestValidationAndStoreErrors(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(Options{Store: newMemoryStore(), Secret: "secret"})

	_, err := manager.Request(ctx, Identity{SenderID: "123"})
	require.EqualError(t, err, "gateway pairing source is required")

	_, err = manager.Request(ctx, Identity{Source: "telegram"})
	require.EqualError(t, err, "gateway pairing sender id is required")

	getErr := errors.New("get pending failed")
	manager = NewManager(Options{Store: &memoryStore{pending: map[string]PendingRequest{}, approved: map[string]ApprovedSender{}, getPendingErr: getErr}, Secret: "secret"})
	_, err = manager.Request(ctx, Identity{Source: "telegram", SenderID: "123"})
	require.ErrorIs(t, err, getErr)

	listErr := errors.New("list pending failed")
	manager = NewManager(Options{Store: &memoryStore{pending: map[string]PendingRequest{}, approved: map[string]ApprovedSender{}, listPendingErr: listErr}, Secret: "secret"})
	_, err = manager.Request(ctx, Identity{Source: "telegram", SenderID: "123"})
	require.ErrorIs(t, err, listErr)

	saveErr := errors.New("save pending failed")
	manager = NewManager(Options{Store: &memoryStore{pending: map[string]PendingRequest{}, approved: map[string]ApprovedSender{}, savePendingErr: saveErr}, Secret: "secret"})
	_, err = manager.Request(ctx, Identity{Source: "telegram", SenderID: "123"})
	require.ErrorIs(t, err, saveErr)
}

func TestManager_RequestReusesPendingRequestAndRefreshesLastSeen(t *testing.T) {
	store := newMemoryStore()
	first := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	now := first
	manager := NewManager(Options{Store: store, Secret: "secret", Now: func() time.Time { return now }})
	_, err := manager.Request(context.Background(), Identity{Source: "telegram", SenderID: "123"})
	require.NoError(t, err)
	now = first.Add(time.Minute)

	challenge, err := manager.Request(context.Background(), Identity{Source: "telegram", SenderID: "123"})

	require.NoError(t, err)
	require.Equal(t, first, challenge.Request.CreatedAt)
	require.Equal(t, now, challenge.Request.LastSeenAt)
	requests, err := store.ListGatewayPairingRequests(context.Background(), "telegram")
	require.NoError(t, err)
	require.Len(t, requests, 1)
}

func TestManager_RequestReturnsExistingSaveError(t *testing.T) {
	store := newMemoryStore()
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	manager := NewManager(Options{Store: store, Secret: "secret", Now: func() time.Time { return now }})
	_, err := manager.Request(context.Background(), Identity{Source: "telegram", SenderID: "123"})
	require.NoError(t, err)
	saveErr := errors.New("refresh pending failed")
	store.savePendingErr = saveErr

	_, err = manager.Request(context.Background(), Identity{Source: "telegram", SenderID: "123"})

	require.ErrorIs(t, err, saveErr)
}

func TestManager_ApproveRejectsExpiredAndAmbiguousCodes(t *testing.T) {
	store := newMemoryStore()
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	manager := NewManager(Options{Store: store, Secret: "secret", RequestTTL: time.Minute, Now: func() time.Time { return now }})
	expired, err := manager.Request(context.Background(), Identity{Source: "telegram", SenderID: "expired"})
	require.NoError(t, err)
	now = now.Add(2 * time.Minute)
	sender, ok, err := manager.Approve(context.Background(), "telegram", expired.Code)
	require.NoError(t, err)
	require.False(t, ok)
	require.Zero(t, sender)

	now = time.Date(2026, 6, 8, 13, 0, 0, 0, time.UTC)
	ambiguous := "11111111"
	manager.verifyCode = func(_ string, _ string, code string, _ time.Time) (bool, error) {
		return code == ambiguous, nil
	}
	require.NoError(t, store.SaveGatewayPairingRequest(context.Background(), PendingRequest{
		Source:    "telegram",
		SenderID:  "a",
		ExpiresAt: now.Add(time.Minute),
	}))
	require.NoError(t, store.SaveGatewayPairingRequest(context.Background(), PendingRequest{
		Source:    "telegram",
		SenderID:  "b",
		ExpiresAt: now.Add(time.Minute),
	}))
	_, _, err = manager.Approve(context.Background(), "telegram", ambiguous)
	require.ErrorIs(t, err, ErrAmbiguousCode)
}

func TestManager_CodeAndVerifyValidateReadiness(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)

	_, err := (*Manager)(nil).Code("telegram", "123", now)
	require.EqualError(t, err, "gateway pairing store is required")

	ok, err := NewManager(Options{Store: newMemoryStore()}).Verify("telegram", "123", "12345678", now)
	require.False(t, ok)
	require.ErrorIs(t, err, ErrSecretRequired)
}

func TestManager_VerifyUsesCustomVerifier(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	manager := NewManager(Options{Store: newMemoryStore(), Secret: "secret"})
	manager.verifyCode = func(source string, senderID string, code string, at time.Time) (bool, error) {
		require.Equal(t, "telegram", source)
		require.Equal(t, "123", senderID)
		require.Equal(t, "11111111", code)
		require.Equal(t, now, at)
		return true, nil
	}

	ok, err := manager.Verify("telegram", "123", "11111111", now)

	require.NoError(t, err)
	require.True(t, ok)
}

func TestManager_ApproveValidationAndStoreErrors(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)

	_, ok, err := (*Manager)(nil).Approve(ctx, "telegram", "12345678")
	require.False(t, ok)
	require.EqualError(t, err, "gateway pairing store is required")

	manager := NewManager(Options{Store: newMemoryStore(), Secret: "secret"})
	_, ok, err = manager.Approve(ctx, "", "12345678")
	require.False(t, ok)
	require.EqualError(t, err, "gateway pairing source is required")

	sender, ok, err := manager.Approve(ctx, "telegram", "")
	require.NoError(t, err)
	require.False(t, ok)
	require.Zero(t, sender)

	listErr := errors.New("list pending failed")
	manager = NewManager(Options{Store: &memoryStore{pending: map[string]PendingRequest{}, approved: map[string]ApprovedSender{}, listPendingErr: listErr}, Secret: "secret"})
	_, ok, err = manager.Approve(ctx, "telegram", "12345678")
	require.False(t, ok)
	require.ErrorIs(t, err, listErr)

	verifyErr := errors.New("verify failed")
	store := newMemoryStore()
	require.NoError(t, store.SaveGatewayPairingRequest(ctx, PendingRequest{
		Source:    "telegram",
		SenderID:  "123",
		ExpiresAt: now.Add(time.Minute),
	}))
	manager = NewManager(Options{Store: store, Secret: "secret", Now: func() time.Time { return now }})
	manager.verifyCode = func(string, string, string, time.Time) (bool, error) {
		return false, verifyErr
	}
	_, ok, err = manager.Approve(ctx, "telegram", "12345678")
	require.False(t, ok)
	require.ErrorIs(t, err, verifyErr)

	store = newMemoryStore()
	require.NoError(t, store.SaveGatewayPairingRequest(ctx, PendingRequest{
		Source:    "telegram",
		SenderID:  "123",
		ExpiresAt: now.Add(time.Minute),
	}))
	saveErr := errors.New("save approved failed")
	store.saveApprovedErr = saveErr
	manager = NewManager(Options{Store: store, Secret: "secret", Now: func() time.Time { return now }})
	manager.verifyCode = func(string, string, string, time.Time) (bool, error) { return true, nil }
	_, ok, err = manager.Approve(ctx, "telegram", "12345678")
	require.False(t, ok)
	require.ErrorIs(t, err, saveErr)

	store = newMemoryStore()
	require.NoError(t, store.SaveGatewayPairingRequest(ctx, PendingRequest{
		Source:    "telegram",
		SenderID:  "123",
		ExpiresAt: now.Add(time.Minute),
	}))
	deleteErr := errors.New("delete pending failed")
	store.deletePendingErr = deleteErr
	manager = NewManager(Options{Store: store, Secret: "secret", Now: func() time.Time { return now }})
	manager.verifyCode = func(string, string, string, time.Time) (bool, error) { return true, nil }
	_, ok, err = manager.Approve(ctx, "telegram", "12345678")
	require.False(t, ok)
	require.ErrorIs(t, err, deleteErr)
}

func TestManager_RequestEnforcesPendingLimit(t *testing.T) {
	store := newMemoryStore()
	manager := NewManager(Options{Store: store, Secret: "secret", PendingLimit: 1})
	_, err := manager.Request(context.Background(), Identity{Source: "telegram", SenderID: "1"})
	require.NoError(t, err)

	_, err = manager.Request(context.Background(), Identity{Source: "telegram", SenderID: "2"})

	require.ErrorIs(t, err, ErrPendingLimit)
}

func TestManager_RejectsMissingSecret(t *testing.T) {
	manager := NewManager(Options{Store: newMemoryStore()})

	_, err := manager.Request(context.Background(), Identity{Source: "telegram", SenderID: "1"})

	require.ErrorIs(t, err, ErrSecretRequired)
}

func TestManager_RejectsMissingStore(t *testing.T) {
	err := (*Manager)(nil).Revoke(context.Background(), "telegram", "123")

	require.EqualError(t, err, "gateway pairing store is required")
}

func TestManager_IsApprovedValidationAndStoreErrors(t *testing.T) {
	approved, err := (*Manager)(nil).IsApproved(context.Background(), "telegram", "123")
	require.False(t, approved)
	require.EqualError(t, err, "gateway pairing store is required")

	storeErr := errors.New("get approved failed")
	approved, err = NewManager(Options{
		Store:  &memoryStore{pending: map[string]PendingRequest{}, approved: map[string]ApprovedSender{}, getApprovedErr: storeErr},
		Secret: "secret",
	}).IsApproved(context.Background(), "telegram", "123")
	require.False(t, approved)
	require.ErrorIs(t, err, storeErr)
}

func TestCloneMapDropsBlankKeys(t *testing.T) {
	require.Nil(t, cloneMap(map[string]string{" ": "ignored"}))
}
