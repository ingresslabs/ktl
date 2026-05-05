#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

status=0

gif_files="$(
  {
    find docs -type f -iname '*.gif' -print 2>/dev/null || true
    find site -type f -iname '*.gif' ! -path 'site/assets/demos/*' -print 2>/dev/null || true
  }
)"
if [[ -n "${gif_files}" ]]; then
  echo "error: GIF assets are not allowed in docs or generated docs output:" >&2
  echo "${gif_files}" >&2
  status=1
fi

gif_refs="$(
  rg -n --hidden '(?i)\.gif|image/gif|GIF89a|Output[[:space:]].*\.gif' \
    docs internal/helpui site/docs.html site/index.json 2>/dev/null || true
)"
if [[ -n "${gif_refs}" ]]; then
  echo "error: GIF references are not allowed in docs surfaces:" >&2
  echo "${gif_refs}" >&2
  status=1
fi

exit "${status}"
