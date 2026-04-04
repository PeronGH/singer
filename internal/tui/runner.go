package tui

import (
	"bufio"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

const maxLogLines = 1000

type runnerModel struct {
	cmd      *exec.Cmd
	sub      chan tea.Msg
	lines    []string
	logView  viewport.Model
	active   bool
	stopping bool
}

func newRunner() runnerModel {
	return runnerModel{
		logView: viewport.New(),
	}
}

func (r *runnerModel) setSize(width, height int) {
	r.logView.SetWidth(width)
	r.logView.SetHeight(height)
}

func (r *runnerModel) hasOutput() bool {
	return r.active || len(r.lines) > 0
}

func (r *runnerModel) start(configJSON []byte) tea.Cmd {
	cmd := exec.Command("sing-box", "run", "-c", "stdin")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return runnerDone(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return runnerDone(err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return runnerDone(err)
	}

	r.cmd = cmd
	r.sub = make(chan tea.Msg, 64)
	r.lines = nil
	r.logView.SetContent("")
	r.active = true
	r.stopping = false

	return tea.Batch(
		streamLogs(cmd, stdin, stdout, configJSON, r.sub),
		waitForLog(r.sub),
	)
}

func (r *runnerModel) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	r.logView, cmd = r.logView.Update(msg)
	return cmd
}

func (r *runnerModel) stop() {
	if r.cmd == nil || r.cmd.Process == nil {
		return
	}

	r.stopping = true
	_ = r.cmd.Process.Kill()
}

func (r *runnerModel) appendLine(line string) tea.Cmd {
	r.lines = append(r.lines, line)
	if len(r.lines) > maxLogLines {
		r.lines = r.lines[len(r.lines)-maxLogLines:]
	}

	r.logView.SetContent(strings.Join(r.lines, "\n"))
	if r.active {
		r.logView.GotoBottom()
	}

	return waitForLog(r.sub)
}

func (r *runnerModel) finish(msg runnerDoneMsg) error {
	r.cmd = nil
	r.sub = nil
	r.active = false

	if r.stopping {
		r.stopping = false
		return nil
	}

	return msg.err
}

func streamLogs(cmd *exec.Cmd, stdin interface {
	Write([]byte) (int, error)
	Close() error
}, stdout interface{ Read([]byte) (int, error) }, configJSON []byte, sub chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		defer close(sub)

		_, _ = stdin.Write(configJSON)
		_ = stdin.Close()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			sub <- logLineMsg(scanner.Text())
		}

		sub <- runnerDoneMsg{err: cmd.Wait()}
		return nil
	}
}

func waitForLog(sub <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub
		if !ok {
			return nil
		}
		return msg
	}
}

func runnerDone(err error) tea.Cmd {
	return func() tea.Msg { return runnerDoneMsg{err: err} }
}
