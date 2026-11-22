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

// processDetacher is implemented by out-of-process services (such as Android's :vpn)
// that outlive the UI process and only need local streams torn down on UI detach/exit.
type processDetacher interface {
	Detach()
}

// processStreamPauser is implemented by processes whose UI log stream can be
// paused while the app is backgrounded to avoid unnecessary wakeups.
type processStreamPauser interface {
	PauseStreams()
	ResumeStreams()
}

type externalEvent struct {
	running bool
	req     runRequest
}

type processStarter func(ctx context.Context, req runRequest, onLine func(string), onDone func(error)) (processHandle, error)
type externalWatcher func(ctx context.Context, onEvent func(externalEvent))
type externalAttacher func(req runRequest, onLine func(string), onDone func(error)) (processHandle, error)

type runner struct {
	mu              sync.Mutex
	proc            processHandle
	startCancel     context.CancelFunc
	watchCancel     context.CancelFunc
	closed          bool
	startProcess    processStarter
	watchExternal   externalWatcher
	attachExternal  externalAttacher
	lookPath        func(string) (string, error)
	metricsClient   *http.Client
	metricsInterval time.Duration
	metricsCancel   context.CancelFunc
	metricsRunID    uint64
	runSeq          uint64

	starting     bool
	active       bool
	stopping     bool
	backgrounded bool
	activeGroup  string
	activeTag    string
	activeIndex  int
	activeAPI    clashAPIInfo
	pending      *runRequest
	metrics      trafficMetrics

	lines  []logEntry
	nextID uint64
}

func newRunner(starter processStarter) *runner {
	if starter == nil {
		starter = startSingBoxProcess
	}
	return &runner{
		startProcess:    starter,
		watchExternal:   watchExternalProcess,
		attachExternal:  attachExternalProcess,
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

func (r *runner) busy() bool {
	return r.starting || r.active
}

func (r *runner) connect(req runRequest, dispatch func(func()), onErr func(error)) {
	if r.busy() {
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
	r.runSeq++
	runID := r.runSeq

	startCtx, startCancel := context.WithCancel(context.Background())
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		startCancel()
		return
	}
	r.startCancel = startCancel
	r.mu.Unlock()

	r.starting = true
	r.active = false
	r.stopping = false
	r.activeGroup = req.groupName
	r.activeTag = req.nodeTag
	r.activeIndex = req.nodeIndex
	r.metrics = trafficMetrics{}
	r.append(logKindStart, fmt.Sprintf("starting sing-box (%s / %s)…", req.groupName, req.nodeTag))

	starter := r.startProcess
	onLine := func(line string) {
		dispatch(func() {
			r.append(logKindOutput, line)
		})
	}
	onDone := func(err error) {
		dispatch(func() {
			if r.runSeq == runID {
				r.finish(err, dispatch, onErr)
			}
		})
	}

	go func() {
		proc, err := starter(startCtx, req, onLine, onDone)
		dispatch(func() {
			r.onStarted(runID, req, proc, err, dispatch, onErr)
		})
	}()
}

func (r *runner) onStarted(runID uint64, req runRequest, proc processHandle, err error, dispatch func(func()), onErr func(error)) {
	r.mu.Lock()
	if r.runSeq == runID && r.startCancel != nil {
		r.startCancel()
		r.startCancel = nil
	}
	closed := r.closed
	r.mu.Unlock()

	if closed || r.runSeq != runID {
		if proc != nil {
			_ = proc.Stop()
		}
		return
	}

	if err != nil {
		wasStopping := r.stopping
		r.resetState()
		if wasStopping {
			r.append(logKindStop, "sing-box stopped")
			onErr(nil)
		} else {
			r.append(logKindStop, fmt.Sprintf("sing-box failed to start: %v", err))
			onErr(err)
		}
		if r.pending != nil {
			next := *r.pending
			r.pending = nil
			onErr(nil)
			r.launch(next, dispatch, onErr)
		}
		return
	}

	r.mu.Lock()
	r.proc = proc
	r.mu.Unlock()

	if r.stopping {
		r.starting = false
		r.active = true
		_ = proc.Stop()
		return
	}

	r.starting = false
	r.active = true
	r.stopping = false
	r.activeAPI = req.api
	r.append(logKindStart, fmt.Sprintf("sing-box started (%s / %s)", req.groupName, req.nodeTag))
	if r.backgrounded {
		if p, ok := proc.(processStreamPauser); ok {
			p.PauseStreams()
		}
		return
	}
	r.startMetrics(req.api, dispatch)
}

func (r *runner) resetState() {
	r.starting = false
	r.active = false
	r.stopping = false
	r.activeGroup = ""
	r.activeTag = ""
	r.activeIndex = -1
	r.activeAPI = clashAPIInfo{}
	r.metrics = trafficMetrics{}
}

func (r *runner) setBackgrounded(bg bool, dispatch func(func())) {
	if r.backgrounded == bg {
		return
	}
	r.backgrounded = bg

	r.mu.Lock()
	proc := r.proc
	r.mu.Unlock()

	if bg {
		r.stopMetrics()
		if p, ok := proc.(processStreamPauser); ok {
			p.PauseStreams()
		}
		return
	}

	if r.active {
		if p, ok := proc.(processStreamPauser); ok {
			p.ResumeStreams()
		}
		r.startMetrics(r.activeAPI, dispatch)
	}
}

func (r *runner) startMetrics(api clashAPIInfo, dispatch func(func())) {
	r.stopMetrics()
	if api.addr == "" || r.backgrounded {
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
	startCancel := r.startCancel
	r.mu.Unlock()
	if proc == nil && startCancel == nil {
		return
	}

	r.stopping = true
	if startCancel != nil {
		startCancel()
	}
	if proc != nil {
		_ = proc.Stop()
	}
}

func (r *runner) finish(err error, dispatch func(func()), onErr func(error)) {
	r.stopMetrics()

	r.mu.Lock()
	r.proc = nil
	if r.startCancel != nil {
		r.startCancel()
		r.startCancel = nil
	}
	closed := r.closed
	r.mu.Unlock()

	r.runSeq++
	wasStopping := r.stopping
	r.resetState()

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
	startCancel := r.startCancel
	r.startCancel = nil
	watchCancel := r.watchCancel
	r.watchCancel = nil
	r.mu.Unlock()

	r.pending = nil
	r.resetState()
	r.stopping = true
	if watchCancel != nil {
		watchCancel()
	}
	if startCancel != nil {
		startCancel()
	}
	if proc != nil {
		if d, ok := proc.(processDetacher); ok {
			d.Detach()
		} else {
			_ = proc.Stop()
		}
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

func (r *runner) isNodeStarting(groupName string, nodeIndex int) bool {
	return r.starting && r.activeGroup == groupName && r.activeIndex == nodeIndex
}

func (r *runner) isNodeActive(groupName string, nodeIndex int) bool {
	return r.active && r.activeGroup == groupName && r.activeIndex == nodeIndex
}

func (r *runner) startWatcher(dispatch func(func()), onAttach func(runRequest), onErr func(error)) {
	if r.watchExternal == nil || r.attachExternal == nil {
		return
	}
	r.mu.Lock()
	if r.closed || r.watchCancel != nil {
		r.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.watchCancel = cancel
	r.mu.Unlock()

	watch := r.watchExternal
	attach := r.attachExternal
	go watch(ctx, func(ev externalEvent) {
		dispatch(func() {
			r.mu.Lock()
			isClosed := r.closed
			r.mu.Unlock()
			if isClosed {
				return
			}

			if !ev.running {
				if r.active && !r.starting {
					r.mu.Lock()
					proc := r.proc
					r.mu.Unlock()
					if d, ok := proc.(processDetacher); ok {
						d.Detach()
					}
					r.finish(nil, dispatch, onErr)
				}
				return
			}

			if r.busy() {
				return
			}

			r.runSeq++
			runID := r.runSeq
			onLine := func(line string) {
				dispatch(func() {
					r.append(logKindOutput, line)
				})
			}
			onDone := func(err error) {
				dispatch(func() {
					if r.runSeq == runID {
						r.finish(err, dispatch, onErr)
					}
				})
			}

			proc, err := attach(ev.req, onLine, onDone)
			if err != nil || proc == nil {
				return
			}

			r.mu.Lock()
			r.proc = proc
			r.mu.Unlock()

			r.starting = false
			r.active = true
			r.stopping = false
			r.activeGroup = ev.req.groupName
			r.activeTag = ev.req.nodeTag
			r.activeIndex = ev.req.nodeIndex
			r.activeAPI = ev.req.api
			r.metrics = trafficMetrics{}
			r.append(logKindStart, fmt.Sprintf("attached to running sing-box (%s / %s)", ev.req.groupName, ev.req.nodeTag))
			if onAttach != nil {
				onAttach(ev.req)
			}
			if r.backgrounded {
				if p, ok := proc.(processStreamPauser); ok {
					p.PauseStreams()
				}
				return
			}
			r.startMetrics(ev.req.api, dispatch)
		})
	})
}
