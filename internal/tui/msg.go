package tui

import "github.com/PeronGH/singer/internal/store"

type fetchDoneMsg struct {
	group store.Group
	err   error
}

type logLineMsg string

type runnerDoneMsg struct {
	err error
}
