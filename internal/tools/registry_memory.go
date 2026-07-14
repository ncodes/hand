package tools

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"sync"

	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/trace"
	"github.com/wandxy/morph/pkg/str"
)

type RegistryOptions struct {
	PermissionPolicy permissions.Policy
}

// InMemoryRegistry is a registry that stores tools in memory.
type InMemoryRegistry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
	groups      map[string]Group
	permissions permissions.Policy
}

// NewInMemoryRegistry creates a new InMemoryRegistry.
func NewInMemoryRegistry(options ...RegistryOptions) *InMemoryRegistry {
	var opts RegistryOptions
	if len(options) > 0 {
		opts = options[0]
	}
	opts.PermissionPolicy.Normalize()

	return &InMemoryRegistry{
		definitions: make(map[string]Definition),
		groups:      make(map[string]Group),
		permissions: opts.PermissionPolicy,
	}
}

// Register registers a new tool in the registry.
func (r *InMemoryRegistry) Register(def Definition) error {
	if r == nil {
		return errors.New("tool registry is required")
	}
	nameValue := str.String(def.Name)
	def.Name = nameValue.Trim()
	if def.Name == "" {
		return errors.New("tool name is required")
	}

	if def.Handler == nil {
		return errors.New("tool handler is required")
	}
	def.Groups = normalizeNames(def.Groups)
	def.Platforms = normalizeNames(def.Platforms)
	if !def.Permission.IsZero() {
		if str.String(def.Permission.Tool).Trim() == "" {
			def.Permission.Tool = def.Name
		}
		permission, err := def.Permission.Normalize()
		if err != nil {
			return err
		}
		def.Permission = permission
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.definitions[def.Name]; exists {
		return errors.New("tool is already registered")
	}

	r.definitions[def.Name] = def
	return nil
}

func (r *InMemoryRegistry) RegisterGroup(group Group) error {
	if r == nil {
		return errors.New("tool registry is required")
	}
	nameValue2 := str.String(group.Name)
	group.Name = nameValue2.Trim()
	if group.Name == "" {
		return errors.New("tool group name is required")
	}
	group.Tools = normalizeNames(group.Tools)
	group.Includes = normalizeNames(group.Includes)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.groups[group.Name]; exists {
		return errors.New("tool group is already registered")
	}

	r.groups[group.Name] = group
	return nil
}

func (r *InMemoryRegistry) Get(name string) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	nameValue3 := str.String(name)
	def, ok := r.definitions[nameValue3.Trim()]
	return def, ok
}

func (r *InMemoryRegistry) GetGroup(name string) (Group, bool) {
	if r == nil {
		return Group{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	nameValue4 := str.String(name)
	group, ok := r.groups[nameValue4.Trim()]
	return group, ok
}

func (r *InMemoryRegistry) List() Definitions {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make(Definitions, 0, len(r.definitions))
	for _, def := range r.definitions {
		definitions = append(definitions, def)
	}

	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})

	return definitions
}

func (r *InMemoryRegistry) ListGroups() []Group {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	groups := make([]Group, 0, len(r.groups))
	for _, group := range r.groups {
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})

	return groups
}

func (r *InMemoryRegistry) Resolve(opts Policy) (Definitions, error) {
	if r == nil {
		return nil, errors.New("tool registry is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(opts.GroupNames) == 0 {
		return filterDefinitions(sortedDefinitions(r.definitions), opts), nil
	}

	selected := make(map[string]Definition)
	resolvedGroups := make(map[string]bool)
	for _, rawName := range normalizeNames(opts.GroupNames) {
		if err := r.resolveGroup(rawName, nil, resolvedGroups, selected); err != nil {
			return nil, err
		}
	}

	definitions := make(Definitions, 0, len(selected))
	for _, def := range selected {
		definitions = append(definitions, def)
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})

	return filterDefinitions(definitions, opts), nil
}

func (r *InMemoryRegistry) Invoke(ctx context.Context, call Call) (Result, error) {
	def, ok := r.Get(call.Name)
	if !ok {
		return Result{Error: Error{Code: "tool_not_registered", Message: "tool is not registered"}.String()}, nil
	}
	r.observePermission(ctx, def)

	result, err := def.Handler.Invoke(ctx, call)
	if err != nil {
		result.Error = Error{Code: "tool_invocation_failed", Message: err.Error()}.String()
		return result, nil
	}
	errorValue := str.String(result.Error)
	if errorValue.Trim() != "" {
		errorValue2 := str.String(result.Error)
		result.Error = normalizeResultError(errorValue2.Trim())
	}

	return result, nil
}

func (r *InMemoryRegistry) observePermission(ctx context.Context, definition Definition) {
	if definition.Permission.IsZero() {
		return
	}

	authorization, ok := permissions.FromContext(ctx)
	if !ok {
		authorization = permissions.AuthorizationContext{
			Actor:       permissions.Actor{Kind: permissions.ActorUnknown},
			SurfaceKind: permissions.SurfaceKindUnknown,
			Surface:     permissions.SurfaceUnknown,
		}
	}
	evaluation := r.permissions.Evaluate(permissions.EvaluationInput{
		Authorization: authorization,
		Operation:     definition.Permission,
	})
	recorder := TraceRecorderFromContext(ctx)
	if recorder == nil {
		return
	}

	effects := make([]string, len(definition.Permission.Effects))
	for index, effect := range definition.Permission.Effects {
		effects[index] = string(effect)
	}
	recorder.Record(trace.EvtPermissionDecisionObserved, trace.PermissionDecisionPayload{
		ActorKind:     string(authorization.Actor.Kind),
		SurfaceKind:   string(authorization.SurfaceKind),
		Surface:       string(authorization.Surface),
		Tool:          definition.Permission.Tool,
		Resource:      string(definition.Permission.Resource),
		Action:        string(definition.Permission.Action),
		Effects:       effects,
		Decision:      string(evaluation.Decision),
		ReasonCode:    evaluation.ReasonCode,
		Rule:          evaluation.Rule,
		Mode:          string(evaluation.Mode),
		OwnerRequired: definition.Permission.OwnerRequired,
	})
}

func (r *InMemoryRegistry) resolveGroup(
	name string,
	stack []string,
	resolved map[string]bool,
	selected map[string]Definition,
) error {
	if resolved[name] {
		return nil
	}

	group, ok := r.groups[name]
	if !ok {
		return errors.New("tool group ('" + name + "') is not registered")
	}

	if slices.Contains(stack, name) {
		return errors.New("tool group ('" + name + "') cycle detected")
	}
	stack = append(stack, name)

	for _, included := range group.Includes {
		if err := r.resolveGroup(included, stack, resolved, selected); err != nil {
			return err
		}
	}

	for _, toolName := range group.Tools {
		def, ok := r.definitions[toolName]
		if !ok {
			return errors.New("tool ('" + toolName + "') referenced by group is not registered")
		}
		selected[toolName] = def
	}

	for _, def := range r.definitions {
		if slices.Contains(def.Groups, name) {
			selected[def.Name] = def
		}
	}

	resolved[name] = true
	return nil
}

func sortedDefinitions(definitions map[string]Definition) Definitions {
	list := make(Definitions, 0, len(definitions))
	for _, def := range definitions {
		list = append(list, def)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

func filterDefinitions(definitions Definitions, opts Policy) Definitions {
	filtered := make(Definitions, 0, len(definitions))
	platformValue := str.String(opts.Platform)
	platform := platformValue.Trim()
	for _, def := range definitions {
		if !opts.Capabilities.Supports(def.Requires) {
			continue
		}
		if platform != "" && !matchesPlatform(def.Platforms, platform) {
			continue
		}
		filtered = append(filtered, def)
	}

	return filtered
}

func matchesPlatform(platforms []string, platform string) bool {
	if len(platforms) == 0 {
		return true
	}

	return slices.Contains(platforms, platform)
}

func normalizeNames(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		valueText := str.String(value).Trim()
		if valueText == "" {
			continue
		}
		if _, ok := seen[valueText]; ok {
			continue
		}
		seen[valueText] = struct{}{}
		normalized = append(normalized, valueText)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeResultError(raw string) string {
	var toolErr Error
	if err := json.Unmarshal([]byte(raw), &toolErr); err == nil {
		code := str.String(toolErr.Code)
		message := str.String(toolErr.Message)
		if code.Trim() != "" && message.Trim() != "" {
			return raw
		}
	}

	return Error{Code: "tool_failed", Message: raw}.String()
}
