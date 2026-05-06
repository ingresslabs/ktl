#!/usr/bin/env bash
set -euo pipefail

: "${S3_CACHE_REF:?set S3_CACHE_REF to s3://bucket/prefix}"
: "${S3_CACHE_REGION:?set S3_CACHE_REGION}"
: "${BUILDX_BUILDER:?set BUILDX_BUILDER to the disposable buildx builder name}"

TORQUE_BIN="${TORQUE_BIN:-./bin/torque}"
BUILD_RUNS="${BUILD_RUNS:-20}"
SHIP_RUNS="${SHIP_RUNS:-20}"
S3_CACHE_NAME="${S3_CACHE_NAME:-torque-s3-cache-e2e}"
BUILDER_ADDR="${BUILDER_ADDR:-docker-container://buildx_buildkit_${BUILDX_BUILDER}0}"
WORKDIR="${WORKDIR:-$(mktemp -d)}"

mkdir -p "$WORKDIR/context" "$WORKDIR/chart/templates" "$WORKDIR/logs"

cat > "$WORKDIR/context/Dockerfile" <<'DOCKERFILE'
FROM alpine:3.20
RUN --mount=type=cache,target=/var/cache/apk apk add --no-cache ca-certificates >/dev/null
RUN printf 'torque-s3-cache-e2e\n' > /marker.txt
CMD ["cat", "/marker.txt"]
DOCKERFILE

cat > "$WORKDIR/chart/Chart.yaml" <<'YAML'
apiVersion: v2
name: torque-s3-cache-e2e
version: 0.1.0
YAML

cat > "$WORKDIR/chart/templates/configmap.yaml" <<'YAML'
apiVersion: v1
kind: ConfigMap
metadata:
  name: "{{ .Release.Name }}"
data:
  fixture: s3-cache-e2e
YAML

run_torque_build() {
  local idx="$1"
  local log="$WORKDIR/logs/build-${idx}.log"
  docker buildx prune -af --builder "$BUILDX_BUILDER" >/dev/null 2>&1 || true
  BUILDKIT_PROGRESS=plain "$TORQUE_BIN" build "$WORKDIR/context" \
    --builder "$BUILDER_ADDR" \
    --cache-dir "$WORKDIR/cache-build-${idx}" \
    --tag torque/s3-cache-e2e:build \
    --push=false \
    --output logs \
    --cache-intel=false \
    --s3-cache "$S3_CACHE_REF" \
    --s3-cache-region "$S3_CACHE_REGION" \
    --s3-cache-name "$S3_CACHE_NAME" \
    >"$log" 2>&1
}

run_torque_ship() {
  local idx="$1"
  local log="$WORKDIR/logs/ship-${idx}.log"
  docker buildx prune -af --builder "$BUILDX_BUILDER" >/dev/null 2>&1 || true
  TORQUE_CACHE_DIR="$WORKDIR/cache-ship-${idx}" \
  BUILDKIT_PROGRESS=plain "$TORQUE_BIN" ship \
    --chart "$WORKDIR/chart" \
    --release "torque-s3-cache-e2e" \
    --namespace "torque-s3-cache-e2e" \
    --build "$WORKDIR/context" \
    --builder "$BUILDER_ADDR" \
    --tag torque/s3-cache-e2e:ship \
    --push=false \
    --attest=false \
    --skip-verify \
    --plan-only \
    --evidence-dir "$WORKDIR/evidence-ship-${idx}" \
    --build-output logs \
    --s3-cache "$S3_CACHE_REF" \
    --s3-cache-region "$S3_CACHE_REGION" \
    --s3-cache-name "$S3_CACHE_NAME" \
    >"$log" 2>&1
}

for idx in $(seq 1 "$BUILD_RUNS"); do
  run_torque_build "$idx"
done

for idx in $(seq 1 "$SHIP_RUNS"); do
  run_torque_ship "$idx"
done

if grep -R --exclude='*.json' -E 'AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN|AKIA[0-9A-Z]{16}' "$WORKDIR/logs" >/dev/null; then
  echo "sensitive AWS material was written to logs" >&2
  exit 1
fi

echo "s3_cache_e2e_ok build_runs=${BUILD_RUNS} ship_runs=${SHIP_RUNS} workdir=${WORKDIR}"
