package permissions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePreset_NormalizesSupportedNames(t *testing.T) {
	tests := []struct {
		input string
		want  Preset
	}{
		{input: " ask ", want: PresetAskForApproval},
		{input: "APPROVE", want: PresetApproveForMe},
		{input: "full-access", want: PresetFullAccess},
		{input: "full_access", want: PresetFullAccess},
		{input: "custom", want: PresetCustom},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			preset, err := ParsePreset(test.input)
			require.NoError(t, err)
			require.Equal(t, test.want, preset)
			require.NotEmpty(t, preset.Label())
			require.NotEmpty(t, preset.Description())
		})
	}

	_, err := ParsePreset("unsafe")
	require.EqualError(t, err, "permission preset must be one of: ask, approve, full_access, custom")
	require.Empty(t, Preset("invalid").Label())
	require.Empty(t, Preset("invalid").Description())
}

func TestPolicy_AskForApprovalPresetClassifiesOperations(t *testing.T) {
	policy := Policy{Preset: PresetAskForApproval}
	authorization := AuthorizationContext{
		Actor: Actor{Kind: ActorLocalOwner}, Surface: SurfaceTUI,
	}
	tests := []struct {
		name      string
		operation Operation
		want      Decision
	}{
		{
			name: "workspace write",
			operation: Operation{
				Tool: "write_file", Resource: ResourceFile, Action: ActionUpdate, Effects: []Effect{EffectWrite},
				TargetScope: TargetScopeWorkspace,
			},
			want: DecisionAllow,
		},
		{
			name: "external write",
			operation: Operation{
				Tool: "write_file", Resource: ResourceFile, Action: ActionUpdate, Effects: []Effect{EffectWrite},
				TargetScope: TargetScopeExternal,
			},
			want: DecisionAsk,
		},
		{
			name: "network",
			operation: Operation{
				Tool: "web_extract", Resource: ResourceNetwork, Action: ActionRead, Effects: []Effect{EffectNetwork},
			},
			want: DecisionAsk,
		},
		{
			name: "command execution",
			operation: Operation{
				Tool: "run_command", Resource: ResourceProcess, Action: ActionExecute, Effects: []Effect{EffectExecution},
			},
			want: DecisionAsk,
		},
		{
			name: "destructive",
			operation: Operation{
				Tool: "memory_delete", Resource: ResourceMemory, Action: ActionDelete,
				Effects: []Effect{EffectDestructive},
			},
			want: DecisionAsk,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evaluation := policy.Evaluate(EvaluationInput{
				Authorization: authorization,
				Operation:     test.operation,
			})
			require.Equal(t, test.want, evaluation.Decision)
			require.Equal(t, PresetAskForApproval, evaluation.Preset)
		})
	}

	evaluation := policy.Evaluate(EvaluationInput{
		Authorization: AuthorizationContext{
			Actor: Actor{Kind: ActorGatewayUser}, Surface: SurfaceTelegram,
		},
		Operation: Operation{
			Resource: ResourceFile, Action: ActionRead, Effects: []Effect{EffectRead},
		},
	})
	require.Equal(t, DecisionDeny, evaluation.Decision)

	evaluation = policy.Evaluate(EvaluationInput{
		Authorization: AuthorizationContext{
			Actor: Actor{Kind: ActorRPCClient}, Surface: SurfaceTUI,
		},
		Operation: Operation{
			Tool: "read_file", Resource: ResourceFile, Action: ActionRead, Effects: []Effect{EffectRead},
		},
	})
	require.Equal(t, DecisionDeny, evaluation.Decision)

	evaluation = policy.Evaluate(EvaluationInput{
		Authorization: authorization,
		Operation: Operation{
			Resource: ResourceConfiguration, Action: ActionUpdate,
			Effects: []Effect{EffectCredentialBearing},
		},
	})
	require.Equal(t, DecisionAllow, evaluation.Decision)
}

func TestPolicy_ApproveForMePresetAsksForExternalWrites(t *testing.T) {
	policy := Policy{Preset: PresetApproveForMe}
	authorization := AuthorizationContext{
		Actor: Actor{Kind: ActorLocalOwner}, Surface: SurfaceCLI,
	}

	externalWrite := policy.Evaluate(EvaluationInput{
		Authorization: authorization,
		Operation: Operation{
			Tool: "write_file", Resource: ResourceFile, Action: ActionUpdate, Effects: []Effect{EffectWrite},
			TargetScope: TargetScopeExternal,
		},
	})
	require.Equal(t, DecisionAsk, externalWrite.Decision)

	networkRead := policy.Evaluate(EvaluationInput{
		Authorization: authorization,
		Operation: Operation{
			Tool: "web_extract", Resource: ResourceNetwork, Action: ActionRead,
			Effects: []Effect{EffectNetwork, EffectExternalSystem},
		},
	})
	require.Equal(t, DecisionAllow, networkRead.Decision)

	evaluation := policy.Evaluate(EvaluationInput{
		Authorization: authorization,
		Operation: Operation{
			Tool: "process", Resource: ResourceProcess, Action: ActionStop,
			Effects: []Effect{EffectExecution, EffectDestructive},
		},
	})
	require.Equal(t, DecisionAsk, evaluation.Decision)
}

func TestPolicy_EffectivePreservesCustomPolicy(t *testing.T) {
	custom := Policy{
		Preset: PresetCustom,
		Rules: []Rule{{
			Name: "deny writes", Effects: []Effect{EffectWrite}, Decision: DecisionDeny,
		}},
	}
	require.Equal(t, DecisionDeny, custom.Evaluate(EvaluationInput{
		Authorization: AuthorizationContext{
			Actor: Actor{Kind: ActorLocalOwner}, Surface: SurfaceCLI,
		},
		Operation: Operation{
			Resource: ResourceFile, Action: ActionUpdate, Effects: []Effect{EffectWrite},
		},
	}).Decision)

	require.Equal(t, PresetCustom, custom.ForPreset("invalid").Preset)

	invalidTargetScope := Policy{Rules: []Rule{{
		Name:         "invalid target scope",
		TargetScopes: []TargetScope{"computer"},
		Decision:     DecisionAllow,
	}}}
	require.EqualError(t, invalidTargetScope.Validate(), "permission rule contains an invalid target scope")
}

func TestEngine_ContextPresetOverridesProfilePreset(t *testing.T) {
	engine := NewEngine(Policy{Preset: PresetCustom, Default: DecisionDeny})
	ctx := WithContext(context.Background(), AuthorizationContext{
		Actor: Actor{Kind: ActorLocalOwner}, Surface: SurfaceCLI,
	})
	ctx = WithPreset(ctx, PresetApproveForMe)

	evaluation, err := engine.Check(ctx, EvaluationInput{Operation: Operation{
		Resource: ResourceFile, Action: ActionRead, Effects: []Effect{EffectRead},
	}})

	require.NoError(t, err)
	require.Equal(t, PresetApproveForMe, engine.Preset(ctx))
	require.Equal(t, PresetApproveForMe, evaluation.Preset)
	require.Equal(t, DecisionAllow, evaluation.Decision)
}
