# S3 Build Cache

Torque can configure BuildKit's native S3 cache backend directly from `torque build` and `torque ship`.

```bash
torque build . \
  --tag ghcr.io/acme/api:dev \
  --s3-cache s3://acme-build-cache/torque/main \
  --s3-cache-region us-east-1
```

The same cache flags are available on `torque ship` and are forwarded only to the build step:

```bash
torque ship \
  --chart ./chart \
  --release api \
  --build . \
  --tag ghcr.io/acme/api:dev \
  --s3-cache s3://acme-build-cache/torque/main \
  --s3-cache-region us-east-1 \
  --yes
```

`--s3-cache` expands to both BuildKit cache import and export entries:

```text
type=s3,bucket=acme-build-cache,region=us-east-1,prefix=torque/main/,name=<manifest>
type=s3,bucket=acme-build-cache,region=us-east-1,prefix=torque/main/,name=<manifest>,mode=max
```

The manifest name defaults to the first image tag, sanitized for S3 object keys. Override it when several jobs should share one logical cache:

```bash
torque build . --tag ghcr.io/acme/api:dev \
  --s3-cache s3://acme-build-cache/torque/main \
  --s3-cache-name api-main \
  --s3-cache-mode max
```

## MCP Cache Advisor

Agents should use the MCP cache advisor tools instead of scraping BuildKit log
lines:

- `torque.cache.inspect` returns normalized `cacheFrom` / `cacheTo` entries,
  S3 manifest settings, warnings, and optional local cache-intel evidence.
- `torque.cache.plan` classifies changed paths into cache impact groups such as
  `dockerfile`, `dependency-layer`, and `source-layer`, then returns warm
  targets.
- `torque.cache.warm` runs a confirmed remote `BuildService.RunBuild` with
  push/load/sign disabled and configured cache exports enabled.

Example MCP call body for planning a shared S3 cache:

```json
{
  "contextDir": ".",
  "dockerfile": "Dockerfile",
  "tags": ["ghcr.io/acme/api:dev"],
  "changedPaths": ["go.mod", "cmd/api/main.go"],
  "s3Cache": "s3://acme-build-cache/torque/main",
  "s3CacheRegion": "us-east-1",
  "s3CacheName": "api-main"
}
```

Warm is a mutating MCP tool because it writes cache exports. Start
`torque-mcp` with `--enable-write`, pass `safety.confirm=true`, and keep AWS
credentials on the BuildKit daemon or workload identity.

## Credentials

Prefer credentials at the BuildKit daemon level. BuildKit's S3 cache backend reads standard AWS SDK configuration from the daemon environment, instance profile, or web identity role. Torque intentionally does not add AWS access keys to CLI arguments.

For enterprise deployments, use an instance profile, workload identity, or a
short-lived BuildKit builder role scoped to the cache bucket/prefix. Do not pass
AWS keys through MCP tool inputs, `torque-agent` gRPC requests, or command-line
arguments. The MCP/remote bridge path should only carry the S3 cache reference,
region, endpoint, path-style mode, and cache manifest name.

For a disposable Docker Buildx builder:

```bash
docker buildx create --name torque-s3-cache --driver docker-container \
  --driver-opt env.AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY_ID" \
  --driver-opt env.AWS_SECRET_ACCESS_KEY="$AWS_SECRET_ACCESS_KEY" \
  --driver-opt env.AWS_SESSION_TOKEN="$AWS_SESSION_TOKEN" \
  --driver-opt env.AWS_REGION="$AWS_REGION" \
  --bootstrap

torque build . --builder docker-container://buildx_buildkit_torque-s3-cache0 \
  --tag torque/s3-cache:test \
  --s3-cache s3://acme-build-cache/torque/e2e \
  --s3-cache-region "$AWS_REGION"
```

Remove disposable builders and buckets after validation:

```bash
docker buildx rm torque-s3-cache
aws s3 rm s3://acme-build-cache/torque/e2e --recursive
```

The repeatable live harness is `scripts/e2e/s3-cache-build-ship.sh`. It expects a disposable Buildx builder with AWS credentials already attached at the daemon level:

```bash
S3_CACHE_REF=s3://acme-build-cache/torque/e2e \
S3_CACHE_REGION=us-east-1 \
BUILDX_BUILDER=torque-s3-cache \
BUILD_RUNS=20 \
SHIP_RUNS=20 \
scripts/e2e/s3-cache-build-ship.sh
```

## S3-Compatible Endpoints

Use `--s3-cache-endpoint-url` and `--s3-cache-path-style` for compatible object stores:

```bash
torque build . --tag registry.local/api:dev \
  --s3-cache s3://torque-cache/builds/api \
  --s3-cache-region us-east-1 \
  --s3-cache-endpoint-url https://s3.example.internal \
  --s3-cache-path-style
```
