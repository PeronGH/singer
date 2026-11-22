package gui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
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
	tunMode    bool
	configJSON []byte
	api        clashAPIInfo
}

type processHandle interface {
	Stop() error
}

type processStarter func(req runRequest, onLine func(string), onDone func(error)) (processHandle, error)

type runner struct {
	mu              sync.Mutex
	proc            processHandle
	closed          bool
	startProcess    processStarter
	lookPath        func(string) (string, error)
	metricsClient   *http.Client
	metricsInterval time.Duration
	metricsCancel   context.CancelFunc
	metricsRunID    uint64

	active      bool
	stopping    bool
	activeGroup string
	activeTag   string
	activeIndex int
	pending     *runRequest
	metrics     trafficMetrics

	lines  []logEntry
	nextID uint64
}

func newRunner(starter processStarter) *runner {
	if starter == nil {
		starter = startSingBoxProcess
	}
	return &runner{
		startProcess:    starter,
		lookPath:        defaultLookPath,
		metricsClient:   newLocalHTTPClient(),
		metricsInterval: defaultMetricsInterval,
		activeIndex:     -1,
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

	proc, err := r.startProcess(req, onLine, onDone)
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
	r.metrics = trafficMetrics{}
	r.append(logKindStart, fmt.Sprintf("sing-box started (%s / %s)", req.groupName, req.nodeTag))
	r.startMetrics(req.api, dispatch)
}

func (r *runner) startMetrics(api clashAPIInfo, dispatch func(func())) {
	r.stopMetrics()
	if api.addr == "" {
		return
	}

	r.metricsRunID++
	runID := r.metricsRunID
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.metricsCancel = cancel
	r.mu.Unlock()

	client := r.metricsClient
	interval := r.metricsInterval
	go runMetricsPoller(ctx, client, api, interval, func(m trafficMetrics) {
		dispatch(func() {
			if r.active && r.metricsRunID == runID {
				r.metrics = m
			}
		})
	})
}

func (r *runner) stopMetrics() {
	r.mu.Lock()
	cancel := r.metricsCancel
	r.metricsCancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
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
	r.stopMetrics()

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
	r.metrics = trafficMetrics{}

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
	r.stopMetrics()

	r.mu.Lock()
	r.closed = true
	proc := r.proc
	r.proc = nil
	r.mu.Unlock()

	r.pending = nil
	r.stopping = true
	r.metrics = trafficMetrics{}
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
