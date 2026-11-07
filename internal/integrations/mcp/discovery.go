package mcp

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	loopbackDiscoveryWorkers = 256
	loopbackDiscoveryTimeout = 20 * time.Second
	loopbackProbeTimeout     = 1200 * time.Millisecond
)

type loopbackEndpoint struct {
	host    string
	network string
	port    int
}

// DiscoverLocalServers checks TCP listeners on IPv4 and IPv6 loopback, then
// tries only common Streamable HTTP MCP paths. It never scans a remote host.
func DiscoverLocalServers(ctx context.Context, onProgress func(int)) ([]DiscoveredServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, loopbackDiscoveryTimeout)
	defer cancel()
	reportProgress := onProgress
	if onProgress != nil {
		var progressMu sync.Mutex
		reportProgress = func(percent int) {
			progressMu.Lock()
			defer progressMu.Unlock()
			onProgress(percent)
		}
	}
	open, scanErr := scanLoopbackListeners(ctx, reportProgress)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(open) == 0 {
		return nil, scanErr
	}

	var completed atomic.Int64
	servers := make([]DiscoveredServer, 0)
	var serversMu sync.Mutex
	var wait sync.WaitGroup
	jobs := make(chan loopbackEndpoint)
	workers := min(loopbackDiscoveryWorkers, len(open))
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for endpoint := range jobs {
				if server, ok := probeLoopbackEndpoint(ctx, endpoint); ok {
					serversMu.Lock()
					servers = append(servers, server)
					serversMu.Unlock()
				}
				finished := completed.Add(1)
				if reportProgress != nil {
					percent := 70 + int(finished*30/int64(len(open)))
					reportProgress(min(percent, 99))
				}
			}
		}()
	}
	for _, endpoint := range open {
		select {
		case jobs <- endpoint:
		case <-ctx.Done():
			close(jobs)
			wait.Wait()
			sortDiscoveredServers(servers)
			return servers, ctx.Err()
		}
	}
	close(jobs)
	wait.Wait()
	sortDiscoveredServers(servers)
	if reportProgress != nil {
		reportProgress(100)
	}
	if ctx.Err() != nil {
		return servers, ctx.Err()
	}
	return servers, scanErr
}

func scanLoopbackListeners(ctx context.Context, onProgress func(int)) ([]loopbackEndpoint, error) {
	total := int64(2 * 65535)
	jobs := make(chan loopbackEndpoint, loopbackDiscoveryWorkers*2)
	var wait sync.WaitGroup
	var scanned atomic.Int64
	var listenersMu sync.Mutex
	var listeners []loopbackEndpoint
	dialer := &net.Dialer{Timeout: 100 * time.Millisecond}
	for range loopbackDiscoveryWorkers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for endpoint := range jobs {
				address := net.JoinHostPort(endpoint.host, strconv.Itoa(endpoint.port))
				connection, err := dialer.DialContext(ctx, endpoint.network, address)
				if err == nil {
					_ = connection.Close()
					listenersMu.Lock()
					listeners = append(listeners, endpoint)
					listenersMu.Unlock()
				}
				count := scanned.Add(1)
				if onProgress != nil && count%4096 == 0 {
					onProgress(int(count * 70 / total))
				}
			}
		}()
	}
	for _, target := range []loopbackEndpoint{{host: "127.0.0.1", network: "tcp4"}, {host: "::1", network: "tcp6"}} {
		for port := 1; port <= 65535; port++ {
			select {
			case jobs <- loopbackEndpoint{host: target.host, network: target.network, port: port}:
			case <-ctx.Done():
				close(jobs)
				wait.Wait()
				sortLoopbackEndpoints(listeners)
				return listeners, ctx.Err()
			}
		}
	}
	close(jobs)
	wait.Wait()
	sortLoopbackEndpoints(listeners)
	return listeners, ctx.Err()
}

func probeLoopbackEndpoint(parent context.Context, endpoint loopbackEndpoint) (DiscoveredServer, bool) {
	for _, path := range []string{"/mcp", "/"} {
		address := net.JoinHostPort(endpoint.host, strconv.Itoa(endpoint.port))
		probeCtx, cancel := context.WithTimeout(parent, loopbackProbeTimeout)
		session, err := Connect(probeCtx, "0.1.0", fmt.Sprintf("http://%s%s", address, path))
		if err != nil {
			cancel()
			if parent.Err() != nil {
				return DiscoveredServer{}, false
			}
			continue
		}
		tools, listErr := session.ListTools(probeCtx)
		name, version := session.ServerIdentity()
		_ = session.Close()
		cancel()
		if listErr == nil {
			return DiscoveredServer{
				Endpoint: fmt.Sprintf("http://%s%s", address, path),
				Name:     name,
				Version:  version,
				Tools:    tools,
			}, true
		}
		if parent.Err() != nil {
			return DiscoveredServer{}, false
		}
		// A successful handshake proves the service is MCP even when tool
		// listing is temporarily unavailable; do not probe another path.
		return DiscoveredServer{}, false
	}
	return DiscoveredServer{}, false
}

func sortLoopbackEndpoints(endpoints []loopbackEndpoint) {
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].port != endpoints[j].port {
			return endpoints[i].port < endpoints[j].port
		}
		return endpoints[i].host < endpoints[j].host
	})
}

func sortDiscoveredServers(servers []DiscoveredServer) {
	sort.Slice(servers, func(i, j int) bool {
		if servers[i].Name != servers[j].Name {
			return servers[i].Name < servers[j].Name
		}
		return servers[i].Endpoint < servers[j].Endpoint
	})
}
