# Architecture Diagrams

Text-only companion notes for the landing page architecture diagrams. The
diagrams live on the landing page; docs stay copy/paste friendly and free of
bitmap assets.

<details open>
<summary>End-to-end delivery loop</summary>

```bash
torque build . --tag ghcr.io/acme/checkout:dev --capture ./build.sqlite
verifier --chart ./chart --release checkout -n prod --format json --report verify.json
torque apply plan --chart ./chart --release checkout -n prod \
  --verify-report verify.json --build-capture ./build.sqlite --output plan.md
torque apply --chart ./chart --release checkout -n prod \
  --capture ./apply.sqlite --yes
torque explain ./apply.sqlite --format markdown
```

Shows the full delivery path from source to build, verifier gate, reviewable
plan, rollout, capture, and explanation, with SQLite evidence attached across
the loop.

</details>

<details open>
<summary>Evidence model</summary>

```sql
SELECT session_id, command, started_at, ended_at
FROM torque_capture_sessions
ORDER BY started_at DESC;

SELECT seq, kind, namespace, pod, source
FROM torque_capture_events
WHERE session_id = ? ORDER BY seq;

SELECT name, seq
FROM torque_capture_artifacts
WHERE session_id = ? ORDER BY seq;
```

Maps sessions, event timelines, artifacts, and tags into the explain path so
captures remain portable and queryable after the cluster context is gone.

</details>

<details open>
<summary>Stack DAG scheduler</summary>

```bash
torque stack plan --config ./stacks/prod --output json
torque stack apply --config ./stacks/prod --yes --capture ./stack.sqlite
torque stack status --config ./stacks/prod --follow
torque stack rerun-failed --config ./stacks/prod --yes
```

Shows how stack input becomes a compiled dependency graph, scheduled waves,
stored run state, and a focused rerun that skips succeeded nodes and retries
failed ones.

</details>

<details open>
<summary>Secret-safe delivery path</summary>

```bash
torque apply plan --chart ./chart --release checkout -n prod \
  --secret-provider vault --output plan.md
torque build . --secret NPM_TOKEN \
  --secrets block --secrets-report ./secrets.json --capture ./build.sqlite
torque secrets discover --scope repo
```

Shows deploy-time `secret://` references resolving through providers while only
audit references are recorded, alongside BuildKit secret mounts and build
guardrail reports.

</details>

<details open>
<summary>Verifier and agent safety matrix</summary>

```bash
verifier --chart ./chart --release checkout -n prod \
  --format json --report verify.json
torque apply plan --chart ./chart --release checkout -n prod \
  --verify-report verify.json --output plan.md
torque agent simulate --scenario prod-apply --scenario destructive-delete \
  --scenario print-secrets --report agent-safety.json
```

Shows verifier coverage across 50 bad manifest categories with
blocked/warned/missed scoring, then maps agent attempts such as prod apply,
destructive delete, secret printing, unverified deploys, and broad log scraping
to guardrail outcomes.

</details>

<details open>
<summary>Package boundaries</summary>

```text
cmd/torque        CLI wiring, flags, command UX
internal/stack    stack compile, scheduling, run state, resume
internal/deploy   Helm plan/apply/delete streams and rollout status
internal/capture  SQLite sessions, events, artifacts, explain summaries
internal/helpui   static docs/search index and help UI
pkg/buildkit      build execution, attestations, OCI/cache repair
pkg/api           gRPC API types for torque-agent integrations
```

Keeps contributor-facing boundaries high-level so CLI wiring, internal runtime
logic, portable evidence, and public package APIs are easy to place.

</details>
