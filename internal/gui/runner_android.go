//go:build android

package gui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/PeronGH/singer/internal/androidvpn"
)

const defaultTunMode = true

func defaultLookPath(string) (string, error) {
	return "", nil
}

var getAndroidService = sync.OnceValues(androidvpn.NewService)

type androidProcess struct {
	svc    androidvpn.Service
	api    clashAPIInfo
	onLine func(string)
	onDone func(error)

	mu         sync.Mutex
	cancel     context.CancelFunc
	streamSkip *atomic.Bool
	paused     bool
	detached   bool
	stopped    bool
}

func newAndroidProcess(svc androidvpn.Service, api clashAPIInfo, onLine func(string), onDone func(error)) *androidProcess {
	p := &androidProcess{
		svc:    svc,
		api:    api,
		onLine: onLine,
		onDone: onDone,
	}
	p.mu.Lock()
	p.startLogStreamLocked()
	p.mu.Unlock()
	return p
}

func (p *androidProcess) startLogStreamLocked() {
	if p.detached || p.stopped || p.paused || p.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	skipDone := &atomic.Bool{}
	p.cancel = cancel
	p.streamSkip = skipDone

	go func() {
		err := streamClashLogs(ctx, p.api, p.onLine)
		if !skipDone.Load() {
			p.onDone(err)
		}
	}()
}

func (p *androidProcess) PauseStreams() {
	p.mu.Lock()
	if p.detached || p.stopped || p.paused {
		p.mu.Unlock()
		return
	}
	p.paused = true
	cancel := p.cancel
	skip := p.streamSkip
	p.cancel = nil
	p.streamSkip = nil
	p.mu.Unlock()

	if skip != nil {
		skip.Store(true)
	}
	if cancel != nil {
		cancel()
	}
}

func (p *androidProcess) ResumeStreams() {
	p.mu.Lock()
	if p.detached || p.stopped || !p.paused {
		p.mu.Unlock()
		return
	}
	p.paused = false
	p.startLogStreamLocked()
	p.mu.Unlock()
}

func (p *androidProcess) Detach() {
	p.mu.Lock()
	if p.detached {
		p.mu.Unlock()
		return
	}
	p.detached = true
	cancel := p.cancel
	skip := p.streamSkip
	p.cancel = nil
	p.streamSkip = nil
	p.mu.Unlock()

	if skip != nil {
		skip.Store(true)
	}
	if cancel != nil {
		cancel()
	}
}

func (p *androidProcess) Stop() error {
	p.mu.Lock()
	alreadyStopped := p.stopped
	p.stopped = true
	p.paused = false
	cancel := p.cancel
	p.cancel = nil
	p.streamSkip = nil
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if alreadyStopped {
		return nil
	}
	return p.svc.Stop()
}

type clashLogMessage struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

func startSingBoxProcess(ctx context.Context, req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
	svc, err := getAndroidService()
	if err != nil {
		return nil, err
	}

	startReq := androidvpn.StartRequest{
		ConfigJSON: string(req.configJSON),
		TunMode:    req.tunMode,
		GroupName:  req.groupName,
		NodeTag:    req.nodeTag,
		NodeIndex:  req.nodeIndex,
		APIAddr:    req.api.addr,
		APISecret:  req.api.secret,
	}
	if err := svc.Start(ctx, startReq); err != nil {
		return nil, err
	}

	return newAndroidProcess(svc, req.api, onLine, onDone), nil
}

func watchExternalProcess(ctx context.Context, onEvent func(externalEvent)) {
	svc, err := getAndroidService()
	if err != nil {
		return
	}
	for st, err := range svc.Watch(ctx) {
		if err != nil {
			return
		}
		onEvent(externalEvent{
			running: st.Running,
			req: runRequest{
				groupName: st.GroupName,
				nodeTag:   st.NodeTag,
				nodeIndex: st.NodeIndex,
				tunMode:   st.TunMode,
				api: clashAPIInfo{
					addr:   st.APIAddr,
					secret: st.APISecret,
				},
			},
		})
	}
}

func attachExternalProcess(req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
	svc, err := getAndroidService()
	if err != nil {
		return nil, err
	}
	return newAndroidProcess(svc, req.api, onLine, onDone), nil
}

func streamClashLogs(ctx context.Context, api clashAPIInfo, onLine func(string)) error {
	if api.addr == "" {
		<-ctx.Done()
		return nil
	}

	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+api.addr+"/logs?level=info", nil)
	if err != nil {
		return err
	}
	if api.secret != "" {
		req.Header.Set("Authorization", "Bearer "+api.secret)
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg clashLogMessage
		if err := json.Unmarshal([]byte(line), &msg); err == nil && msg.Payload != "" {
			if msg.Type != "" {
				onLine(fmt.Sprintf("[%s] %s", strings.ToUpper(msg.Type), msg.Payload))
			} else {
				onLine(msg.Payload)
			}
			continue
		}
		onLine(line)
	}

	if ctx.Err() != nil {
		return nil
	}
	return scanner.Err()
}
