# Verifier

Verifier is the standalone Kubernetes policy verifier included with torque. It checks Helm charts, rendered manifests, and live namespaces with the shared torque verification engine.

## Quick Start

```bash
go install ./cmd/verifier

verifier --chart ./chart --release my-app -n default
verifier --manifest ./rendered.yaml
verifier --namespace default --context my-context
```

## Reports And Baselines

```bash
verifier --manifest ./rendered.yaml --format html --report ./verify-report.html --open
verifier verify.yaml --baseline ./baseline.json
verifier verify.yaml --compare-to ./baseline.json
verifier rules list
```

## Evidence-First Security

```bash
verifier --chart ./chart --release api -n prod \
  --security-profile enterprise \
  --secrets-report secrets.json \
  --security-evidence ./torque-security-evidence \
  --format json --report verify.json
```

The enterprise security profile scans rendered Kubernetes objects for
secret-like values outside approved Secret boundaries, merges redacted
`secret_flow` findings into the verifier report, writes a separate secrets
report, and exports a bundle with `manifest.json`, `secrets.report.json`,
`verifier.report.json`, `redaction.proof.json`, and `reports/security.md`.

The older `verify` binary remains available for existing CI scripts, but new docs and examples use `verifier`.

For the next-generation security direction, see
[Evidence-First Secrets And Verifier Spec](secrets-verifier-evidence-spec.md).

![Verifier report](assets/verifier/verify.png)
