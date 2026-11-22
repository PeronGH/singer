// Package androidvpn bridges Singer's Android UI to an isolated :vpn process
// running sing-box via libbox over Android Binder IPC.
package androidvpn

import (
	"context"
	"iter"
)

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

// Status reports a live state snapshot pushed over Binder from the :vpn process.
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
	// foreground service with req, and returns once :vpn reports running over Binder.
	Start(ctx context.Context, req StartRequest) error
	// Stop stops the running :vpn service cleanly over Binder / service intent.
	Stop() error
	// Watch streams live state updates pushed over Binder from the :vpn process.
	Watch(ctx context.Context) iter.Seq2[Status, error]
}
