# Packaging (deb/rpm)

This directory contains a Docker-based packaging workflow that builds Linux `deb` and `rpm` packages for the torque toolkit without requiring Ruby/FPM tooling on the host.

## Usage

```bash
make package
```

Artifacts are written into `./dist/` and should not be committed.

The packages install:

- `torque`
- `torque-agent`
- `verifier`
- `verify` (deprecated compatibility binary; prefer `verifier`)
- `torque-package`
- `torque-mcp`
- `torque-agent.service` and `torque-mcp.service` systemd units

## Customization

- `PACKAGE_PLATFORMS` controls which Linux platforms to build (default: `linux/amd64`).
- `VERSION` and `LDFLAGS` are inherited from the Makefile defaults.

Example:

```bash
make package PACKAGE_PLATFORMS="linux/amd64 linux/arm64" VERSION=dev
```

## Systemd Daemon Mode

After installing a package, create `/etc/torque/agent.env` from
`/etc/torque/agent.env.example`, set `TORQUE_REMOTE_TOKEN` and
`TORQUE_MCP_TOKEN`, then start the durable services:

```bash
sudo install -m 0600 /etc/torque/agent.env.example /etc/torque/agent.env
sudo systemctl enable --now torque-agent.service torque-mcp.service
```

`torque-agent.service` runs `torque-agent -mode=durable`, persists MirrorService
frames under `/var/lib/torque/agent/`, and requires sandbox execution for remote
builds. `torque-mcp.service` serves authenticated HTTP MCP on
`127.0.0.1:7331` and bridges to the local gRPC agent on `127.0.0.1:7443`.
