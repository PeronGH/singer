package gui

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

const (
	singBoxBinary = "sing-box"
	maxLogLines   = 1000
)

type logKind int

const (
	logKindOutput logKind = iota
	logKindStart
	logKindStop
)

type logEntry struct {
	id   uint64
	kind logKind
	text string
}

type runRequest struct {
	groupName  string
	nodeTag    string
	nodeIndex  int
	configJSON []byte
}

type processHandle interface {
	Stop() error
}

type processStarter func(configJSON []byte, onLine func(string), onDone func(error)) (processHandle, error)

type execProcess struct {
	cmd *exec.Cmd
}

func (p execProcess) Stop() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func startSingBoxProcess(configJSON []byte, onLine func(string), onDone func(error)) (processHandle, error) {
	cmd := exec.Command(singBoxBinary, "run", "-c", "stdin")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		_, _ = stdin.Write(configJSON)
		_ = stdin.Close()

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			onLine(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			onLine(fmt.Sprintf("log stream error: %v", err))
		}
		onDone(cmd.Wait())
	}()

	return execProcess{cmd: cmd}, nil
}

type runner struct {
	mu           sync.Mutex
	proc         processHandle
	closed       bool
	startProcess processStarter
	lookPath     func(string) (string, error)

	active      bool
	stopping    bool
	activeGroup string
	activeTag   string
	activeIndex int
	pending     *runRequest

	lines  []logEntry
	nextID uint64
}

func newRunner(starter processStarter) *runner {
	if starter == nil {
		starter = startSingBoxProcess
	}
	return &runner{
		startProcess: starter,
		lookPath:     exec.LookPath,
		activeIndex:  -1,
	}
}

func (r *runner) checkBinary() error {
	_, err := r.lookPath(singBoxBinary)
	return err
}

func (r *runner) connect(req runRequest, dispatch func(func()), onErr func(error)) {
	if r.active {
		reqCopy := req
		r.pending = &reqCopy
		if !r.stopping {
			r.stop()
		}
		return
	}
	r.launch(req, dispatch, onErr)
}

func (r *runner) launch(req runRequest, dispatch func(func()), onErr func(error)) {
	onLine := func(line string) {
		dispatch(func() {
			r.append(logKindOutput, line)
		})
	}
	onDone := func(err error) {
		dispatch(func() {
			r.finish(err, dispatch, onErr)
		})
	}

	proc, err := r.startProcess(req.configJSON, onLine, onDone)
	if err != nil {
		r.append(logKindStop, fmt.Sprintf("sing-box failed to start: %v", err))
		onErr(err)
		return
	}

	r.mu.Lock()
	closed := r.closed
	if !closed {
		r.proc = proc
	}
	r.mu.Unlock()
	if closed {
		_ = proc.Stop()
		return
	}

	r.active = true
	r.stopping = false
	r.activeGroup = req.groupName
	r.activeTag = req.nodeTag
	r.activeIndex = req.nodeIndex
	r.append(logKindStart, fmt.Sprintf("sing-box started (%s / %s)", req.groupName, req.nodeTag))
}

func (r *runner) stop() {
	r.mu.Lock()
	proc := r.proc
	r.mu.Unlock()
	if proc == nil {
		return
	}

	r.stopping = true
	_ = proc.Stop()
}

func (r *runner) finish(err error, dispatch func(func()), onErr func(error)) {
	r.mu.Lock()
	r.proc = nil
	closed := r.closed
	r.mu.Unlock()

	wasStopping := r.stopping
	r.active = false
	r.stopping = false
	r.activeGroup = ""
	r.activeTag = ""
	r.activeIndex = -1

	if closed {
		r.pending = nil
		return
	}

	switch {
	case wasStopping:
		r.append(logKindStop, "sing-box stopped")
		onErr(nil)
	case err != nil:
		r.append(logKindStop, fmt.Sprintf("sing-box exited: %v", err))
		onErr(err)
	default:
		r.append(logKindStop, "sing-box exited")
		onErr(nil)
	}

	if r.pending != nil {
		next := *r.pending
		r.pending = nil
		onErr(nil)
		r.launch(next, dispatch, onErr)
	}
}

func (r *runner) shutdown() {
	r.mu.Lock()
	r.closed = true
	proc := r.proc
	r.proc = nil
	r.mu.Unlock()

	r.pending = nil
	r.stopping = true
	if proc != nil {
		_ = proc.Stop()
	}
}

func (r *runner) append(kind logKind, text string) {
	r.nextID++
	r.lines = append(r.lines, logEntry{
		id:   r.nextID,
		kind: kind,
		text: text,
	})
	if len(r.lines) > maxLogLines {
		r.lines = r.lines[len(r.lines)-maxLogLines:]
	}
}

func (r *runner) clear() {
	r.lines = nil
}

func (r *runner) allText() string {
	var b strings.Builder
	for i, line := range r.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line.text)
	}
	return b.String()
}

func (r *runner) isNodeActive(groupName string, nodeIndex int) bool {
	return r.active && r.activeGroup == groupName && r.activeIndex == nodeIndex
}
