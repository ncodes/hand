package permissions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetworkTargetFromURL_NormalizesIdentity(t *testing.T) {
	target, err := NetworkTargetFromURL("HTTPS://BÜCHER.EXAMPLE:443/a/../news", " get ", NetworkRequestNavigation)
	require.NoError(t, err)
	require.Equal(t, NetworkTarget{
		Scheme: "https", Host: "xn--bcher-kva.example", Port: 443, Path: "/news",
		Method: "GET", RequestClass: NetworkRequestNavigation,
	}, target)

	_, err = NetworkTargetFromURL("https://user:secret@example.com", "GET", NetworkRequestNavigation)
	require.EqualError(t, err, "permission network URL must not contain inline credentials")
	_, err = NetworkTargetFromURL("not-a-url", "GET", NetworkRequestNavigation)
	require.EqualError(t, err, "permission network URL is invalid")
	_, err = NetworkTargetFromURL("https://example.com:70000", "GET", NetworkRequestNavigation)
	require.EqualError(t, err, "permission network port is invalid")
}

func TestNetworkTargetFromURL_BindsCanonicalQueryWithoutExposingValues(t *testing.T) {
	first, err := NetworkTargetFromURL(
		"https://example.com/news?token=top-secret&page=2", "GET", NetworkRequestNavigation,
	)
	require.NoError(t, err)
	reordered, err := NetworkTargetFromURL(
		"https://example.com/news?page=2&token=top-secret", "GET", NetworkRequestNavigation,
	)
	require.NoError(t, err)
	changed, err := NetworkTargetFromURL(
		"https://example.com/news?page=3&token=top-secret", "GET", NetworkRequestNavigation,
	)
	require.NoError(t, err)

	require.NotEmpty(t, first.QueryHash)
	require.Equal(t, first.QueryHash, reordered.QueryHash)
	require.NotEqual(t, first.QueryHash, changed.QueryHash)
	require.NotContains(t, first.QueryHash, "top-secret")
	require.NotContains(t, getNetworkTargetFingerprint(&first), "top-secret")
}

func TestNetworkSelector_MatchesNormalizedStructuredTarget(t *testing.T) {
	target, err := NetworkTargetFromURL("https://example.com/news/latest", "GET", NetworkRequestNavigation)
	require.NoError(t, err)

	require.True(t, (NetworkSelector{Host: "EXAMPLE.COM", PathPrefix: "/news", Method: "get"}).Matches(target))
	require.False(t, (NetworkSelector{Host: "example.com", PathPrefix: "/new"}).Matches(target))
	require.False(t, (NetworkSelector{Host: "example.net"}).Matches(target))
	require.False(t, (NetworkSelector{PathPrefix: "/news/%2flatest"}).Matches(target))
}

func TestNetworkTarget_NormalizeRejectsAmbiguousValues(t *testing.T) {
	tests := []struct {
		name   string
		target NetworkTarget
	}{
		{name: "scheme", target: NetworkTarget{Scheme: "file", Host: "example.com", Method: "GET", RequestClass: NetworkRequestNavigation}},
		{name: "host", target: NetworkTarget{Scheme: "https", Host: "bad/host", Method: "GET", RequestClass: NetworkRequestNavigation}},
		{name: "path", target: NetworkTarget{Scheme: "https", Host: "example.com", Path: "/a%2fb", Method: "GET", RequestClass: NetworkRequestNavigation}},
		{name: "method", target: NetworkTarget{Scheme: "https", Host: "example.com", RequestClass: NetworkRequestNavigation}},
		{name: "malformed method", target: NetworkTarget{Scheme: "https", Host: "example.com", Method: "GET SPACE", RequestClass: NetworkRequestNavigation}},
		{name: "query hash", target: NetworkTarget{Scheme: "https", Host: "example.com", Method: "GET", QueryHash: "invalid", RequestClass: NetworkRequestNavigation}},
		{name: "class", target: NetworkTarget{Scheme: "https", Host: "example.com", Method: "GET", RequestClass: "other"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.target.Normalize()
			require.Error(t, err)
		})
	}
}

func TestNetworkSelector_NormalizeRejectsMalformedMethod(t *testing.T) {
	_, err := (NetworkSelector{Host: "example.com", Method: "GET SPACE"}).Normalize()
	require.EqualError(t, err, "permission network selector method is invalid")
}

func TestNetworkSelector_NormalizesEveryConstraintAndRejectsInvalidValues(t *testing.T) {
	selector, err := (NetworkSelector{
		Scheme: " HTTPS ", Host: "BÜCHER.EXAMPLE.", Port: 8443, PathPrefix: "/news/../latest",
		Method: " get ", RequestClass: "NAVIGATION",
	}).Normalize()
	require.NoError(t, err)
	require.Equal(t, NetworkSelector{
		Scheme: "https", Host: "xn--bcher-kva.example", Port: 8443, PathPrefix: "/latest",
		Method: "GET", RequestClass: NetworkRequestNavigation,
	}, selector)

	tests := []struct {
		name     string
		selector NetworkSelector
		message  string
	}{
		{name: "empty", selector: NetworkSelector{}, message: "permission network selector must constrain at least one field"},
		{name: "scheme", selector: NetworkSelector{Scheme: "file"}, message: "permission network selector scheme is invalid"},
		{name: "host", selector: NetworkSelector{Host: "bad/host"}, message: "permission network host is invalid"},
		{name: "path", selector: NetworkSelector{PathPrefix: "relative"}, message: "permission network path must be absolute"},
		{name: "class", selector: NetworkSelector{RequestClass: "invalid"}, message: "permission network selector request class is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, normalizeErr := test.selector.Normalize()
			require.EqualError(t, normalizeErr, test.message)
		})
	}
}

func TestPolicy_NetworkSelectorUsesSegmentSafeMatching(t *testing.T) {
	target, err := NetworkTargetFromURL("https://example.com/news/latest", "GET", NetworkRequestNavigation)
	require.NoError(t, err)
	authorization := AuthorizationContext{Actor: Actor{Kind: ActorLocalOwner}, Surface: SurfaceTUI}
	policy := Policy{Rules: []Rule{{
		Name: "allow news", Resources: []Resource{ResourceNetwork}, Actions: []Action{ActionRead},
		Network: []NetworkSelector{{Host: "example.com", PathPrefix: "/news"}}, Decision: DecisionAllow,
	}}}

	allowed := policy.Evaluate(EvaluationInput{
		Authorization: authorization,
		Operation:     Operation{Resource: ResourceNetwork, Action: ActionRead, Network: &target},
	})
	require.Equal(t, DecisionAllow, allowed.Decision)

	target.Path = "/newsletter"
	denied := policy.Evaluate(EvaluationInput{
		Authorization: authorization,
		Operation:     Operation{Resource: ResourceNetwork, Action: ActionRead, Network: &target},
	})
	require.NotEqual(t, DecisionAllow, denied.Decision)
}

func TestPermissionScope_RequiresStructuredNetworkSelectors(t *testing.T) {
	target, err := NetworkTargetFromURL("https://example.com/news", "GET", NetworkRequestNavigation)
	require.NoError(t, err)
	operation := Operation{Resource: ResourceNetwork, Action: ActionRead, Effects: []Effect{EffectNetwork}, Network: &target}

	scope := PermissionScope{
		Restricted: true, Resources: []Resource{ResourceNetwork}, Actions: []Action{ActionRead},
		Effects: []Effect{EffectNetwork}, Network: []NetworkSelector{{Host: "example.com"}},
	}
	require.True(t, scope.Allows(operation))
	scope.Network = nil
	require.False(t, scope.Allows(operation))
}

func TestIntersectScopes_IntersectsStructuredNetworkSelectors(t *testing.T) {
	parent := PermissionScope{
		Restricted: true, Resources: []Resource{ResourceNetwork}, Actions: []Action{ActionRead},
		Effects: []Effect{EffectNetwork}, Network: []NetworkSelector{{Host: "example.com", PathPrefix: "/news"}},
	}
	delegated := PermissionScope{
		Restricted: true, Resources: []Resource{ResourceNetwork}, Actions: []Action{ActionRead},
		Effects: []Effect{EffectNetwork}, Network: []NetworkSelector{{
			Scheme: "https", Host: "example.com", PathPrefix: "/news/latest", Method: "GET",
		}},
	}

	intersection, err := IntersectScopes(parent, delegated)
	require.NoError(t, err)
	require.Equal(t, delegated.Network, intersection.Network)

	delegated.Network[0].Host = "example.net"
	intersection, err = IntersectScopes(parent, delegated)
	require.NoError(t, err)
	require.Empty(t, intersection.Network)
}

func TestFingerprintAndAuthorizedOperations_BindStructuredNetworkIdentity(t *testing.T) {
	left, err := NetworkTargetFromURL("https://example.com/news", "GET", NetworkRequestNavigation)
	require.NoError(t, err)
	right, err := NetworkTargetFromURL("https://example.com/other", "GET", NetworkRequestNavigation)
	require.NoError(t, err)
	authorization := AuthorizationContext{Actor: Actor{Kind: ActorLocalOwner, ID: "owner"}, Surface: SurfaceTUI}
	leftOperation := Operation{Resource: ResourceNetwork, Action: ActionRead, Network: &left}
	rightOperation := Operation{Resource: ResourceNetwork, Action: ActionRead, Network: &right}

	require.NotEqual(t, Fingerprint(authorization, leftOperation), Fingerprint(authorization, rightOperation))
	ctx := WithAuthorizedOperations(context.Background(), []Operation{leftOperation})
	require.True(t, IsOperationAuthorized(ctx, leftOperation))
	require.False(t, IsOperationAuthorized(ctx, rightOperation))
}

func TestOperation_NormalizeRejectsAmbiguousNetworkIdentity(t *testing.T) {
	target, err := NetworkTargetFromURL("https://example.com", "GET", NetworkRequestNavigation)
	require.NoError(t, err)
	_, err = (Operation{
		Resource: ResourceNetwork, Action: ActionRead, Target: "https://example.com", Network: &target,
	}).Normalize()
	require.EqualError(t, err, "permission operation cannot combine a network target and raw target")
	_, err = (Operation{Resource: ResourceFile, Action: ActionRead, Network: &target}).Normalize()
	require.EqualError(t, err, "permission network target requires the network resource")
}

func TestPolicy_RawTargetPrefixCannotAuthorizeStructuredNetworkTarget(t *testing.T) {
	target, err := NetworkTargetFromURL("https://example.com/news", "GET", NetworkRequestNavigation)
	require.NoError(t, err)
	policy := Policy{Rules: []Rule{{
		Name: "legacy prefix", Resources: []Resource{ResourceNetwork}, Actions: []Action{ActionRead},
		TargetPrefixes: []string{"https://example.com"}, Decision: DecisionAllow,
	}}}
	evaluation := policy.Evaluate(EvaluationInput{
		Authorization: AuthorizationContext{Actor: Actor{Kind: ActorLocalOwner}, Surface: SurfaceTUI},
		Operation:     Operation{Resource: ResourceNetwork, Action: ActionRead, Network: &target},
	})
	require.NotEqual(t, DecisionAllow, evaluation.Decision)
}

func TestPolicyAndScope_RejectMixedStructuredAndRawNetworkMatching(t *testing.T) {
	policy := Policy{Rules: []Rule{{
		Name: "ambiguous", TargetPrefixes: []string{"https://example.com"},
		Network: []NetworkSelector{{Host: "example.com"}}, Decision: DecisionAllow,
	}}}
	require.EqualError(t, policy.Validate(), "permission rule cannot combine network selectors and target prefixes")

	_, err := (PermissionScope{
		Restricted: true, TargetPrefixes: []string{"https://example.com"},
		Network: []NetworkSelector{{Host: "example.com"}},
	}).Normalize()
	require.EqualError(t, err, "permission scope cannot combine network selectors and target prefixes")
}

func TestPermissionScope_NetworkSelectorIntersectionUsesNarrowestCompatibleValues(t *testing.T) {
	parent := PermissionScope{Restricted: true, Network: []NetworkSelector{{
		Host: "example.com", PathPrefix: "/news",
	}}}
	delegated := PermissionScope{Restricted: true, Network: []NetworkSelector{{
		Scheme: "https", Host: "example.com", Port: 443, PathPrefix: "/news/latest",
		Method: "GET", RequestClass: NetworkRequestNavigation,
	}}}
	intersection, err := IntersectScopes(parent, delegated)
	require.NoError(t, err)
	require.Equal(t, delegated.Network, intersection.Network)

	parent.Network[0].Scheme = "http"
	intersection, err = IntersectScopes(parent, delegated)
	require.NoError(t, err)
	require.Empty(t, intersection.Network)
	parent.Network[0].Scheme = ""
	parent.Network[0].PathPrefix = "/other"
	intersection, err = IntersectScopes(parent, delegated)
	require.NoError(t, err)
	require.Empty(t, intersection.Network)
}
