# gRPC Agent API (torque-agent)

`torque-agent` exposes `torque` capabilities over gRPC for automation and AI agents:

- Logs: `LogService.StreamLogs`
- Builds: `BuildService.RunBuild`
- Deploy: `DeployService.Apply` / `DeployService.Destroy`
- Stack orchestration: `StackService.Plan` / `StackService.Apply` / `StackService.Delete` / `StackService.Status`
- Verify: `VerifyService.Verify`
- Mirror bus: `MirrorService.Publish` / `MirrorService.Subscribe`
- Agent metadata: `AgentInfoService.GetInfo`

The API definitions live in `proto/torque/api/v1/agent.proto`.

`BuildService.RunBuild` accepts the same generic BuildKit cache specs used by the CLI. For S3 cache, pass `cache_from` / `cache_to` entries such as `type=s3,bucket=acme-build-cache,region=us-east-1,prefix=torque/main/,name=api` and `type=s3,bucket=acme-build-cache,region=us-east-1,prefix=torque/main/,name=api,mode=max`. The first-class CLI/MCP `--s3-cache` / `s3Cache` fields expand to those entries before the gRPC request is sent.

## Enterprise Policy Boundary

For production-like use, treat `torque-agent` as the execution boundary and
make the policy explicit:

- run with TLS plus `-tls-client-ca` so clients must present certificates;
- require `-token` as a second factor on every RPC;
- use a kubeconfig and Kubernetes RBAC role scoped to the namespaces and verbs
  the agent is allowed to touch;
- enable `-mirror-store` so every agent-driven build, deploy, stack run, and log
  stream can be replayed or exported;
- set mirror retention (`-mirror-max-sessions`, `-mirror-max-frames`,
  `-mirror-max-bytes`) instead of leaving durable evidence unbounded;
- keep MCP write tools disabled unless `torque-mcp --enable-write` is
  intentionally configured and individual requests also send `safety.confirm`;
- pass S3 cache credentials through the BuildKit daemon or instance role, not
  through CLI/MCP/gRPC request fields.

The agent does not bypass Kubernetes RBAC or registry IAM. Use MCP
context/namespace allowlists and scoped kubeconfigs as the policy layer until a
dedicated `torque-agent` policy file is added.

## Running torque-agent

```bash
go install ./cmd/torque-agent

# Insecure gRPC by default (plaintext). Prefer SSH tunnels or a private network.
torque-agent -listen :7443 -kubeconfig ~/.kube/config -context <ctx>

# Optional auth token (required for all RPCs when set).
torque-agent -listen :7443 -token "$TORQUE_REMOTE_TOKEN" -kubeconfig ~/.kube/config -context <ctx>

# Optional MirrorService flight recorder (durable sessions + ListSessions/Export).
torque-agent -listen :7443 -mirror-store ~/.torque/agent/mirror.sqlite -kubeconfig ~/.kube/config -context <ctx>

# Optional retention knobs for the flight recorder.
torque-agent -listen :7443 -mirror-store ~/.torque/agent/mirror.sqlite \
  -mirror-max-sessions 200 -mirror-max-frames 5000 -mirror-max-bytes 1000000000

# Optional HTTP gateway for browser UIs (same auth token as gRPC).
torque-agent -listen :7443 -http-listen :8081 -mirror-store ~/.torque/agent/mirror.sqlite

# Optional TLS (and mTLS).
torque-agent -listen :7443 -tls-cert ./server.crt -tls-key ./server.key
torque-agent -listen :7443 -tls-cert ./server.crt -tls-key ./server.key -tls-client-ca ./client-ca.crt
```

## Durable Daemon Mode

`torque-agent -mode durable` is the Linux service profile. It defaults the
MirrorService flight recorder to `/var/lib/torque/agent/mirror.sqlite` when run
as root, enables the HTTP mirror gateway on `127.0.0.1:8081`, and requires
sandbox execution for remote builds unless explicit flags override the defaults.

```bash
torque-agent -mode durable \
  -listen :7443 \
  -token "$TORQUE_REMOTE_TOKEN" \
  -build-sandbox-bin nsjail
```

The release installer can install and start both systemd services:

```bash
curl -fsSL https://ingresslabs.github.io/torque/install.sh | sh -s -- --mode systemd-daemon
sudo systemctl status torque-agent.service torque-mcp.service
```

The generated `/etc/torque/agent.env` contains `TORQUE_REMOTE_TOKEN`,
`TORQUE_MCP_TOKEN`, listen addresses, mirror-store path, and sandbox defaults.
Keep it mode `0600`; copy or replace `/etc/torque/kubeconfig` when the daemon
must perform Kubernetes log, apply, delete, or stack operations.

mTLS-first remote bridge example:

```bash
export TORQUE_REMOTE_TOKEN="$(openssl rand -hex 32)"

torque-agent \
  -listen 0.0.0.0:7443 \
  -token "$TORQUE_REMOTE_TOKEN" \
  -kubeconfig /etc/torque/prod.kubeconfig \
  -context prod \
  -tls-cert /etc/torque/tls/agent.crt \
  -tls-key /etc/torque/tls/agent.key \
  -tls-client-ca /etc/torque/tls/client-ca.crt \
  -mirror-store /var/lib/torque/agent/mirror.sqlite \
  -mirror-max-sessions 500 \
  -mirror-max-frames 20000 \
  -mirror-max-bytes 5000000000
```

## Introspection (reflection)

The agent enables gRPC reflection so dynamic clients can discover the API at runtime.

If you have `grpcurl` installed:

```bash
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" 127.0.0.1:7443 list
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" 127.0.0.1:7443 list torque.api.v1
```

If the agent is running with TLS, omit `-plaintext` and pass a CA bundle instead:

```bash
grpcurl -cacert ./ca.crt -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" 127.0.0.1:7443 list
```

If the agent requires mTLS (`-tls-client-ca`), also pass a client cert/key:

```bash
grpcurl -cacert ./ca.crt -cert ./client.crt -key ./client.key \
  -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" 127.0.0.1:7443 list
```

## Health checks

```bash
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" \
  127.0.0.1:7443 grpc.health.v1.Health/Check
```

## Agent info

```bash
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" \
  127.0.0.1:7443 torque.api.v1.AgentInfoService/GetInfo
```

## Mirror Flight Recorder (sessions)

When `-mirror-store` is set, `torque-agent` persists `MirrorService` frames to SQLite and exposes session metadata:

```bash
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" \
  127.0.0.1:7443 torque.api.v1.MirrorService/ListSessions
```

List sessions also supports query filters (meta/tags/state/last-seen window), for example:

```bash
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" \
  -d '{"limit":200,"meta":{"namespace":"prod","release":"checkout"},"state":"MIRROR_SESSION_STATE_RUNNING"}' \
  127.0.0.1:7443 torque.api.v1.MirrorService/ListSessions
```

Get a single session (metadata + latest cursor):

```bash
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" \
  -d '{"session_id":"<session-id>"}' \
  127.0.0.1:7443 torque.api.v1.MirrorService/GetSession
```

Set session metadata/tags (useful for IDEs/UIs):

```bash
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" \
  -d '{"session_id":"<session-id>","meta":{"command":"torque logs","args":["checkout-.*","--namespace","prod"],"requester":"me@host"},"tags":{"team":"infra"}}' \
  127.0.0.1:7443 torque.api.v1.MirrorService/SetSessionMeta
```

Set session lifecycle status (optional; `torque-agent` also sets this automatically for built-in streaming RPCs when `session_id` is provided):

```bash
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" \
  -d '{"session_id":"<session-id>","status":{"state":"MIRROR_SESSION_STATE_DONE","exit_code":0,"completed_unix_nano":123}}' \
  127.0.0.1:7443 torque.api.v1.MirrorService/SetSessionStatus
```

Delete a session:

```bash
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" \
  -d '{"session_id":"<session-id>"}' \
  127.0.0.1:7443 torque.api.v1.MirrorService/DeleteSession
```

You can export a session as JSONL (one `MirrorFrame` per line, with `sequence` and `received_unix_nano` set):

```bash
grpcurl -plaintext -H "authorization: Bearer $TORQUE_REMOTE_TOKEN" \
  -d '{"session_id":"<session-id>","format":"jsonl"}' \
  127.0.0.1:7443 torque.api.v1.MirrorService/Export
```

## HTTP Gateway (Browser UIs)

When `-http-listen` is set, `torque-agent` exposes a tiny HTTP API that mirrors the MirrorService session surface:

- `POST /api/v1/auth/cookie` (sets an HttpOnly `torque_token` cookie; useful for native browser `EventSource`)
- `DELETE /api/v1/auth/cookie` (clears the cookie)
- `GET /api/v1/mirror/sessions?limit=200`
- `GET /api/v1/mirror/sessions?limit=200&namespace=prod&release=checkout&state=running`
- `GET /api/v1/mirror/sessions/<session-id>`
- `GET /api/v1/mirror/sessions/<session-id>/export?from_sequence=1` (JSONL)
- `GET /api/v1/mirror/sessions/<session-id>/tail?from_sequence=1&replay=1` (SSE: `event: frame` per `MirrorFrame`)
  - Resume: send `Last-Event-ID: <sequence>` (or `?last_event_id=<sequence>`)
  - Tuning: `?heartbeat=15s` (or `heartbeat_ms=15000`), `?retry_ms=1000`
  - Backpressure: if frames cannot be replayed (retention, slow consumer, etc.), the stream emits `event: dropped` with a JSON payload describing the missing sequence range.

Authentication uses the same headers as gRPC (`authorization: Bearer ...` or `x-torque-token: ...`), or the `torque_token` cookie set by `POST /api/v1/auth/cookie`.

## Session IDs

For agent/IDE integrations, treat `session_id` as the cross-RPC correlation key:

- Send `session_id` on `BuildService.RunBuild`, `LogService.StreamLogs`, `DeployService.Apply`/`Destroy`, `StackService.Apply`/`Delete`, and `VerifyService.Verify` to have the agent mirror those streams into `MirrorService` (so multiple subscribers can replay/tail the same session).
- `MirrorService.Publish` also records inbound frames with the same `session_id` and a server-assigned `sequence`.

## Client auth header

When `torque-agent -token ...` is set, clients must send one of:

- `authorization: Bearer <token>`
- `x-torque-token: <token>`

## torque Client TLS Flags

When the agent runs with TLS (`-tls-cert/-tls-key`), `torque` can be pointed at it with:

```bash
torque --remote-agent <host:port> --remote-tls --remote-tls-ca ./ca.crt --remote-token "$TORQUE_REMOTE_TOKEN" logs ...
torque --remote-agent <host:port> --remote-tls --remote-tls-insecure-skip-verify logs ...
torque --remote-agent <host:port> --remote-tls --remote-tls-server-name <name> logs ...
torque --remote-agent <host:port> --remote-tls --remote-tls-ca ./ca.crt \
  --remote-tls-client-cert ./client.crt --remote-tls-client-key ./client.key \
  --remote-token "$TORQUE_REMOTE_TOKEN" logs ...
```
