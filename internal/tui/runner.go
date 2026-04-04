package tui

import (
	"bufio"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type runner struct {
	cmd   *exec.Cmd
	ch    <-chan tea.Msg
	lines []string
}

// start launches "sing-box run -c stdin" with configJSON piped to stdin.
// It returns a tea.Cmd that reads the first log message.
func (r *runner) start(configJSON []byte) tea.Cmd {
	cmd := exec.Command("sing-box", "run", "-c", "stdin")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return done(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return done(err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return done(err)
	}

	go func() {
		_, _ = stdin.Write(configJSON)
		_ = stdin.Close()
	}()

	ch := make(chan tea.Msg, 64)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			ch <- logLineMsg(scanner.Text())
		}
		ch <- runnerDoneMsg{cmd.Wait()}
	}()

	r.cmd = cmd
	r.ch = ch
	r.lines = nil

	return r.waitNext()
}

// waitNext returns a Cmd that reads the next message from the channel.
func (r *runner) waitNext() tea.Cmd {
	ch := r.ch
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return runnerDoneMsg{}
		}
		return msg
	}
}

// stop kills the running sing-box process.
func (r *runner) stop() {
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
		r.cmd = nil
	}
}

func (r *runner) appendLine(line string) {
	r.lines = append(r.lines, line)
}

func (r *runner) logContent() string {
	return strings.Join(r.lines, "\n")
}

func done(err error) tea.Cmd {
	return func() tea.Msg { return runnerDoneMsg{err} }
}
