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

	"github.com/PeronGH/singer/internal/androidvpn"
)

const defaultTunMode = true

func defaultLookPath(string) (string, error) {
	return "", nil
}

type androidProcess struct {
	svc    androidvpn.Service
	cancel context.CancelFunc
	once   sync.Once
}

func (p *androidProcess) Stop() error {
	var err error
	p.once.Do(func() {
		p.cancel()
		err = p.svc.Stop()
	})
	return err
}

type clashLogMessage struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

func startSingBoxProcess(ctx context.Context, req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
	svc, err := androidvpn.NewService()
	if err != nil {
		return nil, err
	}

	if err := svc.Start(ctx, string(req.configJSON), req.tunMode); err != nil {
		return nil, err
	}

	logCtx, cancel := context.WithCancel(context.Background())
	go streamClashLogs(logCtx, req.api, onLine, onDone)
	return &androidProcess{svc: svc, cancel: cancel}, nil
}

func streamClashLogs(ctx context.Context, api clashAPIInfo, onLine func(string), onDone func(error)) {
	if api.addr == "" {
		<-ctx.Done()
		onDone(nil)
		return
	}

	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+api.addr+"/logs?level=info", nil)
	if err != nil {
		onDone(err)
		return
	}
	if api.secret != "" {
		req.Header.Set("Authorization", "Bearer "+api.secret)
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			onDone(nil)
			return
		}
		onDone(err)
		return
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
		onDone(nil)
		return
	}
	onDone(scanner.Err())
}
