// File: cmd/torque-agent/main.go
// Brief: Remote agent CLI entrypoint.

// Package main provides the torque CLI entrypoints.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ingresslabs/torque/internal/agent"
	"github.com/ingresslabs/torque/internal/workflows/buildsvc"
)

func main() {
	mode := flag.String("mode", "serve", "Runtime mode: serve or durable (durable enables mirror storage and sandboxed remote builds by default)")
	listen := flag.String("listen", ":7443", "gRPC listen address (host:port)")
	kubeconfig := flag.String("kubeconfig", "", "Path to kubeconfig for log/traffic services")
	kubeContext := flag.String("context", "", "Kubeconfig context for log/traffic services")
	token := flag.String("token", "", "Authentication token required for all RPCs (optional; sent as `authorization: Bearer <token>`)")
	httpListen := flag.String("http-listen", "", "HTTP listen address for the mirror gateway (optional; exposes /api/v1/mirror/*)")
	tlsCert := flag.String("tls-cert", "", "TLS certificate PEM file for gRPC (optional; enables TLS when set with -tls-key)")
	tlsKey := flag.String("tls-key", "", "TLS private key PEM file for gRPC (optional; enables TLS when set with -tls-cert)")
	tlsClientCA := flag.String("tls-client-ca", "", "Client CA bundle PEM file for mTLS (optional; when set, client certs are required)")
	mirrorStore := flag.String("mirror-store", "", "Path to the SQLite flight recorder DB for MirrorService (optional; enables ListSessions/Export and durable replay)")
	mirrorMaxSessions := flag.Int("mirror-max-sessions", 0, "Max number of mirror sessions to retain in the flight recorder (0 = unlimited)")
	mirrorMaxFrames := flag.Uint64("mirror-max-frames", 0, "Max frames to retain per mirror session in the flight recorder (0 = unlimited)")
	mirrorMaxBytes := flag.Int64("mirror-max-bytes", 0, "Soft cap for retained mirror DB size in bytes (0 = unlimited; best-effort)")
	mirrorPruneInterval := flag.Duration("mirror-prune-interval", 0, "How often to enforce mirror retention (0 = default)")
	buildSandbox := flag.Bool("build-sandbox", false, "Require sandbox execution for remote BuildService.RunBuild requests")
	buildSandboxConfig := flag.String("build-sandbox-config", "", "Default sandbox runtime config for remote builds (requests may override)")
	buildSandboxBin := flag.String("build-sandbox-bin", "", "Default sandbox runtime binary for remote builds")
	buildSandboxLogs := flag.Bool("build-sandbox-logs", false, "Stream sandbox runtime logs for remote builds")
	flag.Parse()

	switch strings.ToLower(strings.TrimSpace(*mode)) {
	case "", "serve":
	case "durable":
		if strings.TrimSpace(*mirrorStore) == "" {
			*mirrorStore = defaultDurableMirrorStore()
		}
		if strings.TrimSpace(*httpListen) == "" {
			*httpListen = "127.0.0.1:8081"
		}
		if !flagWasSet("build-sandbox") {
			*buildSandbox = true
		}
		if !flagWasSet("build-sandbox-logs") {
			*buildSandboxLogs = true
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: unsupported -mode %q (want serve or durable)\n", *mode)
		os.Exit(2)
	}
	if strings.TrimSpace(*token) == "" {
		*token = strings.TrimSpace(os.Getenv("TORQUE_REMOTE_TOKEN"))
	}

	cfg := agent.Config{
		ListenAddr:                *listen,
		KubeconfigPath:            *kubeconfig,
		KubeContext:               *kubeContext,
		AuthToken:                 *token,
		HTTPListenAddr:            *httpListen,
		TLSCertFile:               *tlsCert,
		TLSKeyFile:                *tlsKey,
		TLSClientCAFile:           *tlsClientCA,
		MirrorStore:               *mirrorStore,
		MirrorMaxSessions:         *mirrorMaxSessions,
		MirrorMaxFramesPerSession: *mirrorMaxFrames,
		MirrorMaxBytes:            *mirrorMaxBytes,
		MirrorPruneInterval:       *mirrorPruneInterval,
		BuildRequireSandbox:       *buildSandbox,
		BuildSandboxConfig:        *buildSandboxConfig,
		BuildSandboxBin:           *buildSandboxBin,
		BuildSandboxLogs:          *buildSandboxLogs,
	}
	srv, err := agent.New(cfg, buildsvc.New(buildsvc.Dependencies{}))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func flagWasSet(name string) bool {
	seen := false
	flag.Visit(func(f *flag.Flag) {
		if f != nil && f.Name == name {
			seen = true
		}
	})
	return seen
}

func defaultDurableMirrorStore() string {
	if os.Getuid() == 0 {
		return "/var/lib/torque/agent/mirror.sqlite"
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".torque", "agent", "mirror.sqlite")
	}
	return filepath.Join(os.TempDir(), "torque-agent", "mirror.sqlite")
}
