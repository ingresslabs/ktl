# Torque MCP Server Spec

Status: draft

Last reviewed: 2026-05-06

Target MCP revision: 2025-11-25

## References

- MCP server primitives: https://modelcontextprotocol.io/specification/2025-11-25/server/index
- MCP tools: https://modelcontextprotocol.io/specification/2025-11-25/server/tools
- MCP resources: https://modelcontextprotocol.io/specification/2025-11-25/server/resources
- MCP prompts: https://modelcontextprotocol.io/specification/2025-11-25/server/prompts
- MCP roots: https://modelcontextprotocol.io/specification/2025-11-25/client/roots
- MCP progress: https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/progress
- MCP tasks: https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/tasks
- MCP schema reference: https://modelcontextprotocol.io/specification/2025-11-25/schema
- Existing Torque gRPC agent API: [grpc-agent.md](grpc-agent.md)
- Existing protobuf boundary: [agent.proto](../proto/torque/api/v1/agent.proto)

## Problem

Torque is already agent-oriented, but agents currently have two imperfect integration paths:

1. Shell out to the `torque` CLI and parse terminal output.
2. Speak the `torque-agent` gRPC API directly.

The CLI path is easy but opaque. It hides schemas, makes long-running output hard to resume, and pushes too much safety policy into prompt text. The gRPC path is structured but not discoverable by MCP-capable agents and does not expose prompts/resources in the way agent hosts expect.

The MCP server should make Torque's delivery loop discoverable, structured, auditable, and safe for model-driven use. The server is not a replacement for `torque` or `torque-agent`; it is an agent-facing adapter over the same internal build, verify, plan, apply, log, stack, and capture primitives.

## Goals

- Expose Torque capabilities through MCP tools, resources, and prompts with stable JSON schemas.
- Preserve Torque's evidence-first model: every expensive or mutating operation returns session and artifact resources.
- Make safe read-only workflows easy for agents: inspect, verify, plan, summarize, and diagnose.
- Make write workflows explicit: build push, apply, delete, revert, and stack mutations require server policy approval plus request-level confirmation.
- Support local desktop agents over stdio first.
- Support remote cluster/builder access by bridging to `torque-agent` gRPC instead of duplicating remote execution logic.
- Restrict filesystem access to MCP roots or explicit configured allowlists.
- Never return raw secret values to the model.
- Keep tool names, inputs, and outputs stable enough for automated evaluation.

## Non-goals

- Do not expose arbitrary shell, raw `kubectl`, raw Helm, or raw SQL execution.
- Do not replace Kubernetes RBAC, registry auth, cosign auth, or `torque-agent` auth.
- Do not make mutating operations model-autonomous by default.
- Do not expose kubeconfig contents, secret provider values, or unredacted rendered values.
- Do not implement a general MCP gateway for non-Torque tools.
- Do not require experimental MCP tasks for the first usable version.

## Product Shape

Add a new binary:

```bash
torque-mcp --stdio
torque-mcp --listen 127.0.0.1:7331 --auth-token "$TORQUE_MCP_TOKEN"
torque-mcp --remote-agent 127.0.0.1:7443 --remote-token "$TORQUE_REMOTE_TOKEN"
```

Optionally add an alias later:

```bash
torque mcp serve --stdio
```

The separate binary is preferred for agent clients because it keeps the MCP process model simple, avoids bloating the primary CLI help, and allows package managers to advertise the server entrypoint directly.

## Deployment Modes

| Mode | Transport | Execution backend | Primary user |
| --- | --- | --- | --- |
| Local stdio | MCP stdio | In-process Torque packages | Desktop IDE/agent |
| Local HTTP | MCP Streamable HTTP at `/mcp` | In-process Torque packages | Browser or multi-agent host |
| Remote bridge | stdio or HTTP | `torque-agent` gRPC | Agents that operate against a remote builder/cluster |
| Systemd daemon | HTTP MCP + local gRPC | `torque-agent -mode durable` | Linux host serving durable agent workflows |

### Local stdio

MVP. The MCP client launches `torque-mcp` as a child process. The server writes only JSON-RPC messages to stdout and logs only to stderr, matching MCP stdio requirements.

### Streamable HTTP

Phase 2. Bind to `127.0.0.1` by default. Non-localhost binds require `--allow-remote-bind` and `--auth-token` or mTLS. The server validates `Origin` for browser-originating requests and supports the `MCP-Protocol-Version` header.

HTTP MCP can require bearer auth with `--auth-token` or `TORQUE_MCP_TOKEN`.
Clients send either `authorization: Bearer <token>` or `x-torque-token:
<token>`.

### Remote bridge

Phase 2. `torque-mcp` connects to `torque-agent` using the same gRPC/TLS/token settings already documented in [grpc-agent.md](grpc-agent.md). Remote bridge mode must not reimplement Build, Deploy, Log, Verify, Stack, or Mirror business logic. It maps MCP tool calls to the existing gRPC services:

- `LogService.StreamLogs`
- `BuildService.RunBuild`
- `DeployService.Apply`
- `DeployService.Destroy`
- `StackService.Plan`
- `StackService.Apply`
- `StackService.Delete`
- `StackService.Status`
- `VerifyService.Verify`
- `MirrorService.ListSessions/GetSession/Subscribe/Export`
- `AgentInfoService.GetInfo`

### Systemd daemon

The release installer provides the durable Linux deployment path:

```bash
curl -fsSL https://ingresslabs.github.io/torque/install.sh | sh -s -- --mode systemd-daemon
. /etc/torque/agent.env
curl -fsS -H "authorization: Bearer $TORQUE_MCP_TOKEN" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  http://127.0.0.1:7331/mcp
```

This installs `torque`, `torque-agent`, and `torque-mcp` to `/usr/bin`, writes
`/etc/torque/agent.env` with generated tokens when not already present, and
installs `torque-agent.service` plus `torque-mcp.service`. The agent service
uses `torque-agent -mode=durable`, persists mirror frames under
`/var/lib/torque/agent/`, and requires sandbox execution for remote builds. The
MCP service listens on `127.0.0.1:7331` by default and bridges to the local gRPC
agent at `127.0.0.1:7443`.

## Capability Advertisement

The server advertises these MCP capabilities:

```json
{
  "tools": { "listChanged": false },
  "resources": { "subscribe": true, "listChanged": true },
  "prompts": { "listChanged": false },
  "logging": {}
}
```

Tasks are optional until the implementation has MCP task interoperability tests:

```json
{
  "tasks": {
    "list": {},
    "cancel": {},
    "requests": {
      "tools": {
        "call": {}
      }
    }
  }
}
```

Long-running tools must work without tasks by returning a Torque session resource and by sending MCP progress notifications when the caller provides `_meta.progressToken`.

## Server Configuration

Configuration is read from these locations, later entries overriding earlier entries:

1. Built-in safe defaults.
2. Global Torque config already used by the CLI.
3. `~/.torque/mcp.yaml`.
4. Repo-local `.torque/mcp.yaml` when the selected MCP root contains it.
5. Environment variables.
6. CLI flags.

Example:

```yaml
mode: local
enabledTools:
  readOnly: true
  build: true
  write: false
workspace:
  requireRoots: true
  allowOutsideRoots:
    - ~/.kube
    - ~/.docker
kubernetes:
  allowedContexts:
    - dev
    - staging
  deniedNamespaces:
    - kube-system
    - kube-public
  maxLogTailLines: 2000
delivery:
  requirePlanForApply: true
  maxPlanAge: 30m
  requireVerifiedForApply: true
  requireCaptureForWrites: true
  defaultCaptureDir: .torque/evidence
remote:
  agent: ""
  tls: false
  tokenEnv: TORQUE_REMOTE_TOKEN
limits:
  maxConcurrentSessions: 4
  maxToolRuntime: 30m
  maxArtifactBytes: 1048576
  maxEventsReturned: 500
```

## Architecture

Add:

```text
cmd/torque-mcp/
internal/mcpserver/
  server.go
  config.go
  transport_stdio.go
  transport_http.go
  protocol.go
  roots.go
  paths.go
  safety.go
  sessions.go
  resources.go
  prompts.go
  tools/
    info.go
    build.go
    verify.go
    apply_plan.go
    apply_run.go
    delete.go
    logs.go
    stack.go
    capture.go
    secrets.go
    env.go
  remote/
    client.go
    sessions.go
```

The MCP layer should be thin:

- Tool handlers validate MCP input, enforce MCP-specific policy, and call existing Torque packages.
- Business logic remains in existing packages such as `internal/workflows/buildsvc`, `internal/deploy`, `internal/deployplan`, `internal/verify`, `internal/tailer`, `internal/stack`, `internal/capture`, `internal/agent`, and `internal/api/convert`.
- Remote mode uses `pkg/api/torque/api/v1` clients through `internal/grpcutil`.
- Session state uses the existing capture/mirror concepts rather than introducing a new run model.

## Roots And Path Policy

MCP roots define the workspace boundary. `torque-mcp` must request roots from clients that support them and must validate all file inputs against either:

- an MCP root,
- the current working directory when roots are not supported and `requireRoots=false`, or
- a configured allowlist entry.

Path rules:

- Accept `file://` URIs and relative paths.
- Normalize symlinks before policy checks.
- Reject path traversal outside roots.
- Reject kubeconfig, Docker auth, cosign keys, secret configs, and values files outside roots unless explicitly allowlisted.
- Never return file contents for allowlisted secret-bearing files unless the specific tool is designed to return a redacted summary.
- Store path decisions in the session audit record.

## Common Tool Contract

Tool names use the dotted `torque.*` namespace, for example `torque.apply.plan`.
Names are case-sensitive and stable once released. All `inputSchema` and
`outputSchema` values use JSON Schema 2020-12 unless an explicit `$schema`
declares another supported dialect.

Every tool returns:

- `content`: a short text summary for humans and less capable clients.
- `structuredContent`: machine-readable output conforming to `outputSchema`.
- resource links for sessions, plans, reports, captures, and large artifacts.

Every tool accepts these optional common fields unless noted otherwise:

```json
{
  "workspace": {
    "rootUri": "file:///repo",
    "cwd": "."
  },
  "kube": {
    "context": "dev",
    "kubeconfig": "~/.kube/config",
    "namespace": "default"
  },
  "session": {
    "id": "optional-client-correlation-id",
    "requester": "agent-name"
  },
  "evidence": {
    "capture": "auto",
    "captureTags": {
      "pr": "123"
    }
  },
  "safety": {
    "dryRun": true,
    "confirm": false,
    "nonInteractive": true
  },
  "wait": {
    "mode": "sync",
    "timeoutSeconds": 300
  }
}
```

`wait.mode` values:

- `sync`: run to completion within `timeoutSeconds`.
- `detach`: start the session and return immediately.
- `stream`: keep the MCP request open and emit progress/log notifications when transport supports it.

If an operation cannot finish within the request timeout, return `isError=false` with `state=running` and resource links. Do not convert an in-progress session into a tool error.

## Tool Annotations

Set MCP `annotations` for every tool. Treat them as hints, not policy.

| Tool family | readOnlyHint | destructiveHint | idempotentHint | openWorldHint |
| --- | --- | --- | --- | --- |
| Info, capture summary, plan read | true | false | true | false |
| Cache inspect and cache plan | true | false | true | false |
| Kubernetes read, logs, namespace verify | true | false | false | true |
| Build without `push` or `load` | false | false | false | true |
| Cache warm | false | false | false | true |
| Build with `push`, signing, or `load` | false | false | false | true |
| Apply | false | false | false | true |
| Delete, revert, stack delete | false | true | false | true |

Mutating tools must not rely on annotations for safety. Server-side policy must enforce confirmations and scopes.

## Tool Catalog

### `torque.info`

Read-only. Returns local `torque-mcp`, `torque`, and optional `torque-agent` version/capability information.

Primary uses:

- Sanity-check agent setup.
- Detect remote bridge mode.
- Detect whether write tools are enabled.

Output fields:

- `serverVersion`
- `torqueVersion`
- `mode`
- `remoteAgent`
- `enabledTools`
- `allowedContexts`
- `allowedNamespaces`

### `torque.apply.plan`

Read-only. Maps to `torque apply plan`.

Inputs:

```json
{
  "chart": "./chart",
  "release": "api",
  "namespace": "prod",
  "version": "",
  "valuesFiles": ["values/prod.yaml"],
  "set": ["image.tag=abc123"],
  "setString": [],
  "setFile": [],
  "includeCRDs": false,
  "format": "json",
  "compareTo": "",
  "verifyReports": ["verify.json"],
  "buildCaptures": ["build.sqlite"],
  "workspace": {},
  "kube": {},
  "evidence": {}
}
```

Output fields:

- `planId`
- `renderedSha256`
- `release`
- `namespace`
- `summary`
- `changes`
- `warnings`
- `verify`
- `buildProvenance`
- `resources`

Required resource links:

- `torque://plans/{planId}`
- `torque://plans/{planId}/markdown`
- `torque://plans/{planId}/rendered-manifest`

The plan resource is the required input for `torque.apply.run` unless server policy disables `requirePlanForApply`.

### `torque.verify.chart`

Read-only. Maps to verifier chart mode and remote `VerifyService.Verify`.

Inputs:

- chart, release, namespace, version, values, set values.
- `mode`: `warn`, `block`, or `off`.
- `failOn`: `info`, `low`, `medium`, `high`, or `critical`.
- `format`: `json` by default for agents.
- optional policy reference and baseline.

Output fields:

- `reportId`
- `passed`
- `blocked`
- `summary`
- `findings`
- `renderedSha256`
- `resources`

### `torque.verify.namespace`

Read-only but open-world because it talks to Kubernetes. Maps to verifier namespace mode.

Inputs:

- namespace, kube context, mode, failOn, policy.

Output is the same shape as `torque.verify.chart` without `renderedSha256`.

### `torque.build.run`

Runs a BuildKit/Compose build.

Default behavior:

- `push=false`
- `load=false`
- `capture=auto` when captures are enabled
- `secrets=warn`
- `sandbox` follows the active Torque profile

Inputs mirror `BuildOptions` in `agent.proto` plus Torque-specific guardrails:

```json
{
  "contextDir": ".",
  "dockerfile": "Dockerfile",
  "tags": ["ghcr.io/acme/api:dev"],
  "platforms": ["linux/amd64"],
  "buildArgs": [],
  "secrets": [],
  "cacheFrom": [],
  "cacheTo": [],
  "s3Cache": "s3://acme-build-cache/torque/main",
  "s3CacheRegion": "us-east-1",
  "s3CacheName": "api-main",
  "s3CacheMode": "max",
  "s3CacheEndpointUrl": "",
  "s3CachePathStyle": false,
  "push": false,
  "load": false,
  "sign": false,
  "noCache": false,
  "mode": "auto",
  "composeFiles": [],
  "composeProfiles": [],
  "composeServices": [],
  "sandbox": true,
  "sbom": false,
  "provenance": false,
  "policy": "",
  "policyMode": "enforce",
  "secretsMode": "warn",
  "workspace": {},
  "session": {},
  "evidence": {},
  "wait": {}
}
```

Safety:

- If `push=true`, `sign=true`, or `load=true`, require `enabledTools.build=true` and `safety.confirm=true`.
- Reject interactive build mode under MCP.
- Reject secrets passed as literal values. Only secret names or provider references are allowed.
- Return secret scan findings and policy findings in structured output.

Output fields:

- `sessionId`
- `state`
- `tags`
- `digest`
- `ociOutputDir`
- `capture`
- `policy`
- `secrets`
- `resources`

### `torque.cache.inspect`

Read-only. Returns cache state as structured MCP data so agents do not scrape
BuildKit logs.

Inputs mirror the build cache subset accepted by `torque.build.run`:

```json
{
  "contextDir": ".",
  "dockerfile": "Dockerfile",
  "tags": ["ghcr.io/acme/api:dev"],
  "platforms": ["linux/amd64"],
  "cacheFrom": [],
  "cacheTo": [],
  "s3Cache": "s3://acme-build-cache/torque/main",
  "s3CacheRegion": "us-east-1",
  "s3CacheName": "api-main",
  "s3CacheMode": "max",
  "s3CacheEndpointUrl": "",
  "s3CachePathStyle": false,
  "cacheDir": "",
  "mode": "auto",
  "composeFiles": [],
  "composeServices": []
}
```

Output fields:

- `state`: `cacheless`, `configured`, `s3-enabled`, or `disabled`.
- `isolationKey`: short stable key for the context, Dockerfile, tags,
  platform set, and S3 manifest.
- `s3`: normalized S3 cache ref, region, manifest name, mode, endpoint, and
  credential boundary. Credentials stay at the BuildKit daemon or workload
  identity layer and are never MCP arguments.
- `cacheFrom` / `cacheTo`: normalized BuildKit import/export strings after
  first-class S3 fields are expanded.
- `imports` / `exports`: parsed cache spec summaries with sensitive attributes
  redacted.
- `cacheDir.cacheIntel`: optional local Torque cache-intel artifact counts when
  `cacheDir` is provided.
- `warnings` and `recommendations`.

### `torque.cache.plan`

Read-only. Builds on `torque.cache.inspect` and adds agent-actionable cache
planning.

Additional inputs:

- `changedPaths`: paths from `git diff`, CI change detection, or an agent's
  workspace analysis.
- `baseImages`: optional known base image refs for context.
- `buildArgs` and `secrets`: IDs/references only; secret-like literal values
  produce warnings.

Output fields:

- `inspection`: the same normalized cache state as `torque.cache.inspect`.
- `plan.strategy`: `none`, `disabled`, `s3-shared`, `export-configured`, or
  `import-only`.
- `plan.changedPaths`: per-path cache impact classes such as
  `dockerfile`, `dependency-layer`, `source-layer`, `deploy-input`, and
  `ci-input`.
- `plan.warmTargets`: the exact context, Dockerfile, platforms, imports, and
  exports an agent should pass to `torque.cache.warm`.
- `plan.agentWorkflow`: the recommended inspect -> plan -> warm sequence.
- `plan.remoteServices`: remote gRPC services expected during warm execution.

### `torque.cache.warm`

Mutating. Runs a remote build through `BuildService.RunBuild` with push, load,
and signing disabled, but with configured cache exports enabled. This is the
agent-facing warm path for S3 or explicit `cacheTo` targets.

Safety:

- Requires `torque-mcp --enable-write`.
- Requires `safety.confirm=true`.
- Requires a configured remote `torque-agent`.
- Requires at least one export target from `cacheTo` or `s3Cache`.
- Does not accept AWS credentials as MCP inputs.

Output fields:

- `inspection`: normalized cache state used for the warm.
- `warm.sessionId`
- `warm.state`
- `warm.result.digest`
- `warm.logs`
- resource link for the warm session.

### `torque.ship.run`

Mutating. Runs the `torque ship` workflow through a child Torque process. In remote bridge mode the MCP server passes `--remote-agent` to the child Torque process and supplies `TORQUE_REMOTE_TOKEN` through the process environment, not argv, so tokens do not appear in process args, MCP responses, or session events.

Inputs mirror `torque ship`:

- chart, release, namespace, values, set values.
- build context, Dockerfile, tags, platforms, cache, push/load flags.
- optional first-class S3 cache fields (`s3Cache`, `s3CacheRegion`, `s3CacheName`, `s3CacheMode`, `s3CacheEndpointUrl`, `s3CachePathStyle`) forwarded only to the build step.
- verify mode/fail threshold.
- evidence directory and capture tags.
- apply flags: namespace creation, wait, atomic, timeout, plan-only.

Safety:

- Disabled unless `enabledTools.write=true` for non-plan-only runs.
- Require `safety.confirm=true` or `yes=true`.
- Redact remote tokens and sensitive argument values from returned stdout/stderr, structured `args`, and session events.
- `planOnly=true` or `safety.dryRun=true` forces `torque ship --plan-only`.

Output fields:

- `sessionId`
- `exitCode`
- `stdout`
- `stderr`
- `args` with sensitive values redacted
- `summary` parsed from `<evidenceDir>/ship.json` when present

### `torque.apply.run`

Mutating. Maps to `torque apply` or remote `DeployService.Apply`.

Inputs:

```json
{
  "planId": "plan_...",
  "renderedSha256": "sha256...",
  "chart": "./chart",
  "release": "api",
  "namespace": "prod",
  "valuesFiles": ["values/prod.yaml"],
  "set": ["image.tag=abc123"],
  "helmWait": true,
  "atomic": true,
  "createNamespace": false,
  "timeoutSeconds": 300,
  "watchSeconds": 0,
  "driftGuard": true,
  "requireVerifiedReportId": "verify_...",
  "workspace": {},
  "kube": {},
  "safety": {
    "confirm": true,
    "nonInteractive": true
  },
  "evidence": {
    "capture": "auto"
  }
}
```

Safety:

- Disabled unless `enabledTools.write=true`.
- Require `safety.confirm=true`.
- Require a fresh `planId` created by this MCP server when `requirePlanForApply=true`.
- Require matching `renderedSha256`.
- Require a passing verify report when `requireVerifiedForApply=true`.
- Require namespace/context allowlist match.
- Require capture unless `requireCaptureForWrites=false`.
- If the plan includes deletes/replacements above configured thresholds, return a tool execution error with a summary and do not apply.

Output fields:

- `sessionId`
- `state`
- `release`
- `namespace`
- `revision`
- `status`
- `capture`
- `resources`

### `torque.delete.run`

Destructive. Maps to `torque delete` or remote `DeployService.Destroy`.

Safety:

- Disabled unless `enabledTools.write=true`.
- Require `safety.confirm=true`.
- Require release and namespace.
- Require namespace/context allowlist match.
- Require capture by default.
- Default `dryRun=true` when `safety.confirm=false`, but do not silently delete.

### `torque.revert.run`

Mutating recovery operation. Maps to `torque revert`.

Safety:

- Disabled unless `enabledTools.write=true`.
- Require `safety.confirm=true`.
- Require release, namespace, and target revision or explicit `autoSelectLastKnownGood=true`.
- Return selected revision details before executing when `wait.mode=detach`.

### `torque.logs.query`

Read-only Kubernetes log/event query. Maps to `torque logs` or remote `LogService.StreamLogs`.

Inputs:

- `podQuery`
- `namespaces`
- `allNamespaces`
- `labelSelector`
- `fieldSelector`
- `containers`
- `excludeContainers`
- `excludePods`
- `highlightTerms`
- `includeEvents`
- `eventsOnly`
- `tailLines`
- `follow`
- `timestamps`
- `filter`
- `deployMode`
- `deps`
- stack config for dependency expansion.

Safety:

- Cap `tailLines` by config.
- For `follow=true`, default to `wait.mode=detach` and expose a session tail resource.
- Redact secret-like strings before returning text or capture rows.

Output fields:

- `sessionId`
- `state`
- `lines`
- `lineCount`
- `truncated`
- `resources`

### `torque.stack.plan`

Read-only. Maps to local `torque stack plan` or remote `StackService.Plan` when `torque-mcp --remote-agent` is configured.

Inputs:

- stack config/root/profile.
- selectors: clusters, tags, releases, paths, git range.
- expansion: include dependencies/dependents.
- output format `json`.
- optional `bundle` for local mode; remote bridge mode rejects bundle creation because bundle files must be produced on the agent host.

Output fields:

- `planId`
- `planHash`
- `stackName`
- `profile`
- `nodes`
- `edges`
- `runner`
- `bundle`
- `resources`

### `torque.stack.apply`

Mutating. Maps to remote `StackService.Apply`.

Safety:

- Disabled unless `enabledTools.write=true`.
- Require `safety.confirm=true`.
- `safety.dryRun=true` is accepted without write confirmation for previews.
- Config-based remote runs are supported in the first implementation; sealed bundle/resume flows remain CLI-on-agent workflows.
- Honor locks, concurrency limits, and namespace allowlists.

### `torque.stack.delete`

Destructive. Maps to remote `StackService.Delete`.

Safety:

- Same as `torque.stack.apply`.
- Require stronger delete threshold checks.
- Return a tool execution error if selected node count exceeds `deleteConfirmThreshold` and `confirmLargeDelete` is not true.

### `torque.stack.status`

Read-only. Maps to local `torque stack status --format json` or remote `StackService.Status`.

Outputs current run state, failed nodes, retryable nodes, lock owner, and recent events.

### `torque.stack.rerun_failed`

Mutating. Maps to `torque stack rerun-failed`.

Safety:

- Disabled unless `enabledTools.write=true`.
- Require `safety.confirm=true`.
- Require prior run ID.
- Only schedules nodes whose stored status is failed unless `includeBlocked=true`.

### `torque.capture.summarize`

Read-only. Maps to `internal/capture.Summarize`.

Inputs:

- capture path or session resource URI.
- optional session ID.
- optional event/artifact limits.

Output fields:

- `sessions`
- `eventKinds`
- `artifacts`
- `droppedEvents`
- `suspectedFailure`
- `resources`

### `torque.capture.read_artifact`

Read-only. Reads a named capture artifact with size limits.

Safety:

- Only reads artifacts from captures produced or allowlisted by this MCP server.
- Applies redaction before returning text.
- Large artifacts return a resource link rather than inline content.

### `torque.session.list`

Read-only. Lists live, completed, or persisted MCP/Torque sessions.

Backends:

- local MCP session store,
- local capture DB index,
- remote `MirrorService.ListSessions`.

### `torque.session.get`

Read-only. Returns session metadata, status, latest events, and artifact links.

### `torque.session.tail`

Read-only. Tails a session. In stdio mode this returns a bounded batch and a cursor. In HTTP mode it can use resource subscriptions or task/progress notifications.

### `torque.session.cancel`

Mutating but scoped to a Torque session. Cancels a running MCP-initiated build, log stream, apply, delete, or stack run when cancellation is supported.

Safety:

- Require session ownership or admin policy.
- Return best-effort cancellation state.

### `torque.secret.discover`

Read-only. Maps to `torque secrets discover`.

Output includes providers, references, orphaned references, and invalid references. It never returns secret values.

### `torque.env.inspect`

Read-only. Maps to `torque env`.

Output includes relevant Torque env/config settings with sensitive values redacted.

## Resources

Resource URIs are stable for the lifetime of the server. Persisted captures and remote mirror sessions may outlive the server process.

| URI | MIME type | Description |
| --- | --- | --- |
| `torque://info` | `application/json` | Server, Torque, and remote agent metadata |
| `torque://sessions` | `application/json` | Session list |
| `torque://sessions/{sessionId}` | `application/json` | Session metadata and status |
| `torque://sessions/{sessionId}/events?cursor=N&limit=M` | `application/jsonl` | Session event stream page |
| `torque://sessions/{sessionId}/tail?cursor=N&limit=M` | `text/plain` | Human-readable event/log tail |
| `torque://sessions/{sessionId}/artifacts` | `application/json` | Artifact index |
| `torque://sessions/{sessionId}/artifacts/{name}` | varies | Redacted artifact content |
| `torque://plans/{planId}` | `application/json` | Deploy or stack plan |
| `torque://plans/{planId}/markdown` | `text/markdown` | Review-ready plan summary |
| `torque://plans/{planId}/rendered-manifest` | `text/yaml` | Redacted rendered manifest |
| `torque://verify/{reportId}` | `application/json` | Verifier report |
| `torque://captures/{captureId}/summary` | `application/json` | Capture DB summary |
| `torque://docs/architecture` | `text/markdown` | Embedded [architecture.md](architecture.md) |
| `torque://docs/grpc-agent` | `text/markdown` | Embedded [grpc-agent.md](grpc-agent.md) |

Subscriptions:

- Clients may subscribe to `torque://sessions/{sessionId}`.
- The server emits `notifications/resources/updated` when session status, events, or artifacts change.
- Event resources use cursor-based reads to avoid unbounded responses.

## Prompts

Prompts are user-invoked templates that guide agents toward safe multi-step use of the tools.

### `torque.release_review`

Purpose: produce a PR-ready release review without changing the cluster.

Arguments:

- chart
- release
- namespace
- values files
- image tags
- optional build capture

Expected tool sequence:

1. `torque.verify.chart`
2. `torque.apply.plan`
3. `torque.capture.summarize` when captures are supplied
4. Summarize blockers, risky changes, image provenance, and attachable resources.

### `torque.safe_apply`

Purpose: run the full delivery gate with explicit confirmation.

Expected tool sequence:

1. `torque.build.run` when a build context is provided.
2. `torque.verify.chart`.
3. `torque.apply.plan`.
4. Ask the user to confirm exact release, namespace, context, and rendered digest.
5. `torque.apply.run`.
6. `torque.session.get` and `torque.capture.summarize`.

The prompt must tell the model to stop before step 5 unless the user has confirmed the exact digest and target.

### `torque.incident_diagnose`

Purpose: diagnose an unhealthy workload.

Expected tool sequence:

1. `torque.logs.query` with events.
2. `torque.stack.status` when stack config exists.
3. `torque.capture.summarize` when an apply/log capture exists.
4. Summarize suspected cause, supporting event/log lines, and next action.

### `torque.stack_change_review`

Purpose: review a multi-release stack change.

Expected tool sequence:

1. `torque.stack.plan` with `gitRange`.
2. Optional `torque.stack.status`.
3. Optional verifier calls for affected releases.
4. Summarize DAG order, affected releases, risk, and blocked nodes.

### `torque.evidence_summary`

Purpose: turn one or more capture DBs or mirror sessions into a concise incident/release note.

Expected tool sequence:

1. `torque.capture.summarize` or `torque.session.get`.
2. `torque.capture.read_artifact` for relevant artifacts.
3. Summarize timeline, result, failed phase, attached artifacts, and missing evidence.

## Session Model

Every long-running operation gets a `sessionId`.

States:

- `queued`
- `running`
- `succeeded`
- `failed`
- `cancelled`
- `expired`

Common session fields:

```json
{
  "sessionId": "mcp_01HX...",
  "kind": "build|apply|delete|logs|verify|stack",
  "state": "running",
  "createdAt": "2026-05-06T12:00:00Z",
  "updatedAt": "2026-05-06T12:01:00Z",
  "requester": "codex",
  "workspaceRoot": "file:///repo",
  "kubeContext": "dev",
  "namespace": "prod",
  "release": "api",
  "planId": "plan_...",
  "capture": {
    "path": ".torque/evidence/apply.sqlite",
    "sessionId": "capture-session-id"
  },
  "cursors": {
    "events": 42
  }
}
```

Event schema:

```json
{
  "sequence": 42,
  "time": "2026-05-06T12:01:02Z",
  "level": "info",
  "kind": "deploy.phase|build.log|verify.finding|stack.event|log.line",
  "message": "waiting for deployment/api",
  "namespace": "prod",
  "pod": "",
  "container": "",
  "structured": {}
}
```

Implementation notes:

- Local sessions are stored in memory plus optional SQLite under `.torque/mcp/sessions.sqlite`.
- Remote sessions use `MirrorService` where possible.
- Captures remain the durable evidence format for completed runs.
- MCP session IDs and Torque capture session IDs are distinct but linked.

## Progress And Cancellation

When a request includes `_meta.progressToken`, the server should emit `notifications/progress`.

Progress mapping:

- Build: monotonic step count when available, otherwise event count.
- Verify: phases `collect`, `evaluate`, `done`.
- Apply/delete: Helm/deploy phases and resource readiness counts.
- Stack: completed nodes out of total nodes.
- Logs follow: line count with no total.

Cancellation:

- Honor MCP `notifications/cancelled` for in-flight non-task requests.
- Implement `torque.session.cancel` for detached sessions.
- When MCP tasks are enabled, implement `tasks/cancel` and map it to the same cancellation registry.

## Error Model

Use JSON-RPC protocol errors only for protocol-level problems:

- unknown tool,
- malformed request,
- invalid JSON schema,
- unsupported MCP version,
- internal server failure before tool execution starts.

Use MCP tool execution errors (`isError=true`) for domain errors the model can correct:

- missing chart/release/namespace,
- path outside roots,
- disallowed kube context,
- verifier blocked,
- plan digest mismatch,
- confirmation missing,
- namespace denied,
- apply threshold exceeded,
- BuildKit unavailable,
- remote agent unavailable.

Structured tool errors:

```json
{
  "code": "PLAN_DIGEST_MISMATCH",
  "message": "renderedSha256 does not match the supplied plan",
  "retryable": false,
  "hints": [
    "Call torque.apply.plan again and pass the returned planId to torque.apply.run."
  ],
  "details": {}
}
```

## Security Model

### Default posture

- Read-only tools enabled by default.
- Build tools enabled by default only when `push=false`, `load=false`, and `sign=false`.
- Write tools disabled by default.
- Remote HTTP disabled by default.
- Captures enabled for writes by default.
- Secret redaction always enabled.

### Mutating operation requirements

Mutating tools require all of:

1. Server config enables the tool family.
2. Target context/namespace passes allowlist/denylist policy.
3. Request contains `safety.confirm=true`.
4. Request sets `safety.nonInteractive=true`.
5. Tool-specific proof is supplied, such as `planId`, `renderedSha256`, `planHash`, or signed bundle.
6. Capture is enabled when policy requires it.

### Plan-to-apply invariant

`torque.apply.run` must not render an unrelated manifest after planning. It must either:

- use the exact stored plan inputs and verify the new render has the same `renderedSha256`, or
- fail with `PLAN_DIGEST_MISMATCH`.

This is the core safety guarantee for agents: the model can explain and ask approval for one digest, then apply exactly that digest.

### Secrets

- Do not accept raw secret values in MCP form fields.
- Do not use MCP form elicitation for passwords, API keys, tokens, or payment credentials.
- Only accept secret references, environment variable names, or preconfigured provider names.
- Redact secret-like values in logs, artifacts, errors, and summaries.
- Return secret audit entries as references only.

### Kubernetes and registry safety

- Do not bypass Kubernetes RBAC.
- Do not default to `allNamespaces`.
- Do not allow writes to denied namespaces even if kubeconfig permits them.
- For build push/sign, require explicit registry allowlist when configured.
- For cosign keys and auth files, enforce path policy.

### Audit

Each tool call records an audit event with:

- tool name,
- requester,
- workspace root,
- inputs with secrets redacted,
- target context/namespace/release,
- safety decisions,
- session ID,
- output resource IDs,
- error code when failed.

Audit logs go to stderr for stdio, the HTTP access log for HTTP mode, and optional SQLite for durable review.

## Implementation Plan

### Phase 1: Local stdio, read-mostly

Deliver:

- `torque-mcp --stdio`
- MCP initialize/list tools/call tools/list resources/read prompts/list prompts/get
- roots-based path policy
- tools:
  - `torque.info`
  - `torque.apply.plan`
  - `torque.verify.chart`
  - `torque.verify.namespace`
  - `torque.logs.query` with `follow=false`
  - `torque.stack.plan`
  - `torque.stack.status`
  - `torque.capture.summarize`
  - `torque.capture.read_artifact`
  - `torque.secret.discover`
  - `torque.env.inspect`
- resources:
  - `torque://info`
  - plan resources
  - verify report resources
  - capture summary/artifact resources
  - docs resources
- prompts:
  - `torque.release_review`
  - `torque.incident_diagnose`
  - `torque.stack_change_review`
  - `torque.evidence_summary`

Tests:

- MCP JSON-RPC handshake.
- Tool listing schemas are valid JSON Schema 2020-12.
- Read-only tool calls return `structuredContent`.
- Path policy rejects files outside roots.
- Secret redaction tests.
- Golden prompt tests.

### Phase 2: Sessions, build, safe writes

Deliver:

- session registry and `torque://sessions/*` resources
- resource subscriptions
- progress notifications
- tools:
  - `torque.build.run`
  - `torque.apply.run`
  - `torque.delete.run`
  - `torque.revert.run`
  - `torque.stack.apply`
  - `torque.stack.delete`
  - `torque.stack.rerun_failed`
  - `torque.session.list`
  - `torque.session.get`
  - `torque.session.tail`
  - `torque.session.cancel`
- automatic capture for writes
- plan-to-apply digest enforcement
- write policy config

Tests:

- `apply.run` fails without `confirm=true`.
- `apply.run` fails without a fresh matching `planId`.
- `apply.run` fails on digest mismatch.
- denied namespace cannot be written.
- build push requires confirmation.
- detached sessions can be tailed and cancelled.
- captures are linked from write outputs.

### Phase 3: Remote bridge and HTTP

Deliver:

- remote bridge to `torque-agent`
- Streamable HTTP transport at `/mcp`
- auth token and optional TLS/mTLS
- origin validation
- remote mirror session resources
- remote session export

Tests:

- fake `torque-agent` gRPC service mapping.
- token missing/invalid rejected.
- HTTP origin rejection.
- remote mirror session tail resources.

### Phase 4: MCP tasks and agent evaluations

Deliver:

- optional MCP task support for long-running tool calls.
- `tasks/list`, `tasks/cancel`, and task result retrieval.
- task metadata linked to Torque sessions.
- evaluation fixtures for common agent workflows:
  - release review,
  - safe apply refusal without confirmation,
  - plan digest mismatch,
  - namespace denied,
  - incident diagnosis from capture,
  - stack affected-release review.

## Enterprise Operating Baseline

Enterprise MCP deployments should default to remote bridge mode with
`torque-agent` as the execution boundary and MCP as the typed agent contract.
The minimum production posture is:

- `torque-agent` runs with TLS, `-tls-client-ca`, and `-token`.
- `torque-mcp` connects with `--remote-tls`, CA pinning, client cert/key, and a
  remote token sourced from environment or secret manager.
- MCP write tools require both `--enable-write` at server startup and
  `safety.confirm=true` per request.
- Kubernetes writes are constrained by scoped kubeconfigs/RBAC plus MCP
  context/namespace allowlists.
- Mutating tools must create or link evidence: capture DB, verifier report,
  plan digest, ship summary, stack run ID, or MirrorService session.
- S3 cache credentials stay at the BuildKit daemon/IAM role layer and never in
  MCP tool inputs or responses.
- Remote tokens, registry credentials, kubeconfig paths, secret references, and
  BuildKit secrets are redacted in tool output, session tails, and errors.

See [enterprise-agent-operations.md](enterprise-agent-operations.md) for the
operator-facing baseline and mTLS-first examples.

## Scenario Catalog

The executable scenario catalog lives at [dag-build-scenarios.yaml](../testdata/mcp/dag-build-scenarios.yaml). It defines sixteen remote MCP, gRPC, ship, build, stack, and safety workflows with explicit DAG topologies, build inputs, remote services, redaction expectations, and validation signals.

The catalog is regression-tested by `TestMCPDAGBuildScenarioCatalog`, which verifies scenario count, unique IDs, acyclic graph structure, valid edge endpoints, required MCP call coverage, required remote gRPC coverage, write confirmation coverage, and remote token redaction coverage.

A 500-600 word narrative version for Medium-style publication lives at [remote-torque-mcp-blog.md](remote-torque-mcp-blog.md).

Published scenario matrix:

| Scenario | Shape | Main proof |
| --- | --- | --- |
| `remote-ship-core-fanout` | fanout | MCP build and ship through remote Build/Deploy/Stack services |
| `compose-diamond-promotion` | diamond | Compose build fan-in with verifier and mirror frames |
| `stack-progressive-large-dag` | layered | remote StackService plan/apply/status over a larger graph |
| `multi-namespace-delete-threshold` | multi-root | delete confirmation and threshold policy |
| `secret-provider-build-and-ship` | chain | secret references stay redacted through build and ship |
| `mcp-http-origin-remote-stack` | ingress | HTTP MCP origin and token checks |
| `mirror-session-replay-build-to-apply` | event-bus | replayable Build/Deploy/Stack sessions |
| `microvm-mcp-smoke-before-deploy` | preflight | static MCP binary boots and advertises tools in microVMs |
| `fanin-verifier-gate` | fanin | verifier gate blocks stack apply until all digests are present |
| `rollback-delete-after-failed-ship` | rollback | failed ship evidence drives selective stack delete |

## Acceptance Criteria

- An MCP client can list tools, resources, and prompts without repo-specific setup beyond launching `torque-mcp --stdio`.
- An agent can produce a review-ready apply plan using `torque.apply.plan` and resource links without parsing terminal output.
- An agent cannot apply, delete, revert, push, or stack-apply unless write tools are enabled and the request carries explicit confirmation plus required proof.
- Every mutating tool creates or links durable evidence.
- No tool returns raw secret values in normal or error output.
- All file paths accepted by tools are constrained by MCP roots or explicit allowlists.
- Long-running work is resumable through session resources.
- Remote mode reuses `torque-agent` gRPC services and MirrorService sessions.
- The spec is covered by unit tests and at least one end-to-end MCP client smoke test.

## Open Questions

- Which Go MCP SDK should be used, if any, once 2025-11-25 support is stable enough? If no SDK is suitable, implement the minimal JSON-RPC protocol directly for stdio and HTTP.
- Should `torque-mcp` be shipped as a separate release artifact immediately, or start as `torque mcp serve` and split later?
- Should write tools be installed but disabled by policy, or omitted from `tools/list` until enabled? Omission is safer for model behavior; disabled tools are easier for clients to explain.
- How long should local session resources persist by default after process exit?
- Should `torque.apply.run` require a verifier report by default only in `secure` profile, or always when invoked through MCP?
- How much rendered manifest content should be returned inline before requiring resource reads?
