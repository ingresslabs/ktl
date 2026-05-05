# Demos

Text-only companion commands for the landing page demos. The animated demos live
on the landing page; docs stay copy/paste friendly and free of GIF assets.

<details open>
<summary>Complex DAG stack orchestration</summary>

```bash
torque stack plan --config testdata/stack/e2e/10-large-graph \
  --bundle ./dist/stack-large-graph.tgz
torque stack plan --config testdata/stack/e2e/10-large-graph --output json
torque stack status --config ./stacks/prod --follow
```

Plans a dependency-ordered stack, seals the review bundle, and follows rollout
status in dependency waves.

</details>

<details open>
<summary>DAG performance scheduling</summary>

```bash
torque stack plan --config testdata/stack/e2e/02-fanout --output json
torque stack plan --config testdata/stack/e2e/03-fanin --output json
torque stack plan --config testdata/stack/e2e/10-large-graph --output json
```

Shows how shared roots, joins, and larger graphs are reduced into deterministic
waves before anything is applied.

</details>

<details open>
<summary>Sandboxed builds and secrets</summary>

```bash
torque build sandbox doctor --sandbox-config sandbox/linux-ci.cfg
torque build . --sandbox --sandbox-config sandbox/linux-ci.cfg \
  --capture ./build.sqlite
```

Checks the sandbox profile, then runs the build inside the constrained builder
while writing portable build evidence.

</details>

<details open>
<summary>Helmer archives and verifier gates</summary>

```bash
helmer archive ./chart --output ./chart.tgz
verifier --chart ./chart --release api -n prod --format json --report verify.json
torque apply plan --chart ./chart --release api -n prod \
  --verify-report verify.json --output plan.md
```

Keeps the chart archive, rendered manifests, verifier report, and release plan
bound together for review.

</details>

<details open>
<summary>Helmer HTML plan reports</summary>

```bash
helmer report ./chart --output ./plan.html
torque apply plan --chart ./chart --release api -n prod \
  --visualize --output ./plan.html
torque apply plan --chart ./chart --release api -n prod \
  --build-capture ./build.sqlite --format html --output ./plan.html
```

Produces a reviewable HTML plan report while attaching the same portable build
evidence reviewers use for release decisions.

</details>

<details open>
<summary>Kubernetes logs and evidence capture</summary>

```bash
torque logs 'checkout-.*' -n prod-payments \
  --events --highlight 'ERROR|WARN' --capture ./logs.sqlite --tail 100
torque logs deploy/checkout -n prod-payments \
  --deploy-mode stable+canary --ws-listen :9090
torque explain ./logs.sqlite --format markdown
```

Tails matching pods with events and highlighted failure signals, mirrors the
same stream over WebSocket for live review, and stores the session as portable
SQLite evidence for later explanation.

</details>
