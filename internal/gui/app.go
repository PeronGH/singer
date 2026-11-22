package gui

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/PeronGH/singer/internal/store"
	"github.com/PeronGH/singer/sub"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	navLogs        = "logs"
	navGroupPrefix = "group:"
)

// Option configures an App instance.
type Option func(*App)

// WithHTTPClient overrides the HTTP client used to fetch remote subscriptions.
func WithHTTPClient(client *http.Client) Option {
	return func(a *App) {
		if client != nil {
			a.client = client
		}
	}
}

// WithDispatcher overrides how background goroutines marshal updates onto the UI thread.
func WithDispatcher(fn func(func())) Option {
	return func(a *App) {
		if fn != nil {
			a.dispatch = fn
		}
	}
}

func withProcessStarter(starter processStarter) Option {
	return func(a *App) {
		a.runner = newRunner(starter)
	}
}

type toastRequest struct {
	message     string
	actionLabel string
	onAction    func()
}

// App holds the state and view logic for the Singer desktop UI.
type App struct {
	store    *store.Store
	client   *http.Client
	win      *mygo.Window
	dispatch func(func())

	groups       []store.Group
	selectedNav  string
	lastNav      string
	query        string
	nodeList     ui.ListState
	selectedNode int
	searchHandle ui.Handle

	runner  *runner
	logList ui.ListState

	fetching map[string]bool

	remoteModalOpen         bool
	remoteName              string
	remoteURL               string
	remoteNameErr           string
	remoteURLErr            string
	nodeModalOpen           bool
	editingNodeIndex        int
	nodeURI                 string
	nodeURIErr              string
	confirmDeleteRemoteOpen bool
	confirmDeleteRemoteName string

	errMsg        string
	pendingToasts []toastRequest
}

// New initializes the App by loading persisted proxy groups from s.
func New(s *store.Store, opts ...Option) (*App, error) {
	groups, err := s.LoadGroups()
	if err != nil {
		return nil, err
	}

	selectedNav := navLogs
	if len(groups) > 0 {
		selectedNav = navGroupPrefix + groups[0].Name
	}

	a := &App{
		store:            s,
		client:           &http.Client{Timeout: 15 * time.Second},
		groups:           groups,
		selectedNav:      selectedNav,
		lastNav:          selectedNav,
		selectedNode:     -1,
		editingNodeIndex: -1,
		runner:           newRunner(nil),
		logList:          ui.ListState{FollowEnd: true},
		fetching:         make(map[string]bool),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// AttachWindow binds the MyGo window so background work updates the UI safely on the main thread.
func (a *App) AttachWindow(win *mygo.Window) {
	a.win = win
	if a.dispatch == nil && win != nil {
		a.dispatch = win.Update
	}
}

// Shutdown stops any running sing-box process and cancels queued restarts.
func (a *App) Shutdown() {
	a.runner.shutdown()
}

// View builds the full Singer desktop UI for the current frame.
func (a *App) View(c *ui.Context) {
	a.flushToasts(c)
	a.handleGlobalShortcuts(c)

	if a.selectedNav != a.lastNav {
		a.lastNav = a.selectedNav
		a.query = ""
		a.selectedNode = -1
		a.nodeList = ui.ListState{}
	}

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebarView(c)
		ui.Divider(c)
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			if a.selectedNav == navLogs {
				a.logsView(c)
				return
			}
			if g := a.activeGroup(); g != nil {
				a.groupView(c, g)
				return
			}
			a.logsView(c)
		})
	})

	a.dialogsView(c)
}

func (a *App) handleGlobalShortcuts(c *ui.Context) {
	if a.remoteModalOpen || a.nodeModalOpen || a.confirmDeleteRemoteOpen {
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyN) {
		a.openAddRemoteModal()
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) && a.activeGroup() != nil {
		a.searchHandle.SelectAll(c)
		return
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyR) {
		a.refreshAllRemotes()
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyR) {
		if g := a.activeGroup(); g != nil && g.IsRemote() {
			a.startFetchRemote(g.Name, g.URL)
		}
		return
	}
	if a.selectedNav == navLogs && !c.FocusTakesText() && c.Shortcut(0, ui.KeyEscape) && a.runner.active {
		a.stopRunner()
	}
}

func (a *App) flushToasts(c *ui.Context) {
	if len(a.pendingToasts) == 0 {
		return
	}
	toasts := a.pendingToasts
	a.pendingToasts = nil
	for _, tr := range toasts {
		if tr.actionLabel != "" {
			c.ToastAction(tr.message, tr.actionLabel, tr.onAction)
		} else {
			c.Toast(tr.message)
		}
	}
}

func (a *App) queueToast(msg string) {
	a.pendingToasts = append(a.pendingToasts, toastRequest{message: msg})
}

func (a *App) queueToastAction(msg, actionLabel string, onAction func()) {
	a.pendingToasts = append(a.pendingToasts, toastRequest{
		message:     msg,
		actionLabel: actionLabel,
		onAction:    onAction,
	})
}

func (a *App) dispatchUI(fn func()) {
	if a.dispatch != nil {
		a.dispatch(fn)
		return
	}
	if a.win != nil {
		a.win.Update(fn)
		return
	}
	fn()
}

func (a *App) activeGroup() *store.Group {
	name, ok := strings.CutPrefix(a.selectedNav, navGroupPrefix)
	if !ok {
		return nil
	}
	return a.findGroup(name)
}

func (a *App) findGroup(name string) *store.Group {
	for i := range a.groups {
		if a.groups[i].Name == name {
			return &a.groups[i]
		}
	}
	return nil
}

func (a *App) localGroup() *store.Group {
	for i := range a.groups {
		if a.groups[i].IsLocal() {
			return &a.groups[i]
		}
	}
	return nil
}

func (a *App) remoteGroups() []store.Group {
	remotes := make([]store.Group, 0, len(a.groups))
	for _, g := range a.groups {
		if g.IsRemote() {
			remotes = append(remotes, g)
		}
	}
	return remotes
}

func (a *App) connectNode(groupName string, nodeIndex int) {
	g := a.findGroup(groupName)
	if g == nil {
		return
	}

	_, outbounds, err := sub.ParseBody([]byte(strings.Join(g.Nodes, "\n")))
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	if nodeIndex < 0 || nodeIndex >= len(outbounds) {
		return
	}

	cfg, err := sub.Config(outbounds, outbounds[nodeIndex].Tag)
	if err != nil {
		a.errMsg = err.Error()
		return
	}

	configJSON, err := marshalJSON(cfg)
	if err != nil {
		a.errMsg = err.Error()
		return
	}

	a.errMsg = ""
	a.selectedNav = navLogs
	req := runRequest{
		groupName:  groupName,
		nodeTag:    extractTag(g.Nodes[nodeIndex]),
		nodeIndex:  nodeIndex,
		configJSON: configJSON,
	}
	a.runner.connect(req, a.dispatchUI, func(err error) {
		if err != nil {
			a.errMsg = err.Error()
		} else {
			a.errMsg = ""
		}
	})
}

func (a *App) stopRunner() {
	a.runner.pending = nil
	a.runner.stop()
}

func (a *App) startFetchRemote(name, remoteURL string) {
	if a.fetching[name] {
		return
	}
	a.errMsg = ""
	a.fetching[name] = true

	client := a.client
	go func() {
		group, err := fetchRemote(client, name, remoteURL)
		a.dispatchUI(func() {
			delete(a.fetching, name)
			if err != nil {
				a.errMsg = err.Error()
				return
			}
			if err := a.store.UpsertRemote(group.Name, group.URL, group.Nodes); err != nil {
				a.errMsg = err.Error()
				return
			}
			a.errMsg = ""
			a.upsertGroup(group)
			a.queueToast("Updated " + group.Name)
		})
	}()
}

func (a *App) refreshAllRemotes() {
	remotes := a.remoteGroups()
	for _, g := range remotes {
		a.startFetchRemote(g.Name, g.URL)
	}
}

func (a *App) upsertGroup(data store.Group) {
	found := false
	for i := range a.groups {
		if a.groups[i].Name == data.Name {
			a.groups[i] = data
			found = true
			break
		}
	}
	if !found {
		a.groups = append(a.groups, data)
	}
	a.sortGroups()
	a.selectedNav = navGroupPrefix + data.Name
}

func (a *App) sortGroups() {
	slices.SortStableFunc(a.groups, func(x, y store.Group) int {
		switch {
		case x.IsLocal() && !y.IsLocal():
			return -1
		case !x.IsLocal() && y.IsLocal():
			return 1
		default:
			return strings.Compare(x.Name, y.Name)
		}
	})
}

func (a *App) deleteRemote(name string) {
	if err := a.store.DeleteRemote(name); err != nil {
		a.errMsg = err.Error()
		return
	}
	a.errMsg = ""
	for i := range a.groups {
		if a.groups[i].Name == name {
			a.groups = slices.Delete(a.groups, i, i+1)
			break
		}
	}
	if a.selectedNav == navGroupPrefix+name {
		if len(a.groups) > 0 {
			a.selectedNav = navGroupPrefix + a.groups[0].Name
		} else {
			a.selectedNav = navLogs
		}
	}
	a.queueToast("Deleted remote " + name)
}

func (a *App) saveLocalNodes(nodes []string, toastMsg string) bool {
	if err := a.store.SaveLocal(nodes); err != nil {
		a.errMsg = err.Error()
		return false
	}
	a.errMsg = ""
	if local := a.localGroup(); local != nil {
		local.Nodes = append([]string(nil), nodes...)
	}
	if toastMsg != "" {
		a.queueToast(toastMsg)
	}
	return true
}

func (a *App) deleteLocalNode(index int) {
	local := a.localGroup()
	if local == nil {
		a.errMsg = "local group not loaded"
		return
	}
	if index < 0 || index >= len(local.Nodes) {
		return
	}

	previous := append([]string(nil), local.Nodes...)
	next := append(append([]string(nil), local.Nodes[:index]...), local.Nodes[index+1:]...)
	if !a.saveLocalNodes(next, "") {
		return
	}

	a.queueToastAction("Deleted local node", "Undo", func() {
		a.saveLocalNodes(previous, "Restored local node")
	})
}

func (a *App) openAddRemoteModal() {
	a.remoteName = ""
	a.remoteURL = ""
	a.remoteNameErr = ""
	a.remoteURLErr = ""
	a.remoteModalOpen = true
}

func (a *App) openAddLocalNodeModal() {
	a.editingNodeIndex = -1
	a.nodeURI = ""
	a.nodeURIErr = ""
	a.nodeModalOpen = true
}

func (a *App) openEditLocalNodeModal(index int, uri string) {
	a.editingNodeIndex = index
	a.nodeURI = uri
	a.nodeURIErr = ""
	a.nodeModalOpen = true
}

func (a *App) errorBannerView(c *ui.Context) {
	if a.errMsg == "" {
		return
	}
	t := c.Theme()
	ui.Row(c).Padding(8, 12).Gap(8).Radius(t.Radius).
		Background(t.Danger.Alpha(0.12)).Border(1, t.Danger.Alpha(0.4)).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.errMsg).TextColor(t.Danger).Selectable().Grow(1)
		if ui.Button(c, "Dismiss").Label("Dismiss error").Clicked() {
			a.errMsg = ""
		}
	})
}
