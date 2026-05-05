#!/usr/bin/env sh
set -eu

repo="${TORQUE_REPO:-ingresslabs/torque}"
tool="${TORQUE_TOOL:-torque}"
install_dir="${TORQUE_INSTALL_DIR:-/usr/local/bin}"
token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 2
  }
}

need curl
need tar
need uname
need sed

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux|darwin) ;;
  *)
    echo "unsupported OS: $os" >&2
    exit 2
    ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "unsupported architecture: $arch" >&2
    exit 2
    ;;
esac

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

github_curl() {
  if [ -n "$token" ]; then
    curl -fsSL \
      -H "Authorization: Bearer $token" \
      -H "Accept: application/vnd.github+json" \
      "$@"
  else
    curl -fsSL "$@"
  fi
}

api_url="https://api.github.com/repos/${repo}/releases/latest"
release_json="$(github_curl "$api_url")" || {
  echo "could not read latest release from ${repo}" >&2
  echo "if the repository is private, set GITHUB_TOKEN or GH_TOKEN first" >&2
  exit 1
}

tag="$(
  printf '%s' "$release_json" |
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
    head -n 1
)"
if [ -z "$tag" ]; then
  echo "could not determine latest release tag" >&2
  exit 1
fi

asset="${tool}-${os}-${arch}-${tag}.tar.gz"
url="https://github.com/${repo}/releases/download/${tag}/${asset}"
archive="${tmp}/${asset}"

echo "downloading ${repo} ${tag} (${os}/${arch})" >&2
github_curl -L "$url" -o "$archive" || {
  echo "could not download release asset: ${asset}" >&2
  echo "if the repository is private, set GITHUB_TOKEN or GH_TOKEN first" >&2
  exit 1
}

tar -xzf "$archive" -C "$tmp"
bin_path="$(find "$tmp" -type f -name "$tool" | head -n 1)"
if [ -z "$bin_path" ]; then
  echo "release archive did not contain ${tool}" >&2
  exit 1
fi

chmod 0755 "$bin_path"
if mkdir -p "$install_dir" 2>/dev/null && [ -w "$install_dir" ]; then
  install -m 0755 "$bin_path" "${install_dir}/${tool}"
elif command -v sudo >/dev/null 2>&1; then
  sudo mkdir -p "$install_dir"
  sudo install -m 0755 "$bin_path" "${install_dir}/${tool}"
else
  install_dir="${HOME}/.local/bin"
  mkdir -p "$install_dir"
  install -m 0755 "$bin_path" "${install_dir}/${tool}"
fi

echo "installed ${tool} to ${install_dir}/${tool}" >&2
