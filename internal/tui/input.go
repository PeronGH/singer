package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type inputFlow int

const (
	flowAddNode inputFlow = iota
	flowEditNode
	flowSubName
	flowSubURL
)

type inputModel struct {
	ti      textinput.Model
	flow    inputFlow
	active  bool
	subName string // held between name → URL steps
}

func (m *inputModel) start(flow inputFlow, prompt, value string, width int) tea.Cmd {
	m.flow = flow
	m.active = true
	m.ti = textinput.New()
	m.ti.Prompt = prompt
	m.ti.SetValue(value)
	m.ti.SetWidth(width)
	return m.ti.Focus()
}

func (m *inputModel) Update(msg tea.Msg) tea.Cmd {
	// Handle bracketed paste.
	if paste, ok := msg.(tea.PasteMsg); ok {
		m.ti.SetValue(m.ti.Value() + paste.Content)
		m.ti.CursorEnd()
		return nil
	}

	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		return cmd
	}

	switch km.String() {
	case "enter":
		value := m.ti.Value()
		m.active = false
		return func() tea.Msg { return inputResultMsg{value: value} }
	case "escape":
		m.active = false
		return func() tea.Msg { return inputCancelMsg{} }
	}

	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return cmd
}

func (m *inputModel) View() string {
	return m.ti.View()
}
