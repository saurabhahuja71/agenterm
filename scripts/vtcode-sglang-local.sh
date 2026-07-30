#!/usr/bin/env bash
# VT Code against SGLang via toolcall rewrite proxy (local).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROXY_PORT="${SGLANG_TOOLCALL_PROXY_PORT:-30001}"
UPSTREAM="${SGLANG_UPSTREAM:-http://127.0.0.1:30000}"

if ! curl --noproxy '*' -m 1 -fsS "http://127.0.0.1:${PROXY_PORT}/v1/models" >/dev/null 2>&1; then
  echo "starting sglang-toolcall-proxy on :${PROXY_PORT} -> ${UPSTREAM}"
  nohup python3 "$ROOT/scripts/sglang-toolcall-proxy.py" "$UPSTREAM" "$PROXY_PORT" \
    >"$HOME/.cache/sglang-toolcall-proxy.log" 2>&1 &
  for i in $(seq 1 20); do
    curl --noproxy '*' -m 1 -fsS "http://127.0.0.1:${PROXY_PORT}/v1/models" >/dev/null 2>&1 && break
    sleep 0.25
  done
fi

# Ensure SGLang tunnel/server if helper exists
if declare -F _sglang_ensure_api >/dev/null 2>&1; then
  _sglang_ensure_api || true
fi

model_id=$(curl --noproxy '*' -m 3 -fsS "http://127.0.0.1:${PROXY_PORT}/v1/models" \
  | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["data"][0]["id"])' 2>/dev/null || true)
model_id=$(basename "${model_id:-qwen2.5-coder-32b-q4_k_m.gguf}")
base="http://127.0.0.1:${PROXY_PORT}/v1"

echo "vtcode-sglang-local: base=${base} model=${model_id}"
exec env -u http_proxy -u https_proxy -u HTTP_PROXY -u HTTPS_PROXY \
  OPENAI_API_KEY=sglang \
  LMSTUDIO_BASE_URL="$base" \
  OPENAI_BASE_URL="$base" \
  no_proxy=localhost,127.0.0.1 \
  NO_PROXY=localhost,127.0.0.1 \
  vtcode --provider lmstudio --model "$model_id" "$@"
