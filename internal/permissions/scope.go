package permissions

import (
	"errors"
	"slices"
	"strings"

	"github.com/wandxy/morph/pkg/str"
)

type PermissionScope struct {
	Restricted     bool
	Resources      []Resource
	Actions        []Action
	Effects        []Effect
	TargetPrefixes []string
}

func (s PermissionScope) Normalize() (PermissionScope, error) {
	if len(s.Resources) > 0 {
		s.Resources = normalizeValues(s.Resources, func(value Resource) Resource {
			return Resource(str.String(value).Normalized())
		})
	}
	if len(s.Actions) > 0 {
		s.Actions = normalizeValues(s.Actions, func(value Action) Action {
			return Action(str.String(value).Normalized())
		})
	}
	if len(s.Effects) > 0 {
		s.Effects = normalizeEffects(s.Effects)
	}
	if len(s.TargetPrefixes) > 0 {
		s.TargetPrefixes = normalizeStrings(s.TargetPrefixes)
	}

	for _, resource := range s.Resources {
		if !isValidResource(resource, false) {
			return PermissionScope{}, errors.New("permission scope contains an invalid resource")
		}
	}
	for _, action := range s.Actions {
		if !isValidAction(action, false) {
			return PermissionScope{}, errors.New("permission scope contains an invalid action")
		}
	}
	for _, effect := range s.Effects {
		if !isValidEffect(effect) {
			return PermissionScope{}, errors.New("permission scope contains an invalid effect")
		}
	}
	if !s.Restricted && (len(s.Resources) > 0 || len(s.Actions) > 0 || len(s.Effects) > 0 || len(s.TargetPrefixes) > 0) {
		return PermissionScope{}, errors.New("permission scope constraints require restricted mode")
	}

	return s, nil
}

func (s PermissionScope) Allows(operation Operation) bool {
	s, err := s.Normalize()
	if err != nil {
		return false
	}
	if !s.Restricted {
		return true
	}
	operation, err = operation.Normalize()
	if err != nil {
		return false
	}
	if len(s.Resources) == 0 || !slices.Contains(s.Resources, operation.Resource) {
		return false
	}
	if len(s.Actions) == 0 || !slices.Contains(s.Actions, operation.Action) {
		return false
	}
	for _, effect := range operation.Effects {
		if !slices.Contains(s.Effects, effect) {
			return false
		}
	}

	return matchesTargetPrefix(s.TargetPrefixes, operation.Target)
}

func IntersectScopes(parent PermissionScope, delegated PermissionScope) (PermissionScope, error) {
	parent, err := parent.Normalize()
	if err != nil {
		return PermissionScope{}, err
	}
	delegated, err = delegated.Normalize()
	if err != nil {
		return PermissionScope{}, err
	}
	if !delegated.Restricted {
		return PermissionScope{}, errors.New("delegated permission scope must be restricted")
	}
	if !parent.Restricted {
		return delegated, nil
	}

	return PermissionScope{
		Restricted:     true,
		Resources:      intersectValues(parent.Resources, delegated.Resources),
		Actions:        intersectValues(parent.Actions, delegated.Actions),
		Effects:        intersectValues(parent.Effects, delegated.Effects),
		TargetPrefixes: intersectTargetPrefixes(parent.TargetPrefixes, delegated.TargetPrefixes),
	}, nil
}

func DelegateAuthorization(
	parent AuthorizationContext,
	childID string,
	runID string,
	delegated PermissionScope,
) (AuthorizationContext, error) {
	parent, err := parent.Normalize()
	if err != nil {
		return AuthorizationContext{}, err
	}
	childID = str.String(childID).Trim()
	runID = str.String(runID).Trim()
	if childID == "" || runID == "" {
		return AuthorizationContext{}, errors.New("delegated actor and run ids are required")
	}
	scope, err := IntersectScopes(parent.Scope, delegated)
	if err != nil {
		return AuthorizationContext{}, err
	}

	return AuthorizationContext{
		Actor:           Actor{Kind: ActorSubagent, ID: childID},
		SurfaceKind:     parent.SurfaceKind,
		Surface:         parent.Surface,
		Profile:         parent.Profile,
		SessionID:       parent.SessionID,
		RunID:           runID,
		ParentActorKind: parent.Actor.Kind,
		ParentActorID:   parent.Actor.ID,
		ParentRunID:     parent.RunID,
		Scope:           scope,
	}.Normalize()
}

func intersectValues[T comparable](left []T, right []T) []T {
	result := make([]T, 0)
	for _, value := range left {
		if slices.Contains(right, value) {
			result = append(result, value)
		}
	}

	return result
}

func intersectTargetPrefixes(left []string, right []string) []string {
	if len(left) == 0 {
		return append([]string(nil), right...)
	}
	if len(right) == 0 {
		return append([]string(nil), left...)
	}

	result := make([]string, 0)
	for _, leftPrefix := range left {
		for _, rightPrefix := range right {
			prefix := ""
			switch {
			case strings.HasPrefix(leftPrefix, rightPrefix):
				prefix = leftPrefix
			case strings.HasPrefix(rightPrefix, leftPrefix):
				prefix = rightPrefix
			}
			if prefix != "" && !slices.Contains(result, prefix) {
				result = append(result, prefix)
			}
		}
	}
	slices.Sort(result)

	return result
}
