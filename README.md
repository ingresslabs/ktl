# ktl

Agent-first Kubernetes delivery CLI.

`ktl` gives humans and AI agents one reliable loop for Kubernetes delivery:
build, verify, plan, apply, capture evidence, and explain what happened.

**Docs:** https://ingresslabs.github.io/ktl/

<p align="center">
  <a href="https://ingresslabs.github.io/ktl/">
    <img src="docs/assets/ktl-showcase.gif" alt="ktl showcase" width="900">
  </a>
</p>

## Ship

```bash
ktl ship --chart ./chart --release api -n prod \
  --build . --tag ghcr.io/acme/api:dev --yes
```

Runs build -> verify -> plan -> apply -> capture -> explain.

## Features

- Golden deploy workflow with one trusted command.
- Portable evidence for builds, deploys, logs, and stacks.
- Reviewable Helm plans, diffs, Markdown, and visual artifacts.
- Agent automation through `ktl-agent` gRPC workflows.
- BuildKit, SBOM/provenance, verifier reports, and policy checks.

## Install

Requires Go 1.25.9+.

```bash
go install github.com/ingresslabs/ktl/cmd/ktl@latest
go install github.com/ingresslabs/ktl/cmd/verifier@latest
```

From a checkout:

```bash
make build
./bin/ktl --help
```
