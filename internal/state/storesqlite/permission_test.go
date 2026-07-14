package storesqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/permissions"
)

func TestPermissionStore_PersistsPendingRequestAcrossReopenForRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := NewStore(path)
	require.NoError(t, err)
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	_, _, err = store.CreateApprovalRequest(context.Background(), permissions.ApprovalRequest{
		ID: "approval_restart", Fingerprint: "fingerprint", Status: permissions.ApprovalPending,
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.NoError(t, store.Close())

	reopened, err := NewStore(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	service, err := permissions.NewApprovalService(reopened, permissions.ApprovalOptions{Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.NoError(t, service.Recover(context.Background()))
	request, ok, err := reopened.GetApprovalRequest(context.Background(), "approval_restart")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, permissions.ApprovalCancelled, request.Status)
}

func TestPermissionStore_PersistsApprovalLifecycle(t *testing.T) {
	store := newAutomationSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	request := permissions.ApprovalRequest{
		ID: "approval_one", Fingerprint: "fingerprint", Actor: permissions.Actor{Kind: permissions.ActorLocalOwner, ID: "owner"},
		SurfaceKind: permissions.SurfaceKindLocal, Surface: permissions.SurfaceTUI, Profile: "default", SessionID: "session",
		Tool: "run_command", Resource: permissions.ResourceProcess, Action: permissions.ActionExecute,
		Effects: []permissions.Effect{permissions.EffectExecution}, Summary: "run command", Status: permissions.ApprovalPending,
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}

	created, inserted, err := store.CreateApprovalRequest(ctx, request)
	require.NoError(t, err)
	require.True(t, inserted)
	require.Equal(t, request, created)
	coalesced, inserted, err := store.CreateApprovalRequest(ctx, permissions.ApprovalRequest{
		ID: "approval_two", Fingerprint: request.Fingerprint, Actor: request.Actor, SessionID: request.SessionID,
	})
	require.NoError(t, err)
	require.False(t, inserted)
	require.Equal(t, request.ID, coalesced.ID)

	resolved, err := store.ResolveApprovalRequest(ctx, request.ID, permissions.ApprovalApproved, permissions.GrantOnce, now)
	require.NoError(t, err)
	require.Equal(t, permissions.ApprovalApproved, resolved.Status)
	grant := permissions.ApprovalGrant{
		ID: "grant_one", RequestID: request.ID, Fingerprint: request.Fingerprint, Actor: request.Actor,
		Profile: request.Profile, SessionID: request.SessionID, Scope: permissions.GrantOnce,
		Status: permissions.GrantActive, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	_, err = store.CreateApprovalGrant(ctx, grant)
	require.NoError(t, err)
	found, ok, err := store.FindApprovalGrant(ctx, request.Fingerprint, request.Actor, request.Profile, request.SessionID, now)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, grant.ID, found.ID)
	consumed, err := store.ConsumeApprovalGrant(ctx, grant.ID, now)
	require.NoError(t, err)
	require.Equal(t, permissions.GrantConsumed, consumed.Status)

	requests, err := store.ListApprovalRequests(ctx, permissions.ApprovalQuery{Status: permissions.ApprovalApproved})
	require.NoError(t, err)
	require.Len(t, requests, 1)
	grants, err := store.ListApprovalGrants(ctx, permissions.GrantQuery{Status: permissions.GrantConsumed})
	require.NoError(t, err)
	require.Len(t, grants, 1)
}

func TestPermissionStore_PersistsAlwaysGrantAcrossReopenAndSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := NewStore(path)
	require.NoError(t, err)
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	actor := permissions.Actor{Kind: permissions.ActorLocalOwner, ID: "owner"}
	request := permissions.ApprovalRequest{
		ID: "approval_always", Fingerprint: "exact-operation", Actor: actor, Profile: "default",
		SessionID: "session-a", Status: permissions.ApprovalPending, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	_, _, err = store.CreateApprovalRequest(context.Background(), request)
	require.NoError(t, err)
	_, err = store.ResolveApprovalRequest(
		context.Background(), request.ID, permissions.ApprovalApproved, permissions.GrantAlways, now,
	)
	require.NoError(t, err)
	_, err = store.CreateApprovalGrant(context.Background(), permissions.ApprovalGrant{
		ID: "grant_always", RequestID: request.ID, Fingerprint: request.Fingerprint, Actor: actor,
		Profile: request.Profile, SessionID: request.SessionID, Scope: permissions.GrantAlways,
		Status: permissions.GrantActive, CreatedAt: now,
	})
	require.NoError(t, err)
	require.NoError(t, store.Close())

	reopened, err := NewStore(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	found, ok, err := reopened.FindApprovalGrant(
		context.Background(), request.Fingerprint, actor, request.Profile, "session-b", now.Add(100*365*24*time.Hour),
	)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, permissions.GrantAlways, found.Scope)
	require.True(t, found.ExpiresAt.IsZero())

	legacyRequest := permissions.ApprovalRequest{
		ID: "approval_legacy", Fingerprint: "legacy-operation", Actor: actor, Profile: "default",
		SessionID: "session-a", Status: permissions.ApprovalPending, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	_, _, err = reopened.CreateApprovalRequest(context.Background(), legacyRequest)
	require.NoError(t, err)
	_, err = reopened.ResolveApprovalRequest(
		context.Background(), legacyRequest.ID, permissions.ApprovalApproved, permissions.GrantDurable, now,
	)
	require.NoError(t, err)
	_, err = reopened.CreateApprovalGrant(context.Background(), permissions.ApprovalGrant{
		ID: "grant_legacy", RequestID: legacyRequest.ID, Fingerprint: legacyRequest.Fingerprint, Actor: actor,
		Profile: legacyRequest.Profile, SessionID: legacyRequest.SessionID, Scope: permissions.GrantDurable,
		Status: permissions.GrantActive, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	require.NoError(t, err)
	_, ok, err = reopened.FindApprovalGrant(
		context.Background(), legacyRequest.Fingerprint, actor, legacyRequest.Profile, "session-b", now.Add(30*time.Minute),
	)
	require.NoError(t, err)
	require.True(t, ok)
	_, ok, err = reopened.FindApprovalGrant(
		context.Background(), legacyRequest.Fingerprint, actor, legacyRequest.Profile, "session-b", now.Add(2*time.Hour),
	)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestPermissionStore_ExpiresRevokesAndRecovers(t *testing.T) {
	store := newAutomationSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	_, _, err := store.CreateApprovalRequest(ctx, permissions.ApprovalRequest{
		ID: "approval_pending", Fingerprint: "pending", Status: permissions.ApprovalPending,
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	require.NoError(t, err)
	count, err := store.CancelPendingApprovals(ctx, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	expired := permissions.ApprovalGrant{
		ID: "grant_expired", RequestID: "approval_expired", Fingerprint: "expired",
		Actor: permissions.Actor{Kind: permissions.ActorLocalOwner}, Scope: permissions.GrantSession,
		Status: permissions.GrantActive, CreatedAt: now.Add(-time.Hour), ExpiresAt: now,
	}
	_, _, err = store.CreateApprovalRequest(ctx, permissions.ApprovalRequest{
		ID: expired.RequestID, Status: permissions.ApprovalPending, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	require.NoError(t, err)
	_, err = store.ResolveApprovalRequest(ctx, expired.RequestID, permissions.ApprovalApproved, permissions.GrantSession, now)
	require.NoError(t, err)
	_, err = store.CreateApprovalGrant(ctx, expired)
	require.NoError(t, err)
	_, ok, err := store.FindApprovalGrant(ctx, expired.Fingerprint, expired.Actor, "", "", now)
	require.NoError(t, err)
	require.False(t, ok)

	active := expired
	active.ID = "grant_active"
	active.RequestID = "approval_other"
	active.Fingerprint = "active"
	active.ExpiresAt = now.Add(time.Hour)
	_, _, err = store.CreateApprovalRequest(ctx, permissions.ApprovalRequest{
		ID: active.RequestID, Status: permissions.ApprovalPending, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	require.NoError(t, err)
	_, err = store.ResolveApprovalRequest(ctx, active.RequestID, permissions.ApprovalApproved, permissions.GrantSession, now)
	require.NoError(t, err)
	_, err = store.CreateApprovalGrant(ctx, active)
	require.NoError(t, err)
	revoked, err := store.RevokeApprovalGrant(ctx, active.ID, now)
	require.NoError(t, err)
	require.Equal(t, permissions.GrantRevoked, revoked.Status)
	revoked, err = store.RevokeApprovalGrant(ctx, active.ID, now)
	require.NoError(t, err)
	require.Equal(t, permissions.GrantRevoked, revoked.Status)
}

func TestPermissionStore_PrunesTerminalHistoryInBoundedBatches(t *testing.T) {
	store := newAutomationSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	old := now.Add(-31 * 24 * time.Hour)

	createResolved := func(id string, status permissions.ApprovalStatus, resolvedAt time.Time) {
		t.Helper()
		_, _, err := store.CreateApprovalRequest(ctx, permissions.ApprovalRequest{
			ID: id, Fingerprint: id, Status: permissions.ApprovalPending,
			CreatedAt: resolvedAt.Add(-time.Minute), ExpiresAt: resolvedAt,
		})
		require.NoError(t, err)
		_, err = store.ResolveApprovalRequest(ctx, id, status, "", resolvedAt)
		require.NoError(t, err)
	}
	createResolved("old_denied", permissions.ApprovalDenied, old)
	createResolved("recent_denied", permissions.ApprovalDenied, now.Add(-time.Hour))
	_, _, err := store.CreateApprovalRequest(ctx, permissions.ApprovalRequest{
		ID: "pending", Fingerprint: "pending", Status: permissions.ApprovalPending, CreatedAt: old, ExpiresAt: old,
	})
	require.NoError(t, err)

	for _, id := range []string{"active", "consumed"} {
		_, _, err = store.CreateApprovalRequest(ctx, permissions.ApprovalRequest{
			ID: "request_" + id, Fingerprint: id, Status: permissions.ApprovalPending,
			CreatedAt: old.Add(-time.Minute), ExpiresAt: old,
		})
		require.NoError(t, err)
		_, err = store.ResolveApprovalRequest(ctx, "request_"+id, permissions.ApprovalApproved, permissions.GrantOnce, old)
		require.NoError(t, err)
		_, err = store.CreateApprovalGrant(ctx, permissions.ApprovalGrant{
			ID: "grant_" + id, RequestID: "request_" + id, Fingerprint: id,
			Scope: permissions.GrantOnce, Status: permissions.GrantActive,
			CreatedAt: old, ExpiresAt: now.Add(time.Hour),
		})
		require.NoError(t, err)
	}
	_, _, err = store.CreateApprovalRequest(ctx, permissions.ApprovalRequest{
		ID: "request_always", Fingerprint: "always", Status: permissions.ApprovalPending,
		CreatedAt: old.Add(-time.Minute), ExpiresAt: old,
	})
	require.NoError(t, err)
	_, err = store.ResolveApprovalRequest(ctx, "request_always", permissions.ApprovalApproved, permissions.GrantAlways, old)
	require.NoError(t, err)
	_, err = store.CreateApprovalGrant(ctx, permissions.ApprovalGrant{
		ID: "grant_always", RequestID: "request_always", Fingerprint: "always",
		Scope: permissions.GrantAlways, Status: permissions.GrantActive, CreatedAt: old,
	})
	require.NoError(t, err)
	_, err = store.ConsumeApprovalGrant(ctx, "grant_consumed", old.Add(time.Hour))
	require.NoError(t, err)

	opts := permissions.ApprovalPruneOptions{
		Now: now, RequestRetention: 30 * 24 * time.Hour, GrantRetention: 30 * 24 * time.Hour,
		BatchSize: 10, DryRun: true,
	}
	preview, err := store.PruneApprovals(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, int64(2), preview.Requests)
	require.Equal(t, int64(1), preview.Grants)
	_, ok, err := store.GetApprovalRequest(ctx, "old_denied")
	require.NoError(t, err)
	require.True(t, ok)

	opts.DryRun = false
	pruned, err := store.PruneApprovals(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, preview.Requests, pruned.Requests)
	require.Equal(t, preview.Grants, pruned.Grants)
	for _, id := range []string{"old_denied", "request_consumed"} {
		_, ok, err = store.GetApprovalRequest(ctx, id)
		require.NoError(t, err)
		require.False(t, ok)
	}
	for _, id := range []string{"recent_denied", "pending", "request_active", "request_always"} {
		_, ok, err = store.GetApprovalRequest(ctx, id)
		require.NoError(t, err)
		require.True(t, ok)
	}
}

func TestPermissionStore_DeletesOnlyTerminalApprovalRecords(t *testing.T) {
	store := newAutomationSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	request := permissions.ApprovalRequest{
		ID: "approval_delete", Fingerprint: "delete", Status: permissions.ApprovalPending,
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	_, _, err := store.CreateApprovalRequest(ctx, request)
	require.NoError(t, err)
	_, err = store.DeleteApprovalRequest(ctx, request.ID, now)
	require.EqualError(t, err, "pending approval request cannot be deleted")
	_, err = store.ResolveApprovalRequest(ctx, request.ID, permissions.ApprovalApproved, permissions.GrantAlways, now)
	require.NoError(t, err)
	grant := permissions.ApprovalGrant{
		ID: "grant_delete", RequestID: request.ID, Fingerprint: request.Fingerprint,
		Scope: permissions.GrantAlways, Status: permissions.GrantActive, CreatedAt: now,
	}
	_, err = store.CreateApprovalGrant(ctx, grant)
	require.NoError(t, err)
	require.EqualError(t, store.DeleteApprovalGrant(ctx, grant.ID, now), "active approval grant cannot be deleted; revoke it first")
	linkedGrantID, err := store.DeleteApprovalRequest(ctx, request.ID, now)
	require.NoError(t, err)
	require.Equal(t, grant.ID, linkedGrantID)
	_, ok, err := store.GetApprovalRequest(ctx, request.ID)
	require.NoError(t, err)
	require.False(t, ok)
	grants, err := store.ListApprovalGrants(ctx, permissions.GrantQuery{})
	require.NoError(t, err)
	require.Empty(t, grants)
	_, err = store.DeleteApprovalRequest(ctx, "approval_missing", now)
	require.EqualError(t, err, "approval request not found")
	require.EqualError(t, store.DeleteApprovalGrant(ctx, "grant_missing", now), "approval grant not found")
}

func TestPermissionStore_ListSupportsOffsetAndLimit(t *testing.T) {
	store := newAutomationSQLiteStore(t)
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	for index, id := range []string{"oldest", "middle", "newest"} {
		_, _, err := store.CreateApprovalRequest(context.Background(), permissions.ApprovalRequest{
			ID: id, Fingerprint: id, Status: permissions.ApprovalPending,
			CreatedAt: now.Add(time.Duration(index) * time.Minute), ExpiresAt: now.Add(time.Hour),
		})
		require.NoError(t, err)
	}
	requests, err := store.ListApprovalRequests(context.Background(), permissions.ApprovalQuery{Limit: 1, Offset: 1})
	require.NoError(t, err)
	require.Len(t, requests, 1)
	require.Equal(t, "middle", requests[0].ID)
}
