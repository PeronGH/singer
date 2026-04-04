package tui

// Node-level action messages (bubbled up from group delegate).
type (
	connectMsg    struct{ nodeURI string }
	addNodeMsg    struct{}
	editNodeMsg   struct{ nodeURI string }
	deleteNodeMsg struct{ index int }
	refreshMsg    struct{}
)

// Input result messages.
type (
	inputResultMsg struct{ value string }
	inputCancelMsg struct{}
)

// Subscription fetch messages.
type fetchDoneMsg struct {
	name  string
	url   string
	nodes []string
	err   error
}

// Runner messages (also used in runner.go).
type (
	logLineMsg    string
	runnerDoneMsg struct{ err error }
)
