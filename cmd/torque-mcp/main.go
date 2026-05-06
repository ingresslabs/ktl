package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ingresslabs/torque/internal/mcpserver"
)

func main() {
	cfg := mcpserver.ConfigFromEnv()
	stdio := flag.Bool("stdio", false, "Serve MCP over stdio")
	listen := flag.String("listen", "", "Serve MCP over HTTP at this address (POST /mcp)")
	flag.StringVar(&cfg.AuthToken, "auth-token", cfg.AuthToken, "Bearer token required for HTTP MCP requests (also via TORQUE_MCP_TOKEN)")
	flag.StringVar(&cfg.RemoteAgent, "remote-agent", cfg.RemoteAgent, "Remote torque-agent gRPC endpoint")
	flag.StringVar(&cfg.RemoteToken, "remote-token", cfg.RemoteToken, "Remote torque-agent bearer token")
	flag.BoolVar(&cfg.RemoteTLS, "remote-tls", cfg.RemoteTLS, "Use TLS for remote torque-agent")
	flag.StringVar(&cfg.RemoteTLSCA, "remote-tls-ca", cfg.RemoteTLSCA, "CA bundle for remote torque-agent TLS")
	flag.StringVar(&cfg.RemoteTLSServerName, "remote-tls-server-name", cfg.RemoteTLSServerName, "Server name override for remote TLS")
	flag.BoolVar(&cfg.RemoteTLSInsecureSkipVerify, "remote-tls-insecure-skip-verify", cfg.RemoteTLSInsecureSkipVerify, "Skip remote TLS verification")
	flag.StringVar(&cfg.RemoteTLSClientCert, "remote-tls-client-cert", cfg.RemoteTLSClientCert, "Client certificate for remote mTLS")
	flag.StringVar(&cfg.RemoteTLSClientKey, "remote-tls-client-key", cfg.RemoteTLSClientKey, "Client key for remote mTLS")
	flag.BoolVar(&cfg.EnableWrite, "enable-write", cfg.EnableWrite, "Enable mutating MCP tools when requests also pass safety.confirm=true")
	flag.IntVar(&cfg.MaxEventsReturned, "max-events", cfg.MaxEventsReturned, "Maximum events returned inline from a tool")
	flag.IntVar(&cfg.MaxLogLinesReturned, "max-log-lines", cfg.MaxLogLinesReturned, "Maximum log lines returned inline from torque.logs.query")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := mcpserver.New(cfg)
	switch {
	case *stdio:
		if err := srv.ServeStdio(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "torque-mcp stdio: %v\n", err)
			os.Exit(1)
		}
	case strings.TrimSpace(*listen) != "":
		if err := srv.ServeHTTP(ctx, strings.TrimSpace(*listen)); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "torque-mcp http: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: torque-mcp --stdio OR torque-mcp --listen 127.0.0.1:7331")
		os.Exit(2)
	}
}
