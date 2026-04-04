package tui

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

var stopKey = key.NewBinding(key.WithKeys("escape"), key.WithHelp("esc", "stop"))

func matchCtrlLetter(msg tea.KeyPressMsg, letter rune) bool {
	key := msg.Key()
	if key.Mod&tea.ModCtrl == 0 {
		return false
	}

	return keyMatchesRune(key, letter)
}

func matchLetter(msg tea.KeyPressMsg, letter rune) bool {
	key := msg.Key()
	mods := key.Mod &^ (tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock)
	if mods != 0 && mods != tea.ModShift {
		return false
	}

	return keyMatchesRune(key, letter)
}

func matchPrintable(msg tea.KeyPressMsg, r rune) bool {
	key := msg.Key()
	mods := key.Mod &^ (tea.ModShift | tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock)
	if mods != 0 {
		return false
	}

	want := string(r)
	if text := strings.TrimSpace(key.Text); text == want {
		return true
	}
	return key.Code == r
}

func keyMatchesRune(key tea.Key, letter rune) bool {
	want := unicode.ToLower(letter)

	if text := strings.TrimSpace(key.Text); len(text) == 1 && unicode.ToLower([]rune(text)[0]) == want {
		return true
	}
	if key.Code != 0 && unicode.ToLower(key.Code) == want {
		return true
	}
	if key.BaseCode != 0 && unicode.ToLower(key.BaseCode) == want {
		return true
	}
	return false
}
