#!/usr/bin/env bash
# Deploy or manage the Kokoro TTS container on alpha-old.
set -euo pipefail

HOST="${KOKORO_HOST:-alpha-old}"
# alpha-old: Ryzen 9 7940HS + Radeon 780M (gfx1103 iGPU).
# Official Kokoro ROCm image ships PyTorch without gfx1103 kernels — GPU mode fails
# with "HIP error: invalid device function". Use CPU unless you build PyTorch for gfx1103.
DEVICE="${KOKORO_DEVICE:-cpu}"
CONTAINER="${KOKORO_CONTAINER:-kokoro}"
PORT="${KOKORO_PORT:-8880}"
HSA_GFX="${HSA_OVERRIDE_GFX_VERSION:-11.0.3}"

case "$DEVICE" in
  rocm)
    IMAGE="${KOKORO_IMAGE:-ghcr.io/remsky/kokoro-fastapi-rocm:latest}"
    RUN_EXTRA=(
      --device /dev/kfd
      --device /dev/dri
      --group-add 44    # video
      --group-add 115   # render
      --security-opt seccomp=unconfined
      -e USE_GPU=true
      -e "HSA_OVERRIDE_GFX_VERSION=${HSA_GFX}"
      -e TORCH_ROCM_AOTRITON_ENABLE_EXPERIMENTAL=1
      -v kokoro-miopen-cache:/root/.cache/miopen
      -v kokoro-miopen-config:/root/.config/miopen
    )
    ;;
  cpu)
    IMAGE="${KOKORO_IMAGE:-ghcr.io/remsky/kokoro-fastapi-cpu:latest}"
    RUN_EXTRA=()
    ;;
  *)
    echo "KOKORO_DEVICE must be 'rocm' or 'cpu', got: $DEVICE" >&2
    exit 1
    ;;
esac

usage() {
  cat <<EOF
Usage: $(basename "$0") <up|down|restart|status|logs|benchmark>

Manage Kokoro on ${HOST} (port ${PORT}, device=${DEVICE}).

  up         pull image and start container
  down       stop and remove container
  restart    down then up
  status     show container state, device mode, and probe /v1/models
  logs       tail container logs
  benchmark  time a short TTS request

Environment:
  KOKORO_DEVICE=rocm|cpu   (default: rocm)
  KOKORO_IMAGE=...         override container image
  HSA_OVERRIDE_GFX_VERSION  AMD GFX override (default: 11.0.3 for Phoenix iGPU)
EOF
}

ssh_host() { ssh "$HOST" "$@"; }

docker_run_cmd() {
  local -a cmd=(docker run -d --name "$CONTAINER" --restart unless-stopped -p "${PORT}:8880")
  cmd+=("${RUN_EXTRA[@]}")
  cmd+=("$IMAGE")
  printf '%q ' "${cmd[@]}"
}

cmd_up() {
  echo "Pulling ${IMAGE} on ${HOST} …"
  ssh_host "docker pull ${IMAGE}"
  ssh_host "docker rm -f ${CONTAINER} 2>/dev/null || true"
  echo "Starting Kokoro (${DEVICE}) …"
  ssh_host "$(docker_run_cmd)"
  echo "Waiting for Kokoro on ${HOST}:${PORT} …"
  for _ in $(seq 1 60); do
    code=$(ssh_host "curl -s -o /dev/null -w '%{http_code}' http://localhost:${PORT}/v1/models" || echo 000)
    if [[ "$code" == "200" ]]; then
      echo "Kokoro ready (${DEVICE}) at http://${HOST}:${PORT}/v1"
      ssh_host "docker logs ${CONTAINER} 2>&1 | tail -8"
      return 0
    fi
    sleep 3
  done
  echo "Container started but /v1/models not ready — check: $0 logs" >&2
  exit 1
}

cmd_down() {
  ssh_host "docker rm -f ${CONTAINER} 2>/dev/null || true"
  echo "Kokoro stopped on ${HOST}"
}

cmd_status() {
  echo "Mode: ${DEVICE} (${IMAGE})"
  ssh_host "docker ps -a --filter name=^/${CONTAINER}\$ --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}\t{{.Image}}'"
  code=$(curl -s -o /dev/null -w '%{http_code}' --connect-timeout 3 "http://${HOST}:${PORT}/v1/models" || echo 000)
  echo "Probe http://${HOST}:${PORT}/v1/models → HTTP ${code}"
}

cmd_logs() {
  ssh_host "docker logs -f ${CONTAINER}"
}

cmd_benchmark() {
  local text="The quick brown fox jumps over the lazy dog."
  echo "Benchmarking ${DEVICE} TTS (${#text} chars) …"
  local start end elapsed size
  start=$(date +%s.%N)
  size=$(curl -s -o /tmp/kokoro-bench.mp3 -w '%{size_download}' \
    -X POST "http://${HOST}:${PORT}/v1/audio/speech" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"kokoro\",\"input\":\"${text}\",\"voice\":\"af_heart\",\"response_format\":\"mp3\",\"speed\":1}")
  end=$(date +%s.%N)
  elapsed=$(echo "$end - $start" | bc)
  echo "Time: ${elapsed}s · Size: ${size} bytes"
}

case "${1:-}" in
  up) cmd_up ;;
  down) cmd_down ;;
  restart) cmd_down; cmd_up ;;
  status) cmd_status ;;
  logs) cmd_logs ;;
  benchmark) cmd_benchmark ;;
  *) usage; exit 1 ;;
esac
