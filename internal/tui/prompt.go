package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type promptFlow int

const (
	promptFlowNone promptFlow = iota
	promptFlowAddLocalNode
	promptFlowEditLocalNode
	promptFlowRemoteName
	promptFlowRemoteURL
)

type promptActionKind int

const (
	promptActionNone promptActionKind = iota
	promptActionConfirm
	promptActionCancel
)

type promptAction struct {
	Kind  promptActionKind
	Value string
}

type promptModel struct {
	input      textinput.Model
	flow       promptFlow
	remoteName string
	width      int
}

func newPrompt(width int) promptModel {
	return promptModel{width: width}
}

func (p *promptModel) setWidth(width int) {
	p.width = width
	if p.input.Width() > 0 {
		p.input.SetWidth(promptWidth(width))
	}
}

func (p *promptModel) start(flow promptFlow, prompt, value string) tea.Cmd {
	p.flow = flow
	p.input = textinput.New()
	p.input.Prompt = prompt
	p.input.SetValue(value)
	p.input.SetWidth(promptWidth(p.width))
	return p.input.Focus()
}

func (p *promptModel) reset() {
	p.flow = promptFlowNone
	p.input = textinput.Model{}
}

func (p *promptModel) update(msg tea.Msg) (promptAction, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "enter":
			return promptAction{
				Kind:  promptActionConfirm,
				Value: p.input.Value(),
			}, nil
		case "escape":
			return promptAction{Kind: promptActionCancel}, nil
		}
	}

	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return promptAction{}, cmd
}

func (p *promptModel) view() string {
	return p.input.View()
}

func promptWidth(width int) int {
	if width <= 0 {
		return 16
	}
	return max(width-2, 16)
}
