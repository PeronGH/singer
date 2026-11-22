package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/PeronGH/singer/internal/store"
	"github.com/egoist/mygo/ui"
)

type fakeProcess struct {
	mu      sync.Mutex
	stopped bool
	onLine  func(string)
	onDone  func(error)
}

func newFakeProcess(onLine func(string), onDone func(error)) *fakeProcess {
	return &fakeProcess{
		onLine: onLine,
		onDone: onDone,
	}
}

func (p *fakeProcess) Stop() error {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
	return nil
}

func (p *fakeProcess) isStopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

func (p *fakeProcess) exit(err error) {
	p.onDone(err)
}

func newTestApp(t *testing.T, nodes []string, opts ...Option) (*App, *store.Store, chan func()) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	s, err := store.New()
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if len(nodes) > 0 {
		if err := s.SaveLocal(nodes); err != nil {
			t.Fatalf("SaveLocal: %v", err)
		}
	}

	events := make(chan func(), 32)
	allOpts := append([]Option{
		WithDispatcher(func(fn func()) { events <- fn }),
		withLookPath(func(string) (string, error) { return "/usr/bin/sing-box", nil }),
		withAPIAllocator(func() (clashAPIInfo, error) { return clashAPIInfo{}, nil }),
	}, opts...)
	app, err := New(s, allOpts...)
	if err != nil {
		t.Fatalf("gui.New: %v", err)
	}
	return app, s, events
}

func drainEvents(t *testing.T, events chan func(), tt *ui.Tester, wait bool) {
	t.Helper()
	if wait {
		select {
		case fn := <-events:
			fn()
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for async UI event")
		}
	}
	for {
		select {
		case fn := <-events:
			fn()
		default:
			tt.Frame()
			return
		}
	}
}

func TestConnectUsesGlobalIndexWhenFiltered(t *testing.T) {
	var (
		mu             sync.Mutex
		startedConfigs [][]byte
	)
	starter := func(_ context.Context, req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
		mu.Lock()
		startedConfigs = append(startedConfigs, append([]byte(nil), req.configJSON...))
		mu.Unlock()
		return newFakeProcess(onLine, onDone), nil
	}

	app, _, events := newTestApp(t, []string{
		"socks://10.0.0.1:1080#alpha",
		"socks://10.0.0.2:1080#bravo",
		"socks://10.0.0.3:1080#charlie",
	}, withProcessStarter(starter))

	tt := ui.NewTester(app.View, 960, 640)
	if err := tt.Click("Filter nodes"); err != nil {
		t.Fatalf("click filter: %v", err)
	}
	tt.Type("bravo")

	if tt.HasText("alpha") || !tt.HasText("bravo") {
		t.Fatalf("expected only bravo visible after filtering, texts=%q", tt.Texts())
	}

	if err := tt.Click("Connect bravo"); err != nil {
		t.Fatalf("click connect bravo: %v", err)
	}
	drainEvents(t, events, tt, true)

	mu.Lock()
	defer mu.Unlock()
	if len(startedConfigs) != 1 {
		t.Fatalf("started processes = %d, want 1", len(startedConfigs))
	}
	var cfg struct {
		Route struct {
			Final string `json:"final"`
		} `json:"route"`
	}
	if err := json.Unmarshal(startedConfigs[0], &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.Route.Final != "bravo" {
		t.Fatalf("route.final = %q, want %q", cfg.Route.Final, "bravo")
	}
	if app.runner.activeIndex != 1 {
		t.Fatalf("activeIndex = %d, want 1", app.runner.activeIndex)
	}
	if app.selectedNav != navLogs {
		t.Fatalf("selectedNav = %q, want %q", app.selectedNav, navLogs)
	}
}

func TestLocalNodeAddEditDeleteAndUndo(t *testing.T) {
	app, s, _ := newTestApp(t, nil)
	tt := ui.NewTester(app.View, 960, 640)

	if err := tt.Click("Add Node"); err != nil {
		t.Fatalf("click Add Node: %v", err)
	}
	tt.Type("socks://10.0.0.1:1080#alpha")
	tt.Key(0, ui.KeyEnter)

	if !tt.HasText("alpha") {
		t.Fatalf("expected alpha in UI, texts=%q", tt.Texts())
	}

	groups, err := s.LoadGroups()
	if err != nil {
		t.Fatalf("LoadGroups: %v", err)
	}
	if len(groups[0].Nodes) != 1 || groups[0].Nodes[0] != "socks://10.0.0.1:1080#alpha" {
		t.Fatalf("persisted local nodes = %#v", groups[0].Nodes)
	}

	if err := tt.Click("Edit alpha"); err != nil {
		t.Fatalf("click Edit alpha: %v", err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("socks://10.0.0.2:1080#beta")
	if err := tt.Click("Save"); err != nil {
		t.Fatalf("click Save: %v", err)
	}

	if !tt.HasText("beta") || tt.HasText("alpha") {
		t.Fatalf("expected beta after edit, texts=%q", tt.Texts())
	}

	if err := tt.Click("Delete beta"); err != nil {
		t.Fatalf("click Delete beta: %v", err)
	}
	if tt.HasText("beta") {
		t.Fatalf("expected beta removed after delete, texts=%q", tt.Texts())
	}

	if err := tt.Click("Undo"); err != nil {
		t.Fatalf("click Undo: %v", err)
	}
	if !tt.HasText("beta") {
		t.Fatalf("expected beta restored after Undo, texts=%q", tt.Texts())
	}
}

func TestRemoteAddRefreshAndDelete(t *testing.T) {
	mu := sync.Mutex{}
	payload := "socks://10.0.0.1:1080#edge-1\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	app, s, events := newTestApp(t, nil, WithHTTPClient(srv.Client()))
	tt := ui.NewTester(app.View, 960, 640)

	if err := tt.Click("Add remote"); err != nil {
		t.Fatalf("click Add remote: %v", err)
	}
	tt.Type("work")
	if err := tt.Click("URL"); err != nil {
		t.Fatalf("click URL field: %v", err)
	}
	tt.Type(srv.URL)
	if err := tt.Click("Add Remote"); err != nil {
		t.Fatalf("click Add Remote: %v", err)
	}

	drainEvents(t, events, tt, true)

	if !tt.HasText("work") || !tt.HasText("edge-1") {
		t.Fatalf("expected work group and edge-1 node, texts=%q", tt.Texts())
	}

	mu.Lock()
	payload = "socks://10.0.0.1:1080#edge-1\nsocks://10.0.0.2:1080#edge-2\n"
	mu.Unlock()

	if err := tt.Click("Refresh"); err != nil {
		t.Fatalf("click Refresh: %v", err)
	}
	drainEvents(t, events, tt, true)

	if !tt.HasText("edge-2") {
		t.Fatalf("expected edge-2 after refresh, texts=%q", tt.Texts())
	}

	if err := tt.Click("Delete remote work"); err != nil {
		t.Fatalf("click Delete remote work: %v", err)
	}
	if err := tt.Click("Delete"); err != nil {
		t.Fatalf("confirm delete dialog: %v", err)
	}

	groups, err := s.LoadGroups()
	if err != nil {
		t.Fatalf("LoadGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "local" {
		t.Fatalf("groups after remote delete = %#v", groups)
	}
}

func TestQueuedRestartAndIntentionalStop(t *testing.T) {
	var (
		mu    sync.Mutex
		procs []*fakeProcess
	)
	starter := func(_ context.Context, req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
		p := newFakeProcess(onLine, onDone)
		mu.Lock()
		procs = append(procs, p)
		mu.Unlock()
		return p, nil
	}

	app, _, events := newTestApp(t, []string{
		"socks://10.0.0.1:1080#alpha",
		"socks://10.0.0.2:1080#bravo",
	}, withProcessStarter(starter))
	tt := ui.NewTester(app.View, 960, 640)

	app.connectNode("local", 0)
	drainEvents(t, events, tt, true)

	mu.Lock()
	procCount := len(procs)
	firstProc := procs[0]
	mu.Unlock()
	if procCount != 1 || !app.runner.active || app.runner.activeTag != "alpha" {
		t.Fatalf("first launch failed: procs=%d active=%v tag=%q", procCount, app.runner.active, app.runner.activeTag)
	}

	app.connectNode("local", 1)
	mu.Lock()
	procCount = len(procs)
	mu.Unlock()
	if procCount != 1 {
		t.Fatalf("expected second start to wait for first exit, got %d procs", procCount)
	}
	if !firstProc.isStopped() {
		t.Fatal("expected first process to be stopped for queued restart")
	}
	if app.runner.pending == nil || app.runner.pending.nodeTag != "bravo" {
		t.Fatalf("expected pending bravo restart, got %#v", app.runner.pending)
	}

	firstProc.exit(errors.New("signal: killed"))
	// Drain finish + async second launch completion.
	drainEvents(t, events, tt, true)
	if !app.runner.active {
		drainEvents(t, events, tt, true)
	}

	if app.errMsg != "" {
		t.Fatalf("intentional stop should not set errMsg, got %q", app.errMsg)
	}
	mu.Lock()
	procCount = len(procs)
	secondProc := procs[len(procs)-1]
	mu.Unlock()
	if procCount != 2 || !app.runner.active || app.runner.activeTag != "bravo" {
		t.Fatalf("queued restart did not launch bravo: procs=%d active=%v tag=%q", procCount, app.runner.active, app.runner.activeTag)
	}

	app.Shutdown()
	if !secondProc.isStopped() {
		t.Fatal("Shutdown should stop active process")
	}
	if app.runner.pending != nil {
		t.Fatal("Shutdown should clear pending restart")
	}
}

func TestAccessibilityAllButtonsNamed(t *testing.T) {
	app, _, _ := newTestApp(t, []string{
		"socks://10.0.0.1:1080#alpha",
	})
	tt := ui.NewTester(app.View, 960, 640)

	checkButtons := func(viewName string) {
		t.Helper()
		for _, a := range tt.AccessAll() {
			if a.Role == ui.RoleButton && a.Name == "" {
				t.Errorf("%s: button at %v has empty accessible name", viewName, a.Bounds)
			}
		}
	}

	checkButtons("local group")
	if err := tt.Click("Logs"); err != nil {
		t.Fatalf("click Logs: %v", err)
	}
	checkButtons("logs")
}

func TestMissingSingBoxShowsBlockingScreen(t *testing.T) {
	available := false
	app, _, _ := newTestApp(t, []string{
		"socks://10.0.0.1:1080#alpha",
	}, withLookPath(func(file string) (string, error) {
		if !available {
			return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
		}
		return "/usr/bin/sing-box", nil
	}))
	tt := ui.NewTester(app.View, 960, 640)

	if !tt.HasText("sing-box not found") {
		t.Fatalf("expected blocking screen, texts=%q", tt.Texts())
	}
	if tt.HasText("Add Node") || tt.HasText("alpha") {
		t.Fatalf("main UI should be blocked while sing-box is missing, texts=%q", tt.Texts())
	}

	tt.Key(ui.Cmd, ui.KeyN)
	if app.remoteModalOpen {
		t.Fatal("global shortcuts should be blocked while sing-box is missing")
	}

	for _, a := range tt.AccessAll() {
		if a.Role == ui.RoleButton && a.Name == "" {
			t.Errorf("blocking screen: button at %v has empty accessible name", a.Bounds)
		}
	}

	available = true
	if err := tt.Click("Check Again"); err != nil {
		t.Fatalf("click Check Again: %v", err)
	}

	if tt.HasText("sing-box not found") {
		t.Fatalf("blocking screen should clear after sing-box is detected, texts=%q", tt.Texts())
	}
	if !tt.HasText("Add Node") || !tt.HasText("alpha") {
		t.Fatalf("expected main UI after Check Again, texts=%q", tt.Texts())
	}
}

func TestTunModeToggleGeneratesTunConfig(t *testing.T) {
	var (
		mu             sync.Mutex
		startedConfigs [][]byte
	)
	starter := func(_ context.Context, req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
		mu.Lock()
		startedConfigs = append(startedConfigs, append([]byte(nil), req.configJSON...))
		mu.Unlock()
		return newFakeProcess(onLine, onDone), nil
	}

	app, _, events := newTestApp(t, []string{
		"socks://10.0.0.1:1080#alpha",
	}, withProcessStarter(starter))
	tt := ui.NewTester(app.View, 960, 640)

	if err := tt.Click("TUN Mode"); err != nil {
		t.Fatalf("click TUN Mode: %v", err)
	}
	if !app.tunMode {
		t.Fatal("expected tunMode to be enabled after toggle")
	}
	if err := tt.Click("Connect alpha"); err != nil {
		t.Fatalf("click Connect alpha: %v", err)
	}
	drainEvents(t, events, tt, true)

	mu.Lock()
	defer mu.Unlock()
	if len(startedConfigs) != 1 {
		t.Fatalf("started processes = %d, want 1", len(startedConfigs))
	}

	var cfg struct {
		Inbounds []struct {
			Type    string   `json:"type"`
			Address []string `json:"address"`
		} `json:"inbounds"`
		DNS struct {
			Servers []struct {
				Type string `json:"type"`
			} `json:"servers"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(startedConfigs[0], &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if len(cfg.Inbounds) != 1 || cfg.Inbounds[0].Type != "tun" {
		t.Fatalf("inbounds = %+v, want tun", cfg.Inbounds)
	}
	for _, s := range cfg.DNS.Servers {
		if s.Type == "fakeip" {
			t.Fatal("fakeip must not be present in TUN config")
		}
	}
}

func TestClashAPIMetricsPollingAndUI(t *testing.T) {
	const wantSecret = "test-secret-token"
	var (
		mu   sync.Mutex
		poll int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+wantSecret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mu.Lock()
		poll++
		n := poll
		mu.Unlock()

		resp := map[string]any{
			"downloadTotal": int64(n) * 1024 * 1024,
			"uploadTotal":   int64(n) * 256 * 1024,
			"memory":        uint64(16 * 1024 * 1024),
			"connections":   []map[string]any{{"id": "1"}, {"id": "2"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	var (
		startMu       sync.Mutex
		startedConfig []byte
		proc          *fakeProcess
	)
	starter := func(_ context.Context, req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
		p := newFakeProcess(onLine, onDone)
		startMu.Lock()
		startedConfig = append([]byte(nil), req.configJSON...)
		proc = p
		startMu.Unlock()
		return p, nil
	}

	app, _, events := newTestApp(t, []string{
		"socks://10.0.0.1:1080#alpha",
	},
		withProcessStarter(starter),
		withAPIAllocator(func() (clashAPIInfo, error) {
			return clashAPIInfo{addr: srv.Listener.Addr().String(), secret: wantSecret}, nil
		}),
		withMetricsInterval(20*time.Millisecond),
	)
	defer app.Shutdown()

	tt := ui.NewTester(app.View, 960, 640)
	if err := tt.Click("Connect alpha"); err != nil {
		t.Fatalf("click Connect alpha: %v", err)
	}
	drainEvents(t, events, tt, true)

	var cfg struct {
		Experimental struct {
			ClashAPI struct {
				ExternalController string `json:"external_controller"`
				Secret             string `json:"secret"`
			} `json:"clash_api"`
		} `json:"experimental"`
	}
	startMu.Lock()
	cfgBytes := append([]byte(nil), startedConfig...)
	activeProc := proc
	startMu.Unlock()
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.Experimental.ClashAPI.ExternalController != srv.Listener.Addr().String() {
		t.Fatalf("external_controller = %q, want %q", cfg.Experimental.ClashAPI.ExternalController, srv.Listener.Addr().String())
	}
	if cfg.Experimental.ClashAPI.Secret != wantSecret {
		t.Fatalf("secret = %q, want %q", cfg.Experimental.ClashAPI.Secret, wantSecret)
	}

	// Wait for at least two polls so rate and totals are populated.
	for !app.runner.metrics.ready || app.runner.metrics.downRate <= 0 {
		drainEvents(t, events, tt, true)
	}

	if !app.runner.metrics.ready {
		t.Fatal("expected metrics to be ready")
	}
	if app.runner.metrics.connections != 2 {
		t.Fatalf("connections = %d, want 2", app.runner.metrics.connections)
	}
	if app.runner.metrics.downTotal < 2*1024*1024 || app.runner.metrics.upTotal < 512*1024 {
		t.Fatalf("unexpected totals: %+v", app.runner.metrics)
	}
	if app.runner.metrics.downRate <= 0 || app.runner.metrics.upRate <= 0 {
		t.Fatalf("expected positive transfer rates, got %+v", app.runner.metrics)
	}
	if !tt.HasText("16.0 MB") || !tt.HasText("Connections") {
		t.Fatalf("expected metrics rendered in Logs view, texts=%q", tt.Texts())
	}

	activeProc.exit(nil)
	drainEvents(t, events, tt, true)
	if app.runner.metrics.ready {
		t.Fatal("expected metrics to reset when sing-box exits")
	}
}

func TestCompactMobileLayout(t *testing.T) {
	var (
		mu         sync.Mutex
		startedTun bool
	)
	starter := func(_ context.Context, req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
		mu.Lock()
		startedTun = req.tunMode
		mu.Unlock()
		return newFakeProcess(onLine, onDone), nil
	}

	app, _, events := newTestApp(t, []string{
		"socks://10.0.0.1:1080#alpha",
	}, withProcessStarter(starter))
	const narrowW = 360
	tt := ui.NewTester(app.View, narrowW, 740)

	if !tt.HasText("alpha") || !tt.HasText("Disconnected") {
		t.Fatalf("expected compact mobile view with alpha, texts=%q", tt.Texts())
	}

	// Open Add Node modal at 360px width and verify its primary button fits within the viewport.
	if err := tt.Click("Add Node"); err != nil {
		t.Fatalf("click Add Node: %v", err)
	}
	if btn, ok := tt.AccessByRole(ui.RoleButton, "Add"); !ok || btn.Bounds.X < 0 || btn.Bounds.X+btn.Bounds.W > narrowW {
		t.Fatalf("Add button out of 360px bounds: ok=%v bounds=%+v", ok, btn.Bounds)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatalf("click Cancel: %v", err)
	}

	if err := tt.Click("TUN"); err != nil {
		t.Fatalf("click TUN: %v", err)
	}
	if err := tt.Click("Connect alpha"); err != nil {
		t.Fatalf("click Connect alpha: %v", err)
	}
	drainEvents(t, events, tt, true)

	mu.Lock()
	tun := startedTun
	mu.Unlock()
	if !tun || app.selectedNav != navLogs {
		t.Fatalf("expected TUN connect on mobile layout, startedTun=%v nav=%q", tun, app.selectedNav)
	}
	for _, a := range tt.AccessAll() {
		if a.Role == ui.RoleButton {
			if a.Name == "" {
				t.Errorf("mobile layout: button at %v has empty accessible name", a.Bounds)
			}
			if a.Bounds.W > 0 && a.Bounds.X+a.Bounds.W > narrowW+1 {
				t.Errorf("mobile layout: button %q overflows %dpx width: %+v", a.Name, narrowW, a.Bounds)
			}
		}
	}
}

func TestStartingStateAndCancelMidStart(t *testing.T) {
	release := make(chan struct{})
	var canceled bool
	starter := func(ctx context.Context, req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
		select {
		case <-ctx.Done():
			canceled = true
			return nil, ctx.Err()
		case <-release:
			return newFakeProcess(onLine, onDone), nil
		}
	}

	app, _, events := newTestApp(t, []string{
		"socks://10.0.0.1:1080#alpha",
	}, withProcessStarter(starter))
	tt := ui.NewTester(app.View, 960, 640)

	if err := tt.Click("Connect alpha"); err != nil {
		t.Fatalf("click Connect alpha: %v", err)
	}
	if !app.runner.starting || app.runner.active {
		t.Fatalf("expected starting=true active=false, got starting=%v active=%v", app.runner.starting, app.runner.active)
	}
	if !tt.HasText("Starting · alpha") || !tt.HasText("Starting…") {
		t.Fatalf("expected Starting UI indicators, texts=%q", tt.Texts())
	}

	if err := tt.Click("Stop sing-box"); err != nil {
		t.Fatalf("click Stop sing-box while starting: %v", err)
	}
	drainEvents(t, events, tt, true)

	if !canceled {
		t.Fatal("expected start context to be canceled when stopping mid-start")
	}
	if app.runner.starting || app.runner.active || app.errMsg != "" {
		t.Fatalf("expected clean stop after mid-start cancel, starting=%v active=%v err=%q", app.runner.starting, app.runner.active, app.errMsg)
	}
	close(release)
}

func TestRemoteFetchingStateForNewAndExistingGroup(t *testing.T) {
	var mu sync.Mutex
	unblock := make(chan struct{})
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ch := unblock
		shouldFail := fail
		mu.Unlock()
		<-ch
		if shouldFail {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("socks://10.0.0.1:1080#edge-1\n"))
	}))
	defer srv.Close()

	app, _, events := newTestApp(t, nil, WithHTTPClient(srv.Client()))
	tt := ui.NewTester(app.View, 960, 640)

	app.startFetchRemote("cloud", srv.URL)
	tt.Frame()

	if !tt.HasText("cloud") || !tt.HasText("Fetching nodes…") {
		t.Fatalf("expected fetching placeholder for new remote, texts=%q", tt.Texts())
	}

	close(unblock)
	drainEvents(t, events, tt, true)

	if !tt.HasText("edge-1") || tt.HasText("Fetching nodes…") {
		t.Fatalf("expected edge-1 loaded after fetch, texts=%q", tt.Texts())
	}

	// Failed initial fetch for another new remote should remove its placeholder and show error.
	nextUnblock := make(chan struct{})
	mu.Lock()
	unblock = nextUnblock
	fail = true
	mu.Unlock()
	app.startFetchRemote("broken", srv.URL)
	tt.Frame()
	if !tt.HasText("broken") || !tt.HasText("Fetching nodes…") {
		t.Fatalf("expected fetching placeholder for broken remote, texts=%q", tt.Texts())
	}
	close(nextUnblock)
	drainEvents(t, events, tt, true)

	if app.findGroup("broken") != nil {
		t.Fatal("failed initial fetch should remove temporary placeholder group")
	}
	if app.errMsg == "" {
		t.Fatal("expected errMsg after failed remote fetch")
	}
}

func TestSyncExternalRuntimeAttachesRunningSession(t *testing.T) {
	var attachedProc *fakeProcess
	app, _, events := newTestApp(t, []string{
		"socks://10.0.0.1:1080#alpha",
	}, withExternalRuntime(
		func() (runRequest, bool) {
			return runRequest{
				groupName: "local",
				nodeTag:   "alpha",
				nodeIndex: 0,
				tunMode:   true,
			}, true
		},
		func(req runRequest, onLine func(string), onDone func(error)) (processHandle, error) {
			attachedProc = newFakeProcess(onLine, onDone)
			return attachedProc, nil
		},
	))
	tt := ui.NewTester(app.View, 960, 640)

	app.SyncRuntime()
	drainEvents(t, events, tt, true)

	if !app.runner.active || app.runner.activeTag != "alpha" || !app.tunMode {
		t.Fatalf("expected attached active session, active=%v tag=%q tun=%v", app.runner.active, app.runner.activeTag, app.tunMode)
	}
	if !tt.HasText("Connected") || !tt.HasText("Active") {
		t.Fatalf("expected Connected and Active indicators, texts=%q", tt.Texts())
	}

	attachedProc.exit(nil)
	drainEvents(t, events, tt, true)
	if app.runner.active {
		t.Fatal("expected runner to transition to inactive when external process exits")
	}
}
