// Package androidvpn bridges Singer's Android UI to an isolated :vpn process
// running sing-box via libbox.
package androidvpn

import "context"

// Service controls the isolated Android :vpn process running sing-box via libbox.
//
//mygo:native
type Service interface {
	// Start ensures Android VPN consent when tunMode is true, starts the :vpn
	// foreground service with configJSON, and returns once sing-box is running.
	Start(ctx context.Context, configJSON string, tunMode bool) error
	// Stop stops the running :vpn service cleanly.
	Stop() error
}
