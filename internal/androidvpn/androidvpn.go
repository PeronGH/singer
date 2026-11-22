// Package androidvpn bridges Singer's Android UI to an isolated :vpn process
// running sing-box via libbox.
package androidvpn

import "context"

// StartRequest describes one VPN or local-proxy run request for the :vpn process.
type StartRequest struct {
	ConfigJSON string
	TunMode    bool
	GroupName  string
	NodeTag    string
	NodeIndex  int
	APIAddr    string
	APISecret  string
}

// Status reports whether the :vpn process is running and its active session metadata.
type Status struct {
	Running   bool
	TunMode   bool
	GroupName string
	NodeTag   string
	NodeIndex int
	APIAddr   string
	APISecret string
}

// Service controls the isolated Android :vpn process running sing-box via libbox.
//
//mygo:native
type Service interface {
	// Start ensures Android VPN and notification permissions, starts the :vpn
	// foreground service with req, and returns once sing-box is running.
	Start(ctx context.Context, req StartRequest) error
	// Stop stops the running :vpn service cleanly.
	Stop() error
	// Status reports the current live state of the :vpn service.
	Status() (Status, error)
}
