package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

type modelListLoader interface {
	ListModels(context.Context) (rpcclient.ModelList, error)
}

type modelSelector interface {
	SelectModel(context.Context, string) (rpcclient.ModelOption, error)
}

type modelsLoadedMsg struct {
	List rpcclient.ModelList
	Err  error
}

type modelSelectedMsg struct {
	Model rpcclient.ModelOption
	Err   error
}

func (m *model) startModelsCommand() tea.Cmd {
	client, ok := m.modelClient.(modelListLoader)
	if m.modelClient == nil || !ok {
		return m.setStatus("models unavailable")
	}

	return tea.Batch(
		m.setStatus("loading models"),
		loadModelsCmd(m.chatCtx, client),
	)
}

func loadModelsCmd(ctx context.Context, client modelListLoader) tea.Cmd {
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		list, err := client.ListModels(ctx)
		return modelsLoadedMsg{List: list, Err: err}
	}
}

func (m *model) completeModelsCommand(msg modelsLoadedMsg) tea.Cmd {
	if msg.Err != nil {
		return m.setStatus("models unavailable")
	}

	m.showCommandView(commandViewPayload{
		TitleLeft:       "Models",
		TitleRight:      getModelsCommandTitleRight(),
		TitleRightColor: defaultTUITheme.MutedText,
		Kind:            commandViewKindModels,
		Models:          msg.List.Models,
		ModelProvider:   msg.List.Provider,
		ModelAuthType:   msg.List.AuthType,
	})

	return nil
}

func (m model) isModelsCommandView() bool {
	return m.commandView.Visible && m.commandView.Kind == commandViewKindModels
}

func (m model) renderModelsCommandViewContent(content commandViewContent) string {
	models := m.commandView.Models
	if len(models) == 0 {
		return "No models available."
	}

	offset := min(max(content.Offset, 0), max(len(models)-1, 0))
	height := max(content.Height, 1)
	end := min(offset+height, len(models))
	rows := make([]string, 0, end-offset)
	for index := offset; index < end; index++ {
		row := renderModelsCommandRow(models[index], content.Width)
		if index == m.commandViewItemSelected {
			row = renderSelectedChatsCommandRow(row, content.Width)
		} else if !models[index].Current {
			row = lipgloss.NewStyle().
				Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
				Render(row)
		}
		rows = append(rows, row)
	}

	for len(rows) <= height+1 {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}

func renderModelsCommandRow(model rpcclient.ModelOption, width int) string {
	width = max(width, 1)
	contentWidth := max(width-2, 1)
	name := getModelOptionDisplayName(model)
	detail := getModelOptionDetail(model)
	detailWidth := lipgloss.Width(detail)
	nameWidth := max(contentWidth-detailWidth-2, 1)
	name = truncateCommandMenuText(name, nameWidth)
	gap := max(contentWidth-lipgloss.Width(name)-detailWidth, 1)
	row := name + strings.Repeat(" ", gap) + detail
	if width <= 1 {
		return truncateChatsCommandRow(row, width)
	}

	return " " + truncateChatsCommandRow(row, contentWidth) + " "
}

func getModelOptionDisplayName(model rpcclient.ModelOption) string {
	if strings.TrimSpace(model.Name) != "" {
		return strings.TrimSpace(model.Name)
	}

	return strings.TrimSpace(model.ID)
}

func getModelOptionDetail(model rpcclient.ModelOption) string {
	parts := make([]string, 0, 3)
	if model.Current {
		parts = append(parts, "current")
	}
	if model.Reasoning {
		parts = append(parts, "reasoning")
	}
	if model.ContextWindow > 0 {
		parts = append(parts, fmt.Sprintf("%dk", model.ContextWindow/1000))
	}
	if len(parts) == 0 {
		return strings.TrimSpace(model.API)
	}

	return strings.Join(parts, " · ")
}

func (m *model) updateModelsCommandView(msg tea.Msg) (tea.Model, tea.Cmd) {
	if selected, ok := msg.(modelSelectedMsg); ok {
		return m.completeSelectModel(selected)
	}

	if len(m.commandView.Models) == 0 {
		return *m, nil
	}

	selection := m.commandViewItemSelected
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.Key().Code {
		case tea.KeyUp:
			selection--
		case tea.KeyDown:
			selection++
		case tea.KeyHome:
			selection = 0
		case tea.KeyEnd:
			selection = len(m.commandView.Models) - 1
		case tea.KeyPgUp:
			selection -= max(m.getCommandViewContentHeight(), 1)
		case tea.KeyPgDown:
			selection += max(m.getCommandViewContentHeight(), 1)
		case tea.KeyEnter:
			return m.selectCurrentModelOption()
		default:
			return *m, nil
		}
	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			selection--
		case tea.MouseWheelDown:
			selection++
		default:
			return *m, nil
		}
	default:
		return *m, nil
	}

	m.commandViewItemSelected = min(max(selection, 0), len(m.commandView.Models)-1)
	m.commandViewOffset = getChatsCommandViewOffsetForSelection(
		m.commandViewItemSelected,
		m.commandViewOffset,
		m.getCommandViewContentHeight(),
		len(m.commandView.Models),
	)
	m.clearCommandViewSelection()

	return *m, nil
}

func getModelsCommandTitleRight() string {
	return "enter to select · esc to close"
}

func (m *model) selectCurrentModelOption() (tea.Model, tea.Cmd) {
	model := m.commandView.Models[m.commandViewItemSelected]
	modelID := strings.TrimSpace(model.ID)
	if modelID == "" {
		return *m, m.setStatus("model selection unavailable")
	}
	if model.Current {
		return *m, m.setStatus("model already selected")
	}

	client, ok := m.modelClient.(modelSelector)
	if m.modelClient == nil || !ok {
		return *m, m.setStatus("model selection unavailable")
	}

	next := m.hideCommandView()
	statusCmd := next.setStatus("selecting model")
	return next, tea.Batch(
		statusCmd,
		selectModelCmd(m.chatCtx, client, modelID),
	)
}

func selectModelCmd(ctx context.Context, client modelSelector, modelID string) tea.Cmd {
	if client == nil {
		return nil
	}

	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			return modelSelectedMsg{Err: errors.New("model id is required")}
		}

		model, err := client.SelectModel(ctx, modelID)
		return modelSelectedMsg{Model: model, Err: err}
	}
}

func (m *model) completeSelectModel(msg modelSelectedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return *m, m.setStatus("model selection unavailable")
	}

	modelID := strings.TrimSpace(msg.Model.ID)
	if modelID == "" {
		return *m, m.setStatus("model selection unavailable")
	}

	for index := range m.commandView.Models {
		m.commandView.Models[index].Current = strings.TrimSpace(m.commandView.Models[index].ID) == modelID
	}
	m.commandView.Models = orderModelsCommandOptions(m.commandView.Models)
	m.commandViewItemSelected = 0
	m.commandViewOffset = 0
	m.modelName = getModelDisplayName(modelID)
	m.runtimeInfo.Model = modelID
	if provider := strings.TrimSpace(msg.Model.Provider); provider != "" {
		m.runtimeInfo.Provider = provider
	}

	next := m.hideCommandView()
	return next, next.setStatus("model selected; daemon restarting")
}

func orderModelsCommandOptions(models []rpcclient.ModelOption) []rpcclient.ModelOption {
	ordered := append([]rpcclient.ModelOption(nil), models...)
	for index, model := range ordered {
		if !model.Current || index == 0 {
			continue
		}

		current := ordered[index]
		copy(ordered[1:index+1], ordered[:index])
		ordered[0] = current
		break
	}

	return ordered
}
