#!/usr/bin/env sh
set -eu

repo="${TORQUE_REPO:-ingresslabs/torque}"
tool="${TORQUE_TOOL:-torque}"
version="${TORQUE_VERSION:-latest}"
install_dir="${TORQUE_INSTALL_DIR:-}"
install_dir_explicit=0
os="${TORQUE_OS:-}"
arch="${TORQUE_ARCH:-}"
dry_run="${TORQUE_DRY_RUN:-0}"
checksum="${TORQUE_CHECKSUM:-1}"
mode="${TORQUE_INSTALL_MODE:-binary}"
download_base_url="${TORQUE_DOWNLOAD_BASE_URL:-}"
systemd_start="${TORQUE_SYSTEMD_START:-1}"
systemd_force="${TORQUE_SYSTEMD_FORCE:-0}"
systemd_dir="${TORQUE_SYSTEMD_DIR:-/etc/systemd/system}"
systemd_env_file="${TORQUE_SYSTEMD_ENV_FILE:-/etc/torque/agent.env}"
token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"

usage() {
  cat >&2 <<EOF
Install torque from GitHub Releases.

Usage:
  install.sh [--version <tag>] [--dir <path>] [--tool <name>] [--repo <owner/repo>]
             [--os <linux|darwin>] [--arch <amd64|arm64>] [--dry-run]
             [--skip-checksum]
  install.sh --mode systemd-daemon [--version <tag>] [--repo <owner/repo>] [--no-start]

Environment:
  TORQUE_VERSION       Release tag to install, or latest. Default: latest
  TORQUE_INSTALL_DIR   Install directory. Default: existing binary dir, /usr/local/bin, or ~/.local/bin
  TORQUE_REPO          GitHub repository. Default: ingresslabs/torque
  TORQUE_TOOL          Binary to install. Default: torque
  TORQUE_INSTALL_MODE  binary or systemd-daemon. Default: binary
  TORQUE_DOWNLOAD_BASE_URL Override release asset base URL (for offline/test installs)
  TORQUE_OS            Override detected OS
  TORQUE_ARCH          Override detected architecture
  TORQUE_DRY_RUN       Print what would happen without installing
  TORQUE_CHECKSUM      Verify sha256 when release checksums exist. Default: 1
  TORQUE_SYSTEMD_START Enable and start services in systemd-daemon mode. Default: 1
  TORQUE_SYSTEMD_FORCE Overwrite /etc/torque/agent.env in systemd-daemon mode. Default: 0
  GH_TOKEN/GITHUB_TOKEN Token for private or rate-limited GitHub access
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version|-v)
      version="${2:-}"
      shift 2
      ;;
    --dir|-d|--install-dir)
      install_dir="${2:-}"
      install_dir_explicit=1
      shift 2
      ;;
    --repo)
      repo="${2:-}"
      shift 2
      ;;
    --tool)
      tool="${2:-}"
      shift 2
      ;;
    --os)
      os="${2:-}"
      shift 2
      ;;
    --arch)
      arch="${2:-}"
      shift 2
      ;;
    --dry-run)
      dry_run=1
      shift
      ;;
    --skip-checksum)
      checksum=0
      shift
      ;;
    --mode)
      mode="${2:-}"
      shift 2
      ;;
    --systemd|--systemd-daemon)
      mode="systemd-daemon"
      shift
      ;;
    --no-start)
      systemd_start=0
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 2
      ;;
  esac
done

[ -n "$repo" ] || { echo "repo is required" >&2; exit 2; }
[ -n "$tool" ] || { echo "tool is required" >&2; exit 2; }
[ -n "$version" ] || { echo "version is required" >&2; exit 2; }

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
need mktemp
need tr
need find
need head
need dirname

case "$mode" in
  binary|systemd-daemon) ;;
  *) echo "unsupported install mode: ${mode}" >&2; exit 2 ;;
esac

normalize_os() {
  value="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "$value" in
    linux|darwin) printf '%s\n' "$value" ;;
    *) echo "unsupported OS: $1" >&2; exit 2 ;;
  esac
}

normalize_arch() {
  case "$1" in
    x86_64|amd64) printf '%s\n' amd64 ;;
    arm64|aarch64) printf '%s\n' arm64 ;;
    *) echo "unsupported architecture: $1" >&2; exit 2 ;;
  esac
}

if [ -z "$os" ]; then
  os="$(uname -s)"
fi
os="$(normalize_os "$os")"

if [ -z "$arch" ]; then
  arch="$(uname -m)"
fi
arch="$(normalize_arch "$arch")"

if [ "$mode" = "systemd-daemon" ] && [ "$os" != "linux" ]; then
  echo "systemd-daemon mode is only supported on Linux" >&2
  exit 2
fi

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

if [ "$version" = "latest" ]; then
  api_url="https://api.github.com/repos/${repo}/releases/latest"
  release_json="$(github_curl "$api_url")" || {
    echo "could not read latest release from ${repo}" >&2
    exit 1
  }
  version="$(
    printf '%s' "$release_json" |
      sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
      head -n 1
  )"
  [ -n "$version" ] || {
    echo "could not determine latest release tag" >&2
    exit 1
  }
fi

base_url="${download_base_url:-https://github.com/${repo}/releases/download/${version}}"

default_install_dir() {
  tool_name="$1"
  if existing="$(command -v "$tool_name" 2>/dev/null)"; then
    case "$existing" in
      */*) printf '%s\n' "${existing%/*}"; return ;;
    esac
  fi
  if [ -d /usr/local/bin ] && { [ -w /usr/local/bin ] || command -v sudo >/dev/null 2>&1; }; then
    printf '%s\n' /usr/local/bin
    return
  fi
  printf '%s\n' "${HOME}/.local/bin"
}

if [ "$mode" = "systemd-daemon" ]; then
  if [ "$install_dir_explicit" = "1" ] && [ "$install_dir" != "/usr/bin" ]; then
    echo "systemd-daemon mode installs service binaries to /usr/bin; omit --dir or pass --dir /usr/bin" >&2
    exit 2
  fi
  install_dir="/usr/bin"
fi

if [ -z "$install_dir" ]; then
  install_dir="$(default_install_dir "$tool")"
fi

sha_cmd() {
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s\n' "sha256sum"
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s\n' "shasum -a 256"
  else
    return 1
  fi
}

verify_checksum() {
  asset="$1"
  archive="$2"
  url="$3"
  [ "$checksum" = "1" ] || return 0
  cmd="$(sha_cmd)" || {
    echo "checksum tool not found; skipping sha256 verification" >&2
    return 0
  }

  checksum_file="${tmp}/checksums.txt"
  checksum_url="${base_url}/checksums-${os}-${arch}-${version}.txt"
  if ! github_curl "$checksum_url" -o "$checksum_file" 2>/dev/null; then
    if ! github_curl "${url}.sha256" -o "$checksum_file" 2>/dev/null; then
      echo "checksums not found for ${asset}; skipping sha256 verification" >&2
      return 0
    fi
  fi

  expected="$(
    sed -n "s/^\([0-9a-fA-F][0-9a-fA-F]*\)[[:space:]][[:space:]]*.*${asset}\$/\1/p" "$checksum_file" |
      head -n 1
  )"
  if [ -z "$expected" ]; then
    echo "checksum file did not mention ${asset}; skipping sha256 verification" >&2
    return 0
  fi

  actual="$($cmd "$archive" | sed 's/[[:space:]].*//')"
  if [ "$actual" != "$expected" ]; then
    echo "sha256 mismatch for ${asset}" >&2
    echo "expected: ${expected}" >&2
    echo "actual:   ${actual}" >&2
    exit 1
  fi
  echo "verified sha256: ${actual}" >&2
}

run_root() {
  if [ "$dry_run" = "1" ]; then
    printf 'dry run: would run as root:' >&2
    for arg in "$@"; do
      printf ' %s' "$arg" >&2
    done
    printf '\n' >&2
    return 0
  fi
  if [ "$(id -u 2>/dev/null || echo 1)" = "0" ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    echo "root privileges are required; rerun as root or install sudo" >&2
    exit 1
  fi
}

install_binary() {
  src="$1"
  dst="$2"
  dst_dir="$(dirname "$dst")"
  chmod 0755 "$src"
  if mkdir -p "$dst_dir" 2>/dev/null && [ -w "$dst_dir" ]; then
    if command -v install >/dev/null 2>&1; then
      install -m 0755 "$src" "$dst"
    else
      cp "$src" "$dst"
      chmod 0755 "$dst"
    fi
  else
    run_root mkdir -p "$dst_dir"
    if command -v install >/dev/null 2>&1; then
      run_root install -m 0755 "$src" "$dst"
    else
      run_root cp "$src" "$dst"
      run_root chmod 0755 "$dst"
    fi
  fi
}

install_release_tool() {
  tool_name="$1"
  asset="${tool_name}-${os}-${arch}-${version}.tar.gz"
  url="${base_url}/${asset}"
  archive="${tmp}/${asset}"
  extract_dir="${tmp}/extract-${tool_name}"
  dest_dir="$install_dir"
  if [ -z "$dest_dir" ]; then
    dest_dir="$(default_install_dir "$tool_name")"
  fi

  echo "repo:    ${repo}" >&2
  echo "version: ${version}" >&2
  echo "target:  ${os}/${arch}" >&2
  echo "asset:   ${asset}" >&2
  echo "dest:    ${dest_dir}/${tool_name}" >&2

  if [ "$dry_run" = "1" ]; then
    echo "dry run: would download ${url}" >&2
    return 0
  fi

  echo "downloading ${asset}" >&2
  github_curl -L "$url" -o "$archive" || {
    echo "could not download release asset: ${asset}" >&2
    exit 1
  }

  verify_checksum "$asset" "$archive" "$url"

  mkdir -p "$extract_dir"
  tar -xzf "$archive" -C "$extract_dir"
  bin_path="$(find "$extract_dir" -type f -name "$tool_name" | head -n 1)"
  if [ -z "$bin_path" ]; then
    echo "release archive did not contain ${tool_name}" >&2
    exit 1
  fi

  install_binary "$bin_path" "${dest_dir}/${tool_name}"
  echo "installed ${tool_name} to ${dest_dir}/${tool_name}" >&2
  case "$tool_name" in
    torque)
      "${dest_dir}/${tool_name}" version 2>/dev/null || true
      ;;
  esac
}

random_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  if command -v od >/dev/null 2>&1; then
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
    printf '\n'
    return
  fi
  echo "missing openssl or od for token generation" >&2
  exit 2
}

render_agent_env() {
  remote_token="${TORQUE_REMOTE_TOKEN:-}"
  mcp_token="${TORQUE_MCP_TOKEN:-}"
  if [ -z "$remote_token" ]; then
    remote_token="$(random_token)"
  fi
  if [ -z "$mcp_token" ]; then
    mcp_token="$(random_token)"
  fi
  cat <<EOF
TORQUE_REMOTE_TOKEN=${remote_token}
TORQUE_MCP_TOKEN=${mcp_token}
TORQUE_AGENT_LISTEN=${TORQUE_AGENT_LISTEN:-:7443}
TORQUE_AGENT_HTTP_LISTEN=${TORQUE_AGENT_HTTP_LISTEN:-127.0.0.1:8081}
TORQUE_AGENT_MIRROR_STORE=${TORQUE_AGENT_MIRROR_STORE:-/var/lib/torque/agent/mirror.sqlite}
TORQUE_AGENT_SANDBOX_BIN=${TORQUE_AGENT_SANDBOX_BIN:-nsjail}
TORQUE_AGENT_SANDBOX_CONFIG=${TORQUE_AGENT_SANDBOX_CONFIG:-}
TORQUE_MCP_LISTEN=${TORQUE_MCP_LISTEN:-127.0.0.1:7331}
TORQUE_MCP_REMOTE_AGENT=${TORQUE_MCP_REMOTE_AGENT:-127.0.0.1:7443}
KUBECONFIG=${KUBECONFIG:-/etc/torque/kubeconfig}
TORQUE_KUBE_CONTEXT=${TORQUE_KUBE_CONTEXT:-}
EOF
}

render_agent_unit() {
  cat <<'EOF'
[Unit]
Description=Torque durable gRPC agent
Documentation=https://github.com/ingresslabs/torque/blob/main/docs/grpc-agent.md
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
Environment=TORQUE_AGENT_LISTEN=:7443
Environment=TORQUE_AGENT_HTTP_LISTEN=127.0.0.1:8081
Environment=TORQUE_AGENT_MIRROR_STORE=/var/lib/torque/agent/mirror.sqlite
Environment=TORQUE_AGENT_SANDBOX_BIN=nsjail
Environment=TORQUE_AGENT_SANDBOX_CONFIG=
EnvironmentFile=-/etc/torque/agent.env
StateDirectory=torque
CacheDirectory=torque
RuntimeDirectory=torque
WorkingDirectory=/var/lib/torque
ExecStart=/usr/bin/torque-agent -mode=durable -listen=${TORQUE_AGENT_LISTEN} -http-listen=${TORQUE_AGENT_HTTP_LISTEN} -kubeconfig=${KUBECONFIG} -context=${TORQUE_KUBE_CONTEXT} -mirror-store=${TORQUE_AGENT_MIRROR_STORE} -build-sandbox -build-sandbox-bin=${TORQUE_AGENT_SANDBOX_BIN} -build-sandbox-config=${TORQUE_AGENT_SANDBOX_CONFIG} -build-sandbox-logs
Restart=on-failure
RestartSec=2s
KillSignal=SIGTERM
TimeoutStopSec=30s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
}

render_mcp_unit() {
  cat <<'EOF'
[Unit]
Description=Torque MCP HTTP bridge
Documentation=https://github.com/ingresslabs/torque/blob/main/docs/mcp-server-spec.md
Wants=network-online.target torque-agent.service
After=network-online.target torque-agent.service

[Service]
Type=simple
Environment=TORQUE_MCP_LISTEN=127.0.0.1:7331
Environment=TORQUE_MCP_REMOTE_AGENT=127.0.0.1:7443
EnvironmentFile=-/etc/torque/agent.env
ExecStart=/usr/bin/torque-mcp --listen=${TORQUE_MCP_LISTEN} --remote-agent=${TORQUE_MCP_REMOTE_AGENT}
Restart=on-failure
RestartSec=2s
KillSignal=SIGTERM
TimeoutStopSec=30s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
}

install_root_file() {
  src="$1"
  dst="$2"
  mode_bits="$3"
  run_root mkdir -p "$(dirname "$dst")"
  run_root install -m "$mode_bits" "$src" "$dst"
}

install_systemd_units() {
  if [ "$dry_run" != "1" ]; then
    command -v systemctl >/dev/null 2>&1 || {
      echo "systemctl is required for systemd-daemon mode" >&2
      exit 2
    }
  fi

  env_tmp="${tmp}/agent.env"
  agent_unit_tmp="${tmp}/torque-agent.service"
  mcp_unit_tmp="${tmp}/torque-mcp.service"
  render_agent_env > "$env_tmp"
  render_agent_unit > "$agent_unit_tmp"
  render_mcp_unit > "$mcp_unit_tmp"

  if [ -f "$systemd_env_file" ] && [ "$systemd_force" != "1" ]; then
    echo "keeping existing ${systemd_env_file} (set TORQUE_SYSTEMD_FORCE=1 to overwrite)" >&2
  else
    install_root_file "$env_tmp" "$systemd_env_file" 0600
    echo "wrote ${systemd_env_file}" >&2
  fi
  install_root_file "$agent_unit_tmp" "${systemd_dir}/torque-agent.service" 0644
  install_root_file "$mcp_unit_tmp" "${systemd_dir}/torque-mcp.service" 0644
  echo "wrote systemd units under ${systemd_dir}" >&2

  run_root systemctl daemon-reload
  if [ "$systemd_start" = "1" ]; then
    run_root systemctl enable torque-agent.service torque-mcp.service
    run_root systemctl restart torque-agent.service torque-mcp.service
    echo "started torque-agent.service and torque-mcp.service" >&2
  else
    echo "systemd services installed but not started; run: systemctl enable --now torque-agent.service torque-mcp.service" >&2
  fi
}

if [ "$mode" = "systemd-daemon" ]; then
  install_dir="${install_dir:-/usr/bin}"
  install_release_tool torque
  install_release_tool torque-agent
  install_release_tool torque-mcp
  install_systemd_units
else
  install_release_tool "$tool"
fi
