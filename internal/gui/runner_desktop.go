//go:build !android

package gui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

const defaultTunMode = false

var defaultLookPath = exec.LookPath

type execProcess struct {
	cmd  *exec.Cmd
	done <-chan struct{}
}

func (p execProcess) Stop() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		return p.cmd.Process.Kill()
	}
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		return p.cmd.Process.Kill()
	}
	if p.done == nil {
		return nil
	}
	go func() {
		select {
		case <-p.done:
		case <-time.After(2 * time.Second):
			_ = p.cmd.Process.Kill()
		}
	}()
	return nil
}

func startSingBoxProcess(ctx context.Context, req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command(singBoxBinary, "run", "--disable-color", "-c", "stdin")

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

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = stdin.Write(req.configJSON)
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

	return execProcess{cmd: cmd, done: done}, nil
}
