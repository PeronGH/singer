package gui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

const defaultMetricsInterval = time.Second

type clashAPIInfo struct {
	addr   string
	secret string
}

type trafficMetrics struct {
	ready       bool
	upRate      int64
	downRate    int64
	upTotal     int64
	downTotal   int64
	connections int
	memory      uint64
}

type connectionsSnapshot struct {
	DownloadTotal int64             `json:"downloadTotal"`
	UploadTotal   int64             `json:"uploadTotal"`
	Connections   []json.RawMessage `json:"connections"`
	Memory        uint64            `json:"memory"`
}

func allocateClashAPI() (clashAPIInfo, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return clashAPIInfo{}, err
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return clashAPIInfo{}, err
	}

	return clashAPIInfo{
		addr:   addr,
		secret: hex.EncodeToString(raw[:]),
	}, nil
}

func newLocalHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
}

func fetchConnectionsSnapshot(ctx context.Context, client *http.Client, api clashAPIInfo) (connectionsSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+api.addr+"/connections", nil)
	if err != nil {
		return connectionsSnapshot{}, err
	}
	if api.secret != "" {
		req.Header.Set("Authorization", "Bearer "+api.secret)
	}

	resp, err := client.Do(req)
	if err != nil {
		return connectionsSnapshot{}, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return connectionsSnapshot{}, fmt.Errorf("clash api status: %s", resp.Status)
	}

	var snap connectionsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return connectionsSnapshot{}, err
	}
	return snap, nil
}

func runMetricsPoller(ctx context.Context, client *http.Client, api clashAPIInfo, interval time.Duration, onUpdate func(trafficMetrics)) {
	if api.addr == "" {
		return
	}
	if interval <= 0 {
		interval = defaultMetricsInterval
	}

	var (
		hasPrev  bool
		prevUp   int64
		prevDown int64
		prevTime time.Time
		lastSent trafficMetrics
	)

	poll := func() {
		now := time.Now()
		snap, err := fetchConnectionsSnapshot(ctx, client, api)
		if err != nil {
			return
		}

		m := trafficMetrics{
			ready:       true,
			upTotal:     snap.UploadTotal,
			downTotal:   snap.DownloadTotal,
			connections: len(snap.Connections),
			memory:      snap.Memory,
		}
		if hasPrev {
			if dt := now.Sub(prevTime).Seconds(); dt > 0 {
				if du := snap.UploadTotal - prevUp; du > 0 {
					m.upRate = int64(float64(du) / dt)
				}
				if dd := snap.DownloadTotal - prevDown; dd > 0 {
					m.downRate = int64(float64(dd) / dt)
				}
			}
		}

		hasPrev = true
		prevUp = snap.UploadTotal
		prevDown = snap.DownloadTotal
		prevTime = now

		if m != lastSent {
			lastSent = m
			onUpdate(m)
		}
	}

	poll()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}

func formatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func formatRate(bytesPerSec int64) string {
	return formatBytes(bytesPerSec) + "/s"
}
