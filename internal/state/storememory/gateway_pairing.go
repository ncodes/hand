package storememory

import (
	"context"
	"errors"
	"sort"

	"github.com/wandxy/morph/pkg/gateway/pairing"
	"github.com/wandxy/morph/pkg/stringx"
)

func (s *Store) SaveGatewayPairingRequest(ctx context.Context, request pairing.PendingRequest) error {
	_ = ctx
	if s == nil {
		return errors.New("store is required")
	}

	request.Source = normalizeGatewayPairingSource(request.Source)
	request.SenderID = stringx.String(request.SenderID).Trim()
	if request.Source == "" {
		return errors.New("gateway pairing source is required")
	}
	if request.SenderID == "" {
		return errors.New("gateway pairing sender id is required")
	}

	request.Metadata = cloneGatewayPairingMetadata(request.Metadata)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairingRequests[gatewayPairingKey(request.Source, request.SenderID)] = request

	return nil
}

func (s *Store) GetGatewayPairingRequest(
	ctx context.Context,
	source string,
	senderID string,
) (pairing.PendingRequest, bool, error) {
	_ = ctx
	if s == nil {
		return pairing.PendingRequest{}, false, errors.New("store is required")
	}

	source = normalizeGatewayPairingSource(source)
	senderID = stringx.String(senderID).Trim()
	if source == "" {
		return pairing.PendingRequest{}, false, errors.New("gateway pairing source is required")
	}
	if senderID == "" {
		return pairing.PendingRequest{}, false, errors.New("gateway pairing sender id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	request, ok := s.pairingRequests[gatewayPairingKey(source, senderID)]
	request.Metadata = cloneGatewayPairingMetadata(request.Metadata)

	return request, ok, nil
}

func (s *Store) ListGatewayPairingRequests(
	ctx context.Context,
	source string,
) ([]pairing.PendingRequest, error) {
	_ = ctx
	if s == nil {
		return nil, errors.New("store is required")
	}

	source = normalizeGatewayPairingSource(source)
	s.mu.RLock()
	defer s.mu.RUnlock()

	var requests []pairing.PendingRequest
	for _, request := range s.pairingRequests {
		if source != "" && request.Source != source {
			continue
		}
		request.Metadata = cloneGatewayPairingMetadata(request.Metadata)
		requests = append(requests, request)
	}
	sort.Slice(requests, func(i int, j int) bool {
		if requests[i].Source == requests[j].Source {
			return requests[i].SenderID < requests[j].SenderID
		}

		return requests[i].Source < requests[j].Source
	})

	return requests, nil
}

func (s *Store) DeleteGatewayPairingRequest(ctx context.Context, source string, senderID string) error {
	_ = ctx
	if s == nil {
		return errors.New("store is required")
	}

	source = normalizeGatewayPairingSource(source)
	senderID = stringx.String(senderID).Trim()
	if source == "" {
		return errors.New("gateway pairing source is required")
	}
	if senderID == "" {
		return errors.New("gateway pairing sender id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pairingRequests, gatewayPairingKey(source, senderID))

	return nil
}

func (s *Store) ClearGatewayPairingRequests(ctx context.Context, source string) error {
	_ = ctx
	if s == nil {
		return errors.New("store is required")
	}

	source = normalizeGatewayPairingSource(source)
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, request := range s.pairingRequests {
		if source == "" || request.Source == source {
			delete(s.pairingRequests, key)
		}
	}

	return nil
}

func (s *Store) SaveGatewayPairedSender(ctx context.Context, sender pairing.ApprovedSender) error {
	_ = ctx
	if s == nil {
		return errors.New("store is required")
	}

	sender.Source = normalizeGatewayPairingSource(sender.Source)
	sender.SenderID = stringx.String(sender.SenderID).Trim()
	if sender.Source == "" {
		return errors.New("gateway pairing source is required")
	}
	if sender.SenderID == "" {
		return errors.New("gateway pairing sender id is required")
	}

	sender.Metadata = cloneGatewayPairingMetadata(sender.Metadata)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairedSenders[gatewayPairingKey(sender.Source, sender.SenderID)] = sender

	return nil
}

func (s *Store) GetGatewayPairedSender(
	ctx context.Context,
	source string,
	senderID string,
) (pairing.ApprovedSender, bool, error) {
	_ = ctx
	if s == nil {
		return pairing.ApprovedSender{}, false, errors.New("store is required")
	}

	source = normalizeGatewayPairingSource(source)
	senderID = stringx.String(senderID).Trim()
	if source == "" {
		return pairing.ApprovedSender{}, false, errors.New("gateway pairing source is required")
	}
	if senderID == "" {
		return pairing.ApprovedSender{}, false, errors.New("gateway pairing sender id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	sender, ok := s.pairedSenders[gatewayPairingKey(source, senderID)]
	sender.Metadata = cloneGatewayPairingMetadata(sender.Metadata)

	return sender, ok, nil
}

func (s *Store) ListGatewayPairedSenders(
	ctx context.Context,
	source string,
) ([]pairing.ApprovedSender, error) {
	_ = ctx
	if s == nil {
		return nil, errors.New("store is required")
	}

	source = normalizeGatewayPairingSource(source)
	s.mu.RLock()
	defer s.mu.RUnlock()

	var senders []pairing.ApprovedSender
	for _, sender := range s.pairedSenders {
		if source != "" && sender.Source != source {
			continue
		}
		sender.Metadata = cloneGatewayPairingMetadata(sender.Metadata)
		senders = append(senders, sender)
	}
	sort.Slice(senders, func(i int, j int) bool {
		if senders[i].Source == senders[j].Source {
			return senders[i].SenderID < senders[j].SenderID
		}

		return senders[i].Source < senders[j].Source
	})

	return senders, nil
}

func (s *Store) DeleteGatewayPairedSender(ctx context.Context, source string, senderID string) error {
	_ = ctx
	if s == nil {
		return errors.New("store is required")
	}

	source = normalizeGatewayPairingSource(source)
	senderID = stringx.String(senderID).Trim()
	if source == "" {
		return errors.New("gateway pairing source is required")
	}
	if senderID == "" {
		return errors.New("gateway pairing sender id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pairedSenders, gatewayPairingKey(source, senderID))

	return nil
}

func gatewayPairingKey(source string, senderID string) string {
	return normalizeGatewayPairingSource(source) + "\x00" + stringx.String(senderID).Trim()
}

func normalizeGatewayPairingSource(source string) string {
	return stringx.String(source).Normalized()
}

func cloneGatewayPairingMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	clone := make(map[string]string, len(values))
	for key, value := range values {
		key = stringx.String(key).Trim()
		if key == "" {
			continue
		}
		clone[key] = stringx.String(value).Trim()
	}
	if len(clone) == 0 {
		return nil
	}

	return clone
}
