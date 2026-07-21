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
		reason    string
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
			want:   DecisionAsk,
			reason: "This action connects to an online service to send or receive data.",
		},
		{
			name: "browser network",
			operation: Operation{
				Tool: "browser", Resource: ResourceBrowser, Action: ActionUpdate,
				Effects: []Effect{EffectWrite, EffectNetwork, EffectExternalSystem},
			},
			want:   DecisionAsk,
			reason: "This browser action may load content from or send data to a website.",
		},
		{
			name: "command execution",
			operation: Operation{
				Tool: "run_command", Resource: ResourceProcess, Action: ActionExecute, Effects: []Effect{EffectExecution},
			},
			want:   DecisionAsk,
			reason: "This action runs a program on your computer.",
		},
		{
			name: "destructive",
			operation: Operation{
				Tool: "memory_delete", Resource: ResourceMemory, Action: ActionDelete,
				Effects: []Effect{EffectDestructive},
			},
			want:   DecisionAsk,
			reason: "This action can permanently delete or overwrite data.",
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
			if test.reason != "" {
				require.Equal(t, test.reason, evaluation.Reason)
			}
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

func TestPolicy_PresetUsesConfiguredRulesAsHigherPriorityOverlay(t *testing.T) {
	automation := AuthorizationContext{
		Actor: Actor{Kind: ActorAutomation, ID: "auto_news"}, Surface: SurfaceAutomation,
	}
	localOwner := AuthorizationContext{
		Actor: Actor{Kind: ActorLocalOwner}, Surface: SurfaceTUI,
	}
	networkRead := Operation{
		Tool: "web_extract", Resource: ResourceNetwork, Action: ActionRead,
		Effects: []Effect{EffectRead, EffectNetwork, EffectExternalSystem},
	}

	tests := []struct {
		name          string
		policy        Policy
		authorization AuthorizationContext
		want          Decision
		rule          string
	}{
		{
			name: "allow rule opens automation denied by preset default",
			policy: Policy{Preset: PresetApproveForMe, Rules: []Rule{{
				Name: "allow news automation", ActorKinds: []ActorKind{ActorAutomation},
				ActorIDs: []string{"auto_news"}, Tools: []string{"web_extract"}, Decision: DecisionAllow,
			}}},
			authorization: automation,
			want:          DecisionAllow,
			rule:          "allow news automation",
		},
		{
			name: "allow rule overrides built in ask",
			policy: Policy{Preset: PresetAskForApproval, Rules: []Rule{{
				Name: "allow local news", ActorKinds: []ActorKind{ActorLocalOwner},
				Tools: []string{"web_extract"}, Effects: []Effect{EffectNetwork}, Decision: DecisionAllow,
			}}},
			authorization: localOwner,
			want:          DecisionAllow,
			rule:          "allow local news",
		},
		{
			name: "deny rule overrides preset behavior",
			policy: Policy{Preset: PresetApproveForMe, Rules: []Rule{{
				Name: "deny automation network", ActorKinds: []ActorKind{ActorAutomation},
				Effects: []Effect{EffectNetwork}, Decision: DecisionDeny,
			}}},
			authorization: automation,
			want:          DecisionDeny,
			rule:          "deny automation network",
		},
		{
			name: "nonmatching rule falls through to preset default",
			policy: Policy{Preset: PresetApproveForMe, Rules: []Rule{{
				Name: "allow another automation", ActorIDs: []string{"auto_other"}, Decision: DecisionAllow,
			}}},
			authorization: automation,
			want:          DecisionDeny,
		},
		{
			name: "nonmatching rule falls through to built in preset rule",
			policy: Policy{Preset: PresetAskForApproval, Rules: []Rule{{
				Name: "allow commands", Tools: []string{"run_command"}, Decision: DecisionAllow,
			}}},
			authorization: localOwner,
			want:          DecisionAsk,
			rule:          "preset.ask.network",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evaluation := test.policy.Evaluate(EvaluationInput{
				Authorization: test.authorization,
				Operation:     networkRead,
			})

			require.Equal(t, test.want, evaluation.Decision)
			require.Equal(t, test.rule, evaluation.Rule)
		})
	}
}

func TestPolicy_LabelMarksRuleEnhancedPresetsAsCustomized(t *testing.T) {
	rules := []Rule{{Name: "rule", Decision: DecisionAllow}}

	require.Equal(t, "Ask for approval (customized)", (Policy{Preset: PresetAskForApproval, Rules: rules}).Label())
	require.Equal(t, "Approve for me (customized)", (Policy{Preset: PresetApproveForMe, Rules: rules}).Label())
	require.Equal(t, "Full access", (Policy{Preset: PresetFullAccess, Rules: rules}).Label())
	require.Equal(t, "Custom", (Policy{Preset: PresetCustom, Rules: rules}).Label())
	require.Equal(t, "Ask for approval", (Policy{Preset: PresetAskForApproval}).Label())
	require.Equal(t, "Approve for me", (Policy{Preset: PresetApproveForMe}).Label())
	require.Empty(t, (Policy{Preset: "invalid"}).Label())
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
	engine := NewEngine(Policy{Preset: PresetCustom, Default: DecisionDeny, Rules: []Rule{{
		Name: "allow reads", Resources: []Resource{ResourceFile}, Actions: []Action{ActionRead}, Decision: DecisionAllow,
	}}})
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
	require.Equal(t, "allow reads", evaluation.Rule)
}
