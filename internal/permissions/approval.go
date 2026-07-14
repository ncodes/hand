package permissions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wandxy/morph/pkg/nanoid"
)

const (
	ApprovalRequestIDPrefix         = "approval_"
	ApprovalGrantIDPrefix           = "grant_"
	DefaultApprovalRequestTTL       = 2 * time.Minute
	DefaultApprovalOnceTTL          = 2 * time.Minute
	DefaultApprovalSessionTTL       = 8 * time.Hour
	DefaultApprovalRequestRetention = 30 * 24 * time.Hour
	DefaultApprovalGrantRetention   = 30 * 24 * time.Hour
	DefaultApprovalCleanupInterval  = time.Hour
	DefaultApprovalCleanupBatchSize = 100
)

type ApprovalStatus string

const (
	ApprovalPending   ApprovalStatus = "pending"
	ApprovalApproved  ApprovalStatus = "approved"
	ApprovalDenied    ApprovalStatus = "denied"
	ApprovalExpired   ApprovalStatus = "expired"
	ApprovalCancelled ApprovalStatus = "cancelled"
	ApprovalFailed    ApprovalStatus = "failed"
)

type GrantStatus string

const (
	GrantActive   GrantStatus = "active"
	GrantConsumed GrantStatus = "consumed"
	GrantExpired  GrantStatus = "expired"
	GrantRevoked  GrantStatus = "revoked"
)

type GrantScope string

const (
	GrantOnce    GrantScope = "once"
	GrantSession GrantScope = "session"
	GrantAlways  GrantScope = "always"
	GrantDurable GrantScope = "durable"
)

type ApprovalRequest struct {
	ID          string
	Fingerprint string
	Actor       Actor
	SurfaceKind SurfaceKind
	Surface     Surface
	Profile     string
	SessionID   string
	RunID       string
	Tool        string
	Resource    Resource
	Action      Action
	Effects     []Effect
	Summary     string
	Reason      string
	Status      ApprovalStatus
	Scope       GrantScope
	GrantID     string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ResolvedAt  time.Time
}

type ApprovalGrant struct {
	ID          string
	RequestID   string
	Fingerprint string
	Actor       Actor
	Profile     string
	SessionID   string
	Scope       GrantScope
	Status      GrantStatus
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ConsumedAt  time.Time
	RevokedAt   time.Time
}

func (g ApprovalGrant) IsExpiredAt(now time.Time) bool {
	return g.Scope != GrantAlways && !g.ExpiresAt.After(now)
}

func (g ApprovalGrant) IsActiveAt(now time.Time) bool {
	return g.Status == GrantActive && !g.IsExpiredAt(now)
}

type ApprovalQuery struct {
	Status ApprovalStatus
	Limit  int
	Offset int
}

type GrantQuery struct {
	Status GrantStatus
	Limit  int
	Offset int
}

type ApprovalPruneOptions struct {
	Now              time.Time
	RequestRetention time.Duration
	GrantRetention   time.Duration
	BatchSize        int
	DryRun           bool
}

type ApprovalPruneResult struct {
	Requests      int64
	Grants        int64
	RequestCutoff time.Time
	GrantCutoff   time.Time
	DryRun        bool
}

type ApprovalRecordKind string

const (
	ApprovalRecordRequest ApprovalRecordKind = "request"
	ApprovalRecordGrant   ApprovalRecordKind = "grant"
)

type ApprovalDeleteResult struct {
	ID            string
	Kind          ApprovalRecordKind
	LinkedGrantID string
}

type ApprovalStore interface {
	CreateApprovalRequest(context.Context, ApprovalRequest) (ApprovalRequest, bool, error)
	GetApprovalRequest(context.Context, string) (ApprovalRequest, bool, error)
	ListApprovalRequests(context.Context, ApprovalQuery) ([]ApprovalRequest, error)
	ResolveApprovalRequest(context.Context, string, ApprovalStatus, GrantScope, time.Time) (ApprovalRequest, error)
	CancelPendingApprovals(context.Context, time.Time) (int64, error)
	CreateApprovalGrant(context.Context, ApprovalGrant) (ApprovalGrant, error)
	FindApprovalGrant(context.Context, string, Actor, string, string, time.Time) (ApprovalGrant, bool, error)
	ConsumeApprovalGrant(context.Context, string, time.Time) (ApprovalGrant, error)
	ListApprovalGrants(context.Context, GrantQuery) ([]ApprovalGrant, error)
	RevokeApprovalGrant(context.Context, string, time.Time) (ApprovalGrant, error)
	DeleteApprovalRequest(context.Context, string, time.Time) (string, error)
	DeleteApprovalGrant(context.Context, string, time.Time) error
	PruneApprovals(context.Context, ApprovalPruneOptions) (ApprovalPruneResult, error)
}

type ApprovalAuditor interface {
	ApprovalChanged(context.Context, ApprovalRequest)
}

type Approver interface {
	Authorize(context.Context, EvaluationInput) error
}

type ApprovalOptions struct {
	RequestTTL       time.Duration
	OnceTTL          time.Duration
	SessionTTL       time.Duration
	Now              func() time.Time
	Auditor          ApprovalAuditor
	RequestRetention time.Duration
	GrantRetention   time.Duration
	CleanupInterval  time.Duration
	CleanupBatchSize int
}

type ApprovalService struct {
	store   ApprovalStore
	opts    ApprovalOptions
	mu      sync.Mutex
	waiters map[string][]chan ApprovalRequest
}

func NewApprovalService(store ApprovalStore, opts ApprovalOptions) (*ApprovalService, error) {
	if store == nil {
		return nil, errors.New("approval store is required")
	}
	if opts.RequestTTL <= 0 {
		opts.RequestTTL = DefaultApprovalRequestTTL
	}
	if opts.OnceTTL <= 0 {
		opts.OnceTTL = DefaultApprovalOnceTTL
	}
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = DefaultApprovalSessionTTL
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if opts.RequestRetention < 0 || opts.GrantRetention < 0 {
		return nil, errors.New("approval retention must be greater than or equal to zero")
	}
	if opts.RequestRetention == 0 {
		opts.RequestRetention = DefaultApprovalRequestRetention
	}
	if opts.GrantRetention == 0 {
		opts.GrantRetention = DefaultApprovalGrantRetention
	}
	if opts.CleanupInterval <= 0 {
		opts.CleanupInterval = DefaultApprovalCleanupInterval
	}
	if opts.CleanupBatchSize <= 0 {
		opts.CleanupBatchSize = DefaultApprovalCleanupBatchSize
	}

	return &ApprovalService{store: store, opts: opts, waiters: make(map[string][]chan ApprovalRequest)}, nil
}

func (s *ApprovalService) Recover(ctx context.Context) error {
	if s == nil || s.store == nil {
		return errors.New("approval service is required")
	}
	if _, err := s.store.CancelPendingApprovals(ctx, s.opts.Now()); err != nil {
		return err
	}
	_, err := s.Prune(ctx, false)
	return err
}

func (s *ApprovalService) StartCleanup(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(s.opts.CleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.Prune(context.WithoutCancel(ctx), false)
			}
		}
	}()
}

func (s *ApprovalService) Prune(ctx context.Context, dryRun bool) (ApprovalPruneResult, error) {
	if s == nil || s.store == nil {
		return ApprovalPruneResult{}, errors.New("approval service is required")
	}
	return s.store.PruneApprovals(ctx, ApprovalPruneOptions{
		Now: s.opts.Now(), RequestRetention: s.opts.RequestRetention,
		GrantRetention: s.opts.GrantRetention, BatchSize: s.opts.CleanupBatchSize, DryRun: dryRun,
	})
}

func (s *ApprovalService) Authorize(ctx context.Context, input EvaluationInput) error {
	if s == nil || s.store == nil {
		return errors.New("approval service is unavailable")
	}
	authorization, operation, fingerprint, err := normalizeApprovalInput(ctx, input)
	if err != nil {
		return err
	}
	now := s.opts.Now()
	grant, ok, err := s.store.FindApprovalGrant(
		ctx,
		fingerprint,
		authorization.Actor,
		authorization.Profile,
		authorization.SessionID,
		now,
	)
	if err != nil {
		return fmt.Errorf("failed to read approval grant: %w", err)
	}
	if ok {
		if grant.Scope == GrantOnce {
			if _, err := s.store.ConsumeApprovalGrant(ctx, grant.ID, now); err != nil {
				return fmt.Errorf("failed to consume approval grant: %w", err)
			}
		}
		return nil
	}
	if !isInteractiveApprovalSurface(authorization.Surface) {
		return &DecisionError{Code: ErrorCodeApprovalRequired, Evaluation: Evaluation{
			Decision: DecisionAsk,
			Reason:   "approval requires an interactive local surface",
		}}
	}

	request := ApprovalRequest{
		ID:          nanoid.MustGenerate(ApprovalRequestIDPrefix),
		Fingerprint: fingerprint,
		Actor:       authorization.Actor,
		SurfaceKind: authorization.SurfaceKind,
		Surface:     authorization.Surface,
		Profile:     authorization.Profile,
		SessionID:   authorization.SessionID,
		RunID:       authorization.RunID,
		Tool:        operation.Tool,
		Resource:    operation.Resource,
		Action:      operation.Action,
		Effects:     append([]Effect(nil), operation.Effects...),
		Summary:     getApprovalSummary(operation),
		Reason:      input.ApprovalReason,
		Status:      ApprovalPending,
		CreatedAt:   now,
		ExpiresAt:   now.Add(s.opts.RequestTTL),
	}
	request, _, err = s.store.CreateApprovalRequest(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to create approval request: %w", err)
	}
	s.audit(ctx, request)

	resolved, err := s.wait(ctx, request)
	if err != nil {
		return err
	}
	s.audit(ctx, resolved)
	if resolved.Status != ApprovalApproved {
		return &DecisionError{Code: ErrorCodeDenied, Evaluation: Evaluation{
			Decision: DecisionDeny,
			Reason:   "approval " + string(resolved.Status),
		}}
	}
	grant, ok, err = s.store.FindApprovalGrant(
		ctx,
		fingerprint,
		authorization.Actor,
		authorization.Profile,
		authorization.SessionID,
		s.opts.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to verify approval grant: %w", err)
	}
	if !ok {
		return errors.New("approved request has no matching grant")
	}
	if grant.Scope == GrantOnce {
		if _, err := s.store.ConsumeApprovalGrant(ctx, grant.ID, s.opts.Now()); err != nil {
			return fmt.Errorf("failed to consume approval grant: %w", err)
		}
	}
	return nil
}

func (s *ApprovalService) Resolve(ctx context.Context, id string, approved bool, scope GrantScope) (ApprovalRequest, error) {
	if s == nil || s.store == nil {
		return ApprovalRequest{}, errors.New("approval service is required")
	}
	if !approved {
		scope = ""
	} else if scope != GrantOnce && scope != GrantSession && scope != GrantAlways {
		return ApprovalRequest{}, errors.New("approval scope must be one of: once, session, always")
	}
	existing, ok, err := s.store.GetApprovalRequest(ctx, id)
	if err != nil {
		return ApprovalRequest{}, err
	}
	if !ok {
		return ApprovalRequest{}, errors.New("approval request not found")
	}
	wantedStatus := ApprovalDenied
	if approved {
		wantedStatus = ApprovalApproved
	}
	if existing.Status != ApprovalPending {
		if existing.Status == wantedStatus && existing.Scope == scope {
			return existing, nil
		}
		return ApprovalRequest{}, errors.New("approval request is already resolved")
	}
	if approved && scope == GrantAlways && !isAlwaysApprovalAvailable(existing.Effects) {
		return ApprovalRequest{}, errors.New("always approval is unavailable for these effects")
	}
	now := s.opts.Now()
	request, err := s.store.ResolveApprovalRequest(ctx, id, wantedStatus, scope, now)
	if err != nil {
		return ApprovalRequest{}, err
	}
	if approved {
		grant := ApprovalGrant{
			ID:          nanoid.MustGenerate(ApprovalGrantIDPrefix),
			RequestID:   request.ID,
			Fingerprint: request.Fingerprint,
			Actor:       request.Actor,
			Profile:     request.Profile,
			SessionID:   request.SessionID,
			Scope:       scope,
			Status:      GrantActive,
			CreatedAt:   now,
			ExpiresAt:   s.getGrantExpiry(now, scope),
		}
		grant, err = s.store.CreateApprovalGrant(ctx, grant)
		if err != nil {
			failed, resolveErr := s.store.ResolveApprovalRequest(ctx, id, ApprovalFailed, scope, now)
			if resolveErr == nil {
				s.audit(ctx, failed)
				s.notify(failed)
			} else {
				s.failRequest(ctx, request, err)
			}
			return ApprovalRequest{}, err
		}
		request.GrantID = grant.ID
	}
	s.audit(ctx, request)
	s.notify(request)
	return request, nil
}

func (s *ApprovalService) Get(ctx context.Context, id string) (ApprovalRequest, bool, error) {
	return s.store.GetApprovalRequest(ctx, id)
}

func (s *ApprovalService) List(ctx context.Context, query ApprovalQuery) ([]ApprovalRequest, error) {
	return s.store.ListApprovalRequests(ctx, query)
}

func (s *ApprovalService) ListGrants(ctx context.Context, query GrantQuery) ([]ApprovalGrant, error) {
	return s.store.ListApprovalGrants(ctx, query)
}

func (s *ApprovalService) Revoke(ctx context.Context, id string) (ApprovalGrant, error) {
	if s == nil || s.store == nil {
		return ApprovalGrant{}, errors.New("approval service is required")
	}
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, ApprovalRequestIDPrefix) {
		request, ok, err := s.store.GetApprovalRequest(ctx, id)
		if err != nil {
			return ApprovalGrant{}, err
		}
		if !ok {
			return ApprovalGrant{}, errors.New("approval request not found")
		}
		if request.GrantID == "" {
			return ApprovalGrant{}, errors.New("approval request has no grant")
		}
		id = request.GrantID
	}
	return s.store.RevokeApprovalGrant(ctx, id, s.opts.Now())
}

func (s *ApprovalService) Delete(ctx context.Context, id string) (ApprovalDeleteResult, error) {
	if s == nil || s.store == nil {
		return ApprovalDeleteResult{}, errors.New("approval service is required")
	}
	id = strings.TrimSpace(id)
	switch {
	case strings.HasPrefix(id, ApprovalRequestIDPrefix):
		linkedGrantID, err := s.store.DeleteApprovalRequest(ctx, id, s.opts.Now())
		if err != nil {
			return ApprovalDeleteResult{}, err
		}
		return ApprovalDeleteResult{
			ID: id, Kind: ApprovalRecordRequest, LinkedGrantID: linkedGrantID,
		}, nil
	case strings.HasPrefix(id, ApprovalGrantIDPrefix):
		if err := s.store.DeleteApprovalGrant(ctx, id, s.opts.Now()); err != nil {
			return ApprovalDeleteResult{}, err
		}
		return ApprovalDeleteResult{ID: id, Kind: ApprovalRecordGrant}, nil
	default:
		return ApprovalDeleteResult{}, errors.New("approval or grant id is required")
	}
}

func (s *ApprovalService) wait(ctx context.Context, request ApprovalRequest) (ApprovalRequest, error) {
	updates := make(chan ApprovalRequest, 1)
	s.mu.Lock()
	s.waiters[request.ID] = append(s.waiters[request.ID], updates)
	s.mu.Unlock()
	removed := false
	defer func() {
		if !removed {
			s.removeWaiter(request.ID, updates)
		}
	}()
	current, ok, err := s.store.GetApprovalRequest(ctx, request.ID)
	if err != nil {
		s.failRequest(ctx, request, err)
		return ApprovalRequest{}, err
	}
	if !ok {
		err := errors.New("approval request not found")
		s.failRequest(ctx, request, err)
		return ApprovalRequest{}, err
	}
	if current.Status != ApprovalPending {
		return current, nil
	}

	timer := time.NewTimer(max(request.ExpiresAt.Sub(s.opts.Now()), 0))
	defer timer.Stop()
	select {
	case resolved := <-updates:
		return resolved, nil
	case <-ctx.Done():
		remaining := s.removeWaiter(request.ID, updates)
		removed = true
		if remaining == 0 {
			terminalCtx := context.WithoutCancel(ctx)
			resolved, err := s.store.ResolveApprovalRequest(
				terminalCtx, request.ID, ApprovalCancelled, "", s.opts.Now(),
			)
			if err == nil {
				s.audit(terminalCtx, resolved)
				s.notify(resolved)
			} else {
				s.failRequest(terminalCtx, request, err)
			}
		}
		return ApprovalRequest{}, ctx.Err()
	case <-timer.C:
		terminalCtx := context.WithoutCancel(ctx)
		resolved, err := s.store.ResolveApprovalRequest(terminalCtx, request.ID, ApprovalExpired, "", s.opts.Now())
		if err != nil {
			s.failRequest(terminalCtx, request, err)
			return ApprovalRequest{}, err
		}
		s.audit(terminalCtx, resolved)
		s.notify(resolved)
		return resolved, nil
	}
}

func (s *ApprovalService) notify(request ApprovalRequest) {
	s.mu.Lock()
	waiters := append([]chan ApprovalRequest(nil), s.waiters[request.ID]...)
	s.mu.Unlock()
	for _, waiter := range waiters {
		select {
		case waiter <- request:
		default:
		}
	}
}

func (s *ApprovalService) removeWaiter(id string, waiter chan ApprovalRequest) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := s.waiters[id]
	for index, value := range values {
		if value == waiter {
			values = append(values[:index], values[index+1:]...)
			break
		}
	}
	if len(values) == 0 {
		delete(s.waiters, id)
		return 0
	}
	s.waiters[id] = values
	return len(values)
}

func (s *ApprovalService) getGrantExpiry(now time.Time, scope GrantScope) time.Time {
	switch scope {
	case GrantOnce:
		return now.Add(s.opts.OnceTTL)
	case GrantSession:
		return now.Add(s.opts.SessionTTL)
	default:
		return time.Time{}
	}
}

func (s *ApprovalService) audit(ctx context.Context, request ApprovalRequest) {
	if s.opts.Auditor != nil {
		s.opts.Auditor.ApprovalChanged(ctx, request)
	}
}

func (s *ApprovalService) failRequest(ctx context.Context, request ApprovalRequest, cause error) {
	request.Status = ApprovalFailed
	request.Reason = "approval workflow failed: " + cause.Error()
	request.ResolvedAt = s.opts.Now()
	s.audit(context.WithoutCancel(ctx), request)
	s.notify(request)
}

func normalizeApprovalInput(
	ctx context.Context,
	input EvaluationInput,
) (AuthorizationContext, Operation, string, error) {
	authorization, ok := FromContext(ctx)
	if !ok {
		return AuthorizationContext{}, Operation{}, "", errors.New("authorization context is required")
	}
	operation, err := input.Operation.Normalize()
	if err != nil {
		return AuthorizationContext{}, Operation{}, "", err
	}
	return authorization, operation, Fingerprint(authorization, operation), nil
}

func isInteractiveApprovalSurface(surface Surface) bool {
	return surface == SurfaceCLI || surface == SurfaceTUI
}

func getApprovalSummary(operation Operation) string {
	return fmt.Sprintf("%s · %s %s", operation.Tool, operation.Action, operation.Resource)
}

func isAlwaysApprovalAvailable(effects []Effect) bool {
	for _, effect := range effects {
		switch effect {
		case EffectDestructive, EffectCredentialBearing, EffectPrivilegeChanging,
			EffectExecution, EffectNetwork, EffectExternalSystem:
			return false
		}
	}
	return true
}
