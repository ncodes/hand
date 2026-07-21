package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/wandxy/morph/internal/permissions"
)

type permissionApprovalOption struct {
	label    string
	detail   string
	key      rune
	approved bool
	scope    permissions.GrantScope
}

func (m *model) showPermissionApprovalCommandView(message permissionApprovalMsg) {
	options := getPermissionApprovalOptions(message)
	m.showCommandView(commandViewPayload{
		TitleIcon:       permissionStatusIcon,
		TitleLeft:       "Permission approval",
		TitleSubtext:    message.Summary,
		TitleRight:      getPermissionApprovalCommandHint(options),
		TitleRightColor: defaultTUITheme.MutedText,
		Kind:            commandViewKindApproval,
		Height:          len(options) + 3,
	})
	m.commandViewItemSelected = 0
	m.commandViewOffset = 0
}

func getPermissionApprovalCommandHint(options []permissionApprovalOption) string {
	keys := make([]string, len(options))
	for index, option := range options {
		keys[index] = string(option.key)
	}

	return "↑/↓ select · enter · " + strings.Join(keys, "/")
}

func (m model) isPermissionApprovalCommandView() bool {
	return m.commandView.Visible && m.commandView.Kind == commandViewKindApproval
}

func (m model) renderPermissionApprovalCommandViewContent(content commandViewContent) string {
	message, ok := m.pendingApprovalMessages[m.pendingApprovalID]
	if !ok {
		return "No permission approval pending."
	}

	options := getPermissionApprovalOptions(message)
	offset := min(max(content.Offset, 0), max(len(options)-1, 0))
	height := max(content.Height, 1)
	end := min(offset+height, len(options))
	rows := make([]string, 0, height)
	for index := offset; index < end; index++ {
		option := options[index]
		rows = append(rows, renderCommandListEntryRow(
			option.label,
			option.detail,
			content.Width,
			max(content.Width-2, 1),
			index == m.commandViewItemSelected,
		))
	}
	for len(rows) < height {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}

func (m *model) updatePermissionApprovalCommandView(msg tea.Msg) (tea.Model, tea.Cmd) {
	message, ok := m.pendingApprovalMessages[m.pendingApprovalID]
	if !ok {
		return *m, nil
	}

	options := getPermissionApprovalOptions(message)
	selection := m.commandViewItemSelected
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if option, found := getPermissionApprovalOptionByKey(options, msg.Key().Code); found {
			return *m, m.resolvePermissionApproval(option.approved, option.scope)
		}
		if msg.Key().Code == 'a' {
			return *m, m.setStatus("always approval is unavailable for these effects")
		}
		switch msg.Key().Code {
		case tea.KeyUp:
			selection--
		case tea.KeyDown:
			selection++
		case tea.KeyHome:
			selection = 0
		case tea.KeyEnd:
			selection = len(options) - 1
		case tea.KeyEnter:
			return m.selectPermissionApprovalOption(options)
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
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || !m.isMouseInCommandViewContent(msg.Mouse()) {
			return *m, nil
		}
		selection = m.commandViewOffset + msg.Y - m.getCommandViewContentTop()
		if selection < 0 || selection >= len(options) {
			return *m, nil
		}
		m.commandViewItemSelected = selection
		m.clearCommandViewSelection()
		return m.selectPermissionApprovalOption(options)
	default:
		return *m, nil
	}

	m.commandViewItemSelected = min(max(selection, 0), len(options)-1)
	m.commandViewOffset = getChatsCommandViewOffsetForSelection(
		m.commandViewItemSelected,
		m.commandViewOffset,
		m.getCommandViewContentHeight(),
		len(options),
	)
	m.clearCommandViewSelection()

	return *m, nil
}

func (m *model) selectPermissionApprovalOption(options []permissionApprovalOption) (tea.Model, tea.Cmd) {
	index := min(max(m.commandViewItemSelected, 0), len(options)-1)
	option := options[index]

	return *m, m.resolvePermissionApproval(option.approved, option.scope)
}

func getPermissionApprovalOptions(message permissionApprovalMsg) []permissionApprovalOption {
	options := []permissionApprovalOption{
		{
			label: "Allow once", detail: "y · approve this request only", key: 'y',
			approved: true, scope: permissions.GrantOnce,
		},
		{
			label: "Allow for session", detail: "s · remember this approval for this session", key: 's',
			approved: true, scope: permissions.GrantSession,
		},
	}
	if isAlwaysApprovalAvailable(message.Effects) {
		options = append(options, permissionApprovalOption{
			label: "Always allow", detail: "a · remember this approval until revoked",
			key: 'a', approved: true, scope: permissions.GrantAlways,
		})
	}
	return append(options, permissionApprovalOption{
		label: "Deny", detail: "n · deny this request only", key: 'n',
	})
}

func getPermissionApprovalOptionByKey(options []permissionApprovalOption, key rune) (permissionApprovalOption, bool) {
	for _, option := range options {
		if option.key == key {
			return option, true
		}
	}

	return permissionApprovalOption{}, false
}
