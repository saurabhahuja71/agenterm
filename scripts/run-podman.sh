#!/usr/bin/env bash
# Run agenterm TUI via Podman (or Docker) from any machine.
# Usage:
#   ./scripts/run-podman.sh              # build if needed + open TUI
#   ./scripts/run-podman.sh --ping
#   ./scripts/run-podman.sh --base-url http://192.168.1.50:11434/v1 -m qwen2.5
#   AGENTERM_BASE_URL=http://gpu:11434/v1 ./scripts/run-podman.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${AGENTERM_IMAGE:-agenterm:latest}"
CONFIG_DIR="${AGENTERM_CONFIG_DIR:-$HOME/.agenterm}"

# Prefer podman, fall back to docker
if command -v podman >/dev/null 2>&1; then
  ENGINE=podman
elif command -v docker >/dev/null 2>&1; then
  ENGINE=docker
else
  echo "error: need podman or docker installed" >&2
  exit 1
fi

# Podman rootless: ensure API socket if compose users need it (not required for run)
if [[ "$ENGINE" == podman ]]; then
  export DOCKER_HOST="${DOCKER_HOST:-unix:///run/user/$(id -u)/podman/podman.sock}"
fi

mkdir -p "$CONFIG_DIR"

# Build image if missing
if ! $ENGINE image exists "$IMAGE" 2>/dev/null; then
  echo "Building $IMAGE …"
  $ENGINE build -t "$IMAGE" "$ROOT"
fi

# Extra env for remote Ollama / cloud
ENV_ARGS=()
[[ -n "${AGENTERM_BASE_URL:-}" ]] && ENV_ARGS+=(-e "AGENTERM_BASE_URL=$AGENTERM_BASE_URL")
[[ -n "${AGENTERM_MODEL:-}" ]] && ENV_ARGS+=(-e "AGENTERM_MODEL=$AGENTERM_MODEL")
[[ -n "${AGENTERM_PROVIDER:-}" ]] && ENV_ARGS+=(-e "AGENTERM_PROVIDER=$AGENTERM_PROVIDER")
[[ -n "${AGENTERM_API_KEY:-}" ]] && ENV_ARGS+=(-e "AGENTERM_API_KEY=$AGENTERM_API_KEY")
[[ -n "${XAI_API_KEY:-}" ]] && ENV_ARGS+=(-e "XAI_API_KEY=$XAI_API_KEY")
[[ -n "${OPENAI_API_KEY:-}" ]] && ENV_ARGS+=(-e "OPENAI_API_KEY=$OPENAI_API_KEY")

# Host network so 127.0.0.1:11434 is the host's Ollama on Linux
# On macOS Docker Desktop, use host.docker.internal instead (see README)
NETWORK_ARGS=(--network host)
if [[ "$(uname -s)" == "Darwin" ]]; then
  NETWORK_ARGS=()
  ENV_ARGS+=(-e "AGENTERM_BASE_URL=${AGENTERM_BASE_URL:-http://host.docker.internal:11434/v1}")
  # publish nothing; still need host gateway
  NETWORK_ARGS+=(--add-host=host.docker.internal:host-gateway)
fi

exec $ENGINE run --rm -it \
  "${NETWORK_ARGS[@]}" \
  -e TERM="${TERM:-xterm-256color}" \
  -e COLORTERM="${COLORTERM:-truecolor}" \
  -e AGENTERM_BASE_URL="${AGENTERM_BASE_URL:-http://127.0.0.1:11434/v1}" \
  -e AGENTERM_MODEL="${AGENTERM_MODEL:-llama3.2}" \
  -e AGENTERM_API_KEY="${AGENTERM_API_KEY:-ollama}" \
  "${ENV_ARGS[@]}" \
  -v "$CONFIG_DIR:/home/agenterm/.agenterm:Z" \
  "$IMAGE" \
  "$@"
