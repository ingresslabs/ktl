#!/usr/bin/env bash
set -euo pipefail

suite="release-promote-providers"
pkg="./cmd/torque"
regex='TestReleasePromote(CanaryWritesProofBackedPlan|BlueGreenExecuteFileProvider|BlocksFailedGate|KubernetesCanaryExecuteEndToEnd|KubernetesBlueGreenExecuteEndToEnd|ArgoRolloutsCanaryExecuteEndToEnd|ArgoRolloutsBlueGreenExecuteEndToEnd|ProviderDoesNotMutateWhenGateFails)$'
tests='["TestReleasePromoteCanaryWritesProofBackedPlan","TestReleasePromoteBlueGreenExecuteFileProvider","TestReleasePromoteBlocksFailedGate","TestReleasePromoteKubernetesCanaryExecuteEndToEnd","TestReleasePromoteKubernetesBlueGreenExecuteEndToEnd","TestReleasePromoteArgoRolloutsCanaryExecuteEndToEnd","TestReleasePromoteArgoRolloutsBlueGreenExecuteEndToEnd","TestReleasePromoteProviderDoesNotMutateWhenGateFails"]'

started="$(date +%s)"
status="passed"
if ! go test "${pkg}" -run "${regex}" -count=1 -v >&2; then
  status="failed"
fi
finished="$(date +%s)"
duration="$((finished - started))"

printf '{"suite":"%s","status":"%s","package":"%s","tests":%s,"durationSeconds":%s}\n' "${suite}" "${status}" "${pkg}" "${tests}" "${duration}"

if [[ "${status}" != "passed" ]]; then
  exit 1
fi
