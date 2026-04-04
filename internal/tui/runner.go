package tui

import (
	"bufio"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type runner struct {
	cmd   *exec.Cmd
	sub   chan tea.Msg // log line / done messages arrive here
	lines []string
}

// start launches "sing-box run -c stdin", pipes configJSON to its stdin,
// and begins streaming stdout/stderr to the sub channel.
// Returns a batch: the producer goroutine and the first waitForLog.
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

	r.cmd = cmd
	r.sub = make(chan tea.Msg, 64)
	r.lines = nil

	return tea.Batch(
		streamLogs(cmd, stdin, stdout, configJSON, r.sub),
		waitForLog(r.sub),
	)
}

// streamLogs is the producer: writes config to stdin, then scans
// stdout and sends each line to sub. Sends runnerDoneMsg when done.
func streamLogs(cmd *exec.Cmd, stdin interface {
	Write([]byte) (int, error)
	Close() error
}, stdout interface{ Read([]byte) (int, error) }, configJSON []byte, sub chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		_, _ = stdin.Write(configJSON)
		_ = stdin.Close()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			sub <- logLineMsg(scanner.Text())
		}
		sub <- runnerDoneMsg{cmd.Wait()}
		return nil // producer never returns a message directly
	}
}

// waitForLog blocks until the next message arrives on sub.
// Call this again from Update after each logLineMsg to keep streaming.
func waitForLog(sub <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub
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

// waitNext returns a Cmd to receive the next log message.
func (r *runner) waitNext() tea.Cmd {
	if r.sub == nil {
		return nil
	}
	return waitForLog(r.sub)
}

const maxLogLines = 1000

func (r *runner) appendLine(line string) {
	r.lines = append(r.lines, line)
	if len(r.lines) > maxLogLines {
		r.lines = r.lines[len(r.lines)-maxLogLines:]
	}
}

func (r *runner) logContent() string {
	return strings.Join(r.lines, "\n")
}

func done(err error) tea.Cmd {
	return func() tea.Msg { return runnerDoneMsg{err} }
}
