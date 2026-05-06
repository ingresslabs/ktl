package agent

import (
	"testing"

	apiv1 "github.com/ingresslabs/torque/pkg/api/torque/api/v1"
)

func TestBuildServerAppliesDaemonSandboxDefaults(t *testing.T) {
	srv := &BuildServer{
		RequireSandbox: true,
		SandboxConfig:  "/etc/torque/sandbox/linux-ci.cfg",
		SandboxBin:     "/usr/local/bin/nsjail",
		SandboxLogs:    true,
	}

	opts := srv.buildOptions(&apiv1.RunBuildRequest{Options: &apiv1.BuildOptions{
		ContextDir: ".",
		Tags:       []string{"registry.local/app:test"},
	}})

	if !opts.RequireSandbox {
		t.Fatalf("expected daemon default to require sandbox")
	}
	if opts.SandboxConfig != "/etc/torque/sandbox/linux-ci.cfg" {
		t.Fatalf("sandbox config default not applied: %q", opts.SandboxConfig)
	}
	if opts.SandboxBin != "/usr/local/bin/nsjail" {
		t.Fatalf("sandbox bin default not applied: %q", opts.SandboxBin)
	}
	if !opts.SandboxLogs {
		t.Fatalf("expected daemon default to enable sandbox logs")
	}
}

func TestBuildServerKeepsRequestSandboxOverrides(t *testing.T) {
	srv := &BuildServer{
		RequireSandbox: true,
		SandboxConfig:  "/etc/torque/sandbox/linux-ci.cfg",
		SandboxBin:     "/usr/local/bin/nsjail",
		SandboxLogs:    true,
	}

	opts := srv.buildOptions(&apiv1.RunBuildRequest{Options: &apiv1.BuildOptions{
		SandboxConfig: "/tmp/custom.cfg",
		SandboxBin:    "/opt/nsjail",
		SandboxLogs:   false,
	}})

	if !opts.RequireSandbox {
		t.Fatalf("daemon should still require sandbox")
	}
	if opts.SandboxConfig != "/tmp/custom.cfg" {
		t.Fatalf("request sandbox config override lost: %q", opts.SandboxConfig)
	}
	if opts.SandboxBin != "/opt/nsjail" {
		t.Fatalf("request sandbox bin override lost: %q", opts.SandboxBin)
	}
	if !opts.SandboxLogs {
		t.Fatalf("daemon sandbox logs should remain enabled")
	}
}
