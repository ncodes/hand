package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/permissions"
)

func TestLoad_ParsesPermissionPolicy(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
permissions:
  mode: observe
  default: deny
  surfaceKinds:
    gateway: deny
  surfaces:
    cli: ask
  rules:
    - name: owner workspace writes
      profiles: [work]
      actors: [local_owner]
      surfaceKinds: [local]
      surfaces: [cli]
      tools: [write_file]
      resources: [file]
      actions: [update]
      effects: [write]
      targetPrefixes: [workspace/]
      decision: allow
      reason: trusted workspace write
`), 0o600))

	cfg, err := Load("", configPath)

	require.NoError(t, err)
	require.Equal(t, permissions.DecisionDeny, cfg.Permissions.SurfaceKindDefaults[permissions.SurfaceKindGateway])
	require.Equal(t, permissions.DecisionAsk, cfg.Permissions.SurfaceDefaults[permissions.SurfaceCLI])
	require.Equal(t, permissions.Rule{
		Name:           "owner workspace writes",
		Profiles:       []string{"work"},
		ActorKinds:     []permissions.ActorKind{permissions.ActorLocalOwner},
		SurfaceKinds:   []permissions.SurfaceKind{permissions.SurfaceKindLocal},
		Surfaces:       []permissions.Surface{permissions.SurfaceCLI},
		Tools:          []string{"write_file"},
		Resources:      []permissions.Resource{permissions.ResourceFile},
		Actions:        []permissions.Action{permissions.ActionUpdate},
		Effects:        []permissions.Effect{permissions.EffectWrite},
		TargetPrefixes: []string{"workspace/"},
		Decision:       permissions.DecisionAllow,
		Reason:         "trusted workspace write",
	}, cfg.Permissions.Rules[0])
}

func TestConfig_NormalizePermissions(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Permissions = permissions.Policy{
		SurfaceDefaults: map[permissions.Surface]permissions.Decision{" CLI ": " ASK "},
		Rules:           []permissions.Rule{{Name: " owner ", Decision: " ALLOW "}},
	}

	cfg.Normalize()
	require.Equal(t, permissions.ModeObserve, cfg.Permissions.Mode)
	require.Equal(t, permissions.DecisionDeny, cfg.Permissions.Default)
	require.Equal(t, permissions.DecisionAsk, cfg.Permissions.SurfaceDefaults[permissions.SurfaceCLI])
	require.Equal(t, "owner", cfg.Permissions.Rules[0].Name)
	require.NoError(t, cfg.Permissions.Validate())
}

func TestConfig_ValidateRejectsInvalidPermissions(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Permissions.Mode = "audit"

	err := cfg.ValidateRelaxed()
	require.EqualError(t, err, "permission mode must be one of: observe, enforce")
}

func TestConfig_ValidateAcceptsPermissionEnforcement(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Permissions.Mode = permissions.ModeEnforce

	require.NoError(t, cfg.ValidateRelaxed())
}

func TestNewDefaultConfig_ClonesPermissions(t *testing.T) {
	original := DefaultConfig.Permissions
	t.Cleanup(func() {
		DefaultConfig.Permissions = original
	})
	DefaultConfig.Permissions = permissions.Policy{
		SurfaceKindDefaults: map[permissions.SurfaceKind]permissions.Decision{
			permissions.SurfaceKindLocal: permissions.DecisionAsk,
		},
		SurfaceDefaults: map[permissions.Surface]permissions.Decision{permissions.SurfaceCLI: permissions.DecisionAsk},
		Rules: []permissions.Rule{{
			Name:           "owner",
			Profiles:       []string{"work"},
			ActorKinds:     []permissions.ActorKind{permissions.ActorLocalOwner},
			SurfaceKinds:   []permissions.SurfaceKind{permissions.SurfaceKindLocal},
			Surfaces:       []permissions.Surface{permissions.SurfaceCLI},
			Tools:          []string{"read_file"},
			Resources:      []permissions.Resource{permissions.ResourceFile},
			Actions:        []permissions.Action{permissions.ActionRead},
			Effects:        []permissions.Effect{permissions.EffectRead},
			TargetPrefixes: []string{"workspace/"},
			Decision:       permissions.DecisionAllow,
		}},
	}

	first := NewDefaultConfig()
	second := NewDefaultConfig()
	first.Permissions.SurfaceDefaults[permissions.SurfaceCLI] = permissions.DecisionDeny
	first.Permissions.SurfaceKindDefaults[permissions.SurfaceKindLocal] = permissions.DecisionDeny
	first.Permissions.Rules[0].Profiles[0] = "other"
	first.Permissions.Rules[0].ActorKinds[0] = permissions.ActorGatewayUser
	first.Permissions.Rules[0].SurfaceKinds[0] = permissions.SurfaceKindGateway
	first.Permissions.Rules[0].Surfaces[0] = permissions.SurfaceSlack
	first.Permissions.Rules[0].Tools[0] = "memory_search"
	first.Permissions.Rules[0].Resources[0] = permissions.ResourceMemory
	first.Permissions.Rules[0].Actions[0] = permissions.ActionDelete
	first.Permissions.Rules[0].Effects[0] = permissions.EffectDestructive
	first.Permissions.Rules[0].TargetPrefixes[0] = "outside/"

	require.Equal(t, permissions.DecisionAsk, second.Permissions.SurfaceDefaults[permissions.SurfaceCLI])
	require.Equal(t, permissions.DecisionAsk, second.Permissions.SurfaceKindDefaults[permissions.SurfaceKindLocal])
	require.Equal(t, "work", second.Permissions.Rules[0].Profiles[0])
	require.Equal(t, permissions.ActorLocalOwner, second.Permissions.Rules[0].ActorKinds[0])
	require.Equal(t, permissions.SurfaceKindLocal, second.Permissions.Rules[0].SurfaceKinds[0])
	require.Equal(t, permissions.SurfaceCLI, second.Permissions.Rules[0].Surfaces[0])
	require.Equal(t, "read_file", second.Permissions.Rules[0].Tools[0])
	require.Equal(t, permissions.ResourceFile, second.Permissions.Rules[0].Resources[0])
	require.Equal(t, permissions.ActionRead, second.Permissions.Rules[0].Actions[0])
	require.Equal(t, permissions.EffectRead, second.Permissions.Rules[0].Effects[0])
	require.Equal(t, "workspace/", second.Permissions.Rules[0].TargetPrefixes[0])
}
