package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"

	"aether-dots/internal/midnightsup"
	"aether-dots/internal/store"
)

// midnightServerCommand is the opt-in supervisor. Nothing starts Midnight
// implicitly: approve a binary once, then start/status/stop explicitly.
func midnightServerCommand(db *store.Store, args []string) error {
	usage := errors.New("usage: aether midnight server <approve|start|status|stop>\n  approve --binary /abs/aether-midnight --db /abs/midnight.db --yes\n  start | status | stop")
	if len(args) == 0 {
		return usage
	}
	dir := db.Dir()
	switch args[0] {
	case "approve":
		fs := flag.NewFlagSet("approve", flag.ContinueOnError)
		binary := fs.String("binary", "", "absolute path of the aether-midnight binary")
		dbPath := fs.String("db", "", "absolute path of the Midnight database")
		yes := fs.Bool("yes", false, "approve this exact binary (by SHA-256) to run as a local server")
		if err := fs.Parse(args[1:]); err != nil || *binary == "" || *dbPath == "" {
			return usage
		}
		if !*yes {
			return errors.New("approving a binary lets `aether midnight server start` run it; re-run with --yes to confirm")
		}
		cfg, err := midnightsup.Approve(dir, *binary, *dbPath)
		if err != nil {
			return err
		}
		fmt.Printf("approved %s\n  sha256 %s\n  db     %s\n", cfg.Binary, cfg.SHA256, cfg.DB)
		return nil
	case "start":
		st, err := midnightsup.Start(dir)
		if err != nil {
			return err
		}
		printSupervisorStatus(st)
		return nil
	case "stop":
		stopped, err := midnightsup.Stop(dir, 5*time.Second)
		if err != nil {
			return err
		}
		if stopped {
			fmt.Println("Midnight stopped")
		} else {
			fmt.Println("Midnight was not running")
		}
		return nil
	case "status":
		st, err := midnightsup.Get(dir)
		if err != nil {
			return err
		}
		printSupervisorStatus(st)
		if st.Running {
			reportDotConnections(db, st.Endpoint)
		}
		return nil
	}
	return usage
}

func printSupervisorStatus(st midnightsup.Status) {
	switch {
	case !st.Approved:
		fmt.Println("midnight server: no binary approved")
	case !st.Running:
		fmt.Printf("midnight server: stopped (approved %s)\n", st.Config.Binary)
	default:
		auth := "AUTH NOT ENFORCED"
		if st.AuthEnforced {
			auth = "auth enforced"
		}
		fmt.Printf("midnight server: running pid=%d %s (%s) since %s\n", st.PID, st.Endpoint, auth, st.StartedAt.Format(time.RFC3339))
	}
}

// reportDotConnections checks, with each Dot's own token, every per-Dot
// connection on the supervised server. After a restart the port is unchanged,
// so valid connections keep working and revoked ones show as REJECTED.
func reportDotConnections(db *store.Store, endpoint string) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return
	}
	snapshot, err := db.SnapshotChecked()
	if err != nil {
		fmt.Println("  connections: unavailable:", err)
		return
	}
	for _, server := range snapshot.MCPServers {
		su, err := url.Parse(server.Endpoint)
		if err != nil || su.Host != u.Host || !strings.HasPrefix(server.Name, "Midnight dot-") {
			continue
		}
		options, err := db.MCPConnectOptions(server.ID)
		status := "ok"
		if err != nil {
			status = "TOKEN ERROR"
		} else if _, err := discoverTools(context.Background(), server.Endpoint, options...); err != nil {
			status = "REJECTED/UNREACHABLE"
		}
		fmt.Printf("  %s: %s\n", server.Name, status)
	}
}
