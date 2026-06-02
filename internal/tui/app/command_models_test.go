package tui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

func TestModel_StartModelsCommandLoadsModels(t *testing.T) {
	client := &fakeTUIChatClient{modelList: rpcclient.ModelList{
		Provider: "openai",
		AuthType: "oauth",
		Models: []rpcclient.ModelOption{
			{ID: "gpt-5.4-mini", Name: "GPT 5.4 Mini", Current: true, Reasoning: true, ContextWindow: 272000},
			{ID: "gpt-5.4", Name: "GPT 5.4", ContextWindow: 272000},
		},
	}}
	runModel := newModelWithClient(client)
	runModel.input.SetValue("/models")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	require.NotNil(t, cmd)
	msg := modelsLoadedMessageFromBatch(t, cmd)
	updated, cmd = updated.(model).Update(msg)

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, client.listModelCalls)
	require.Equal(t, commandViewKindModels, runModel.commandView.Kind)
	require.Equal(t, "Models", runModel.commandView.TitleLeft)
	require.Equal(t, "enter to select · esc to close", runModel.commandView.TitleRight)
	require.Equal(t, "openai", runModel.commandView.ModelProvider)
	require.Equal(t, "oauth", runModel.commandView.ModelAuthType)
	require.Len(t, runModel.commandView.Models, 2)
	require.Equal(t, "gpt-5.4-mini", runModel.commandView.Models[0].ID)
}

func TestLoadModelsCmdUsesBackgroundContextWhenMissing(t *testing.T) {
	client := &fakeTUIChatClient{modelList: rpcclient.ModelList{
		Provider: "openai",
		Models:   []rpcclient.ModelOption{{ID: "gpt-4o"}},
	}}

	msg := loadModelsCmd(nil, client)()

	require.Equal(t, 1, client.listModelCalls)
	require.Equal(t, "openai", msg.(modelsLoadedMsg).List.Provider)
	require.Equal(t, "gpt-4o", msg.(modelsLoadedMsg).List.Models[0].ID)
}

func TestModel_RenderModelsCommandViewHighlightsCurrentAndSelection(t *testing.T) {
	runModel := newModel()
	runModel.showCommandView(commandViewPayload{
		Kind: commandViewKindModels,
		Models: []rpcclient.ModelOption{
			{ID: "gpt-5.4-mini", Name: "GPT 5.4 Mini", Current: true, Reasoning: true, ContextWindow: 272000},
			{ID: "gpt-4o", Name: "GPT 4o", ContextWindow: 128000},
		},
	})

	content := stripANSI(runModel.renderModelsCommandViewContent(commandViewContent{Width: 48, Height: 2}))

	require.Contains(t, content, "GPT 5.4 Mini")
	require.Contains(t, content, "current")
	require.Contains(t, content, "reasoning")
	require.Contains(t, content, "272k")
	require.Contains(t, content, "GPT 4o")
}

func TestModel_RenderModelsCommandViewHandlesEmptyAndFallbackDetails(t *testing.T) {
	runModel := newModel()
	runModel.showCommandView(commandViewPayload{Kind: commandViewKindModels})
	require.Equal(t, "No models available.", runModel.renderModelsCommandViewContent(commandViewContent{}))

	row := stripANSI(renderModelsCommandRow(rpcclient.ModelOption{ID: "model-a", API: "openai-responses"}, 1))
	require.Empty(t, row)

	row = stripANSI(renderModelsCommandRow(rpcclient.ModelOption{ID: "model-a", API: "openai-responses"}, 32))
	require.Contains(t, row, "model-a")
	require.Contains(t, row, "openai-responses")
}

func TestModel_SelectModelCallsClientHidesCommandViewAndUpdatesChrome(t *testing.T) {
	client := &fakeTUIChatClient{
		selectedModel: rpcclient.ModelOption{ID: "gpt-4o", Provider: "openai", Current: true},
	}
	runModel := newModelWithClient(client)
	runModel.showCommandView(commandViewPayload{
		Kind: commandViewKindModels,
		Models: []rpcclient.ModelOption{
			{ID: "gpt-5.4-mini", Current: true},
			{ID: "gpt-4o"},
		},
	})
	runModel.commandViewItemSelected = 1

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.commandView.Visible)

	msg := modelSelectedMessageFromBatch(t, cmd)
	updated, cmd = runModel.Update(msg)

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, client.selectModelCalls)
	require.Equal(t, "gpt-4o", client.selectedModelID)
	require.False(t, runModel.commandView.Visible)
	require.Equal(t, "gpt-4o", runModel.runtimeInfo.Model)
	require.Equal(t, "openai", runModel.runtimeInfo.Provider)
	require.Equal(t, "model selected; daemon restarting", runModel.status.Text())
}

func TestModel_UpdateModelsCommandViewNavigatesSelection(t *testing.T) {
	runModel := newModel()
	runModel.showCommandView(commandViewPayload{
		Kind: commandViewKindModels,
		Models: []rpcclient.ModelOption{
			{ID: "model-a"},
			{ID: "model-b"},
			{ID: "model-c"},
		},
	})

	updated, cmd := runModel.updateModelsCommandView(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, runModel.commandViewItemSelected)

	updated, cmd = runModel.updateModelsCommandView(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 2, runModel.commandViewItemSelected)

	updated, cmd = runModel.updateModelsCommandView(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, runModel.commandViewItemSelected)

	updated, cmd = runModel.updateModelsCommandView(tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Zero(t, runModel.commandViewItemSelected)

	updated, cmd = runModel.updateModelsCommandView(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 2, runModel.commandViewItemSelected)

	updated, cmd = runModel.updateModelsCommandView(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Zero(t, runModel.commandViewItemSelected)

	updated, cmd = runModel.updateModelsCommandView(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Greater(t, runModel.commandViewItemSelected, 0)

	updated, cmd = runModel.updateModelsCommandView(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, runModel.commandViewItemSelected)

	updated, cmd = runModel.updateModelsCommandView(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseLeft}))
	require.Nil(t, cmd)
	require.Equal(t, runModel.commandViewItemSelected, updated.(model).commandViewItemSelected)

	updated, cmd = runModel.updateModelsCommandView(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	require.Nil(t, cmd)
	require.Equal(t, runModel.commandViewItemSelected, updated.(model).commandViewItemSelected)

	updated, cmd = runModel.updateModelsCommandView(struct{}{})
	require.Nil(t, cmd)
	require.Equal(t, runModel.commandViewItemSelected, updated.(model).commandViewItemSelected)
}

func TestModel_UpdateModelsCommandViewHandlesEmptyListAndSelectionMessage(t *testing.T) {
	runModel := newModel()
	runModel.showCommandView(commandViewPayload{Kind: commandViewKindModels})

	updated, cmd := runModel.updateModelsCommandView(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	require.Nil(t, cmd)
	require.Zero(t, updated.(model).commandViewItemSelected)

	runModel.showCommandView(commandViewPayload{
		Kind:   commandViewKindModels,
		Models: []rpcclient.ModelOption{{ID: "gpt-4o"}},
	})
	updated, cmd = runModel.updateModelsCommandView(modelSelectedMsg{
		Model: rpcclient.ModelOption{ID: "gpt-4o", Provider: "openai"},
	})
	require.NotNil(t, cmd)
	require.False(t, updated.(model).commandView.Visible)
	require.Equal(t, "gpt-4o", updated.(model).runtimeInfo.Model)
	require.Equal(t, "openai", updated.(model).runtimeInfo.Provider)
}

func TestModel_ModelsCommandReportsUnavailableStates(t *testing.T) {
	runModel := newModel()
	require.NotNil(t, runModel.startModelsCommand())
	require.Equal(t, "models unavailable", runModel.status.Text())

	client := &fakeTUIChatClient{modelListErr: errors.New("load failed")}
	runModel = newModelWithClient(client)
	msg := loadModelsCmd(context.Background(), client)()
	cmd := runModel.completeModelsCommand(msg.(modelsLoadedMsg))
	require.NotNil(t, cmd)
	require.Equal(t, "models unavailable", runModel.status.Text())

	client = &fakeTUIChatClient{selectModelErr: errors.New("select failed")}
	runModel = newModelWithClient(client)
	runModel.showCommandView(commandViewPayload{
		Kind:   commandViewKindModels,
		Models: []rpcclient.ModelOption{{ID: "gpt-4o"}},
	})
	msg = selectModelCmd(context.Background(), client, "gpt-4o")()
	updated, cmd := runModel.updateModelsCommandView(msg)
	require.NotNil(t, cmd)
	require.Equal(t, "model selection unavailable", updated.(model).status.Text())
}

func TestModel_ModelSelectionEdgeCases(t *testing.T) {
	runModel := newModel()
	runModel.showCommandView(commandViewPayload{
		Kind:   commandViewKindModels,
		Models: []rpcclient.ModelOption{{ID: "gpt-4o", Current: true}},
	})
	updated, cmd := runModel.selectCurrentModelOption()
	require.NotNil(t, cmd)
	require.Equal(t, "model already selected", updated.(model).status.Text())

	runModel = newModel()
	runModel.showCommandView(commandViewPayload{
		Kind:   commandViewKindModels,
		Models: []rpcclient.ModelOption{{}},
	})
	updated, cmd = runModel.selectCurrentModelOption()
	require.NotNil(t, cmd)
	require.Equal(t, "model selection unavailable", updated.(model).status.Text())

	runModel.showCommandView(commandViewPayload{
		Kind:   commandViewKindModels,
		Models: []rpcclient.ModelOption{{ID: "gpt-4o"}},
	})
	updated, cmd = runModel.selectCurrentModelOption()
	require.NotNil(t, cmd)
	require.Equal(t, "model selection unavailable", updated.(model).status.Text())

	require.Nil(t, selectModelCmd(context.Background(), nil, "gpt-4o"))
	client := &fakeTUIChatClient{selectedModel: rpcclient.ModelOption{ID: "gpt-4o"}}
	msg := selectModelCmd(nil, client, "gpt-4o")()
	require.NoError(t, msg.(modelSelectedMsg).Err)
	require.Equal(t, "gpt-4o", msg.(modelSelectedMsg).Model.ID)

	msg = selectModelCmd(context.Background(), &fakeTUIChatClient{}, "")()
	require.EqualError(t, msg.(modelSelectedMsg).Err, "model id is required")

	updated, cmd = runModel.completeSelectModel(modelSelectedMsg{})
	require.NotNil(t, cmd)
	require.Equal(t, "model selection unavailable", updated.(model).status.Text())

	require.Equal(t, []rpcclient.ModelOption{{ID: "a"}}, orderModelsCommandOptions([]rpcclient.ModelOption{{ID: "a"}}))
}

func modelsLoadedMessageFromBatch(t *testing.T, cmd tea.Cmd) modelsLoadedMsg {
	t.Helper()

	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)
	require.Len(t, batch, 2)

	msg, ok := batch[1]().(modelsLoadedMsg)
	require.True(t, ok)

	return msg
}

func modelSelectedMessageFromBatch(t *testing.T, cmd tea.Cmd) modelSelectedMsg {
	t.Helper()

	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)
	require.Len(t, batch, 2)

	msg, ok := batch[1]().(modelSelectedMsg)
	require.True(t, ok)

	return msg
}
