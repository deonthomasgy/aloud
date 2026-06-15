# invtts — Kokoro Text-to-Speech

A small Go web app with a clean UI for pasting text and turning it into
natural-sounding speech using the [Kokoro-82M](https://kokorottsai.com/#tryonline)
TTS model.

`kokorottsai.com` itself is a demo/marketing site with no callable API, so this
app talks to a **self-hosted, OpenAI-compatible Kokoro endpoint** (e.g.
[Kokoro-FastAPI](https://github.com/remsky/Kokoro-FastAPI) or
[docker-kokoro](https://github.com/hwdsl2/docker-kokoro)). The Go server proxies
synthesis requests to that endpoint, so your browser never needs the upstream URL
or key.

```
browser  ──►  invtts (local, :8080)  ──►  Kokoro on alpha-old (:8880, /v1/audio/speech)
```

## Quick start (native macOS app)

Build and open the **SwiftUI desktop app** — no browser, no local server:

```bash
make app
open Invtts.app
```

The app talks directly to Kokoro on **alpha-old** (`http://alpha-old:8880/v1`). Change the endpoint in **Invtts → Settings** if needed.

## Quick start (web UI, optional)

Kokoro runs in Docker on **alpha-old** (Tailscale). The Go server proxies to it — default `KOKORO_BASE_URL` is `http://alpha-old:8880/v1`.

**1. Start Kokoro on alpha-old** (once, or after updates):

```bash
make kokoro-up
# or: ./scripts/kokoro-alpha-old.sh up
```

**2. Run invtts locally:**

```bash
go run .
```

Open http://localhost:8080. The footer shows whether Kokoro is reachable.

Check Kokoro from here:

```bash
make kokoro-status
curl http://alpha-old:8880/v1/models
```

## Kokoro on alpha-old

The helper script SSHs to `alpha-old` and manages the `kokoro` container:

```bash
./scripts/kokoro-alpha-old.sh up       # pull + start
./scripts/kokoro-alpha-old.sh status   # container + HTTP probe
./scripts/kokoro-alpha-old.sh logs     # tail logs
./scripts/kokoro-alpha-old.sh down     # stop and remove
```

Override the SSH host with `KOKORO_HOST` if needed.

**GPU note:** alpha-old has a Radeon 780M iGPU (`gfx1103`). The official ROCm
image (`kokoro-fastapi-rocm`) fails on this chip because PyTorch only ships HIP
kernels for `gfx1100`–`gfx1102`, not `gfx1103` — you get `HIP error: invalid
device function`. The deploy script defaults to the **CPU** image, which works.
To force ROCm anyway: `KOKORO_DEVICE=rocm ./scripts/kokoro-alpha-old.sh up`.

To run Kokoro somewhere else instead, set `KOKORO_BASE_URL` (see Configuration).

## Run invtts in Docker (optional)

If you prefer a containerised UI locally (still talks to alpha-old):

```bash
docker compose up --build
```

## Configuration

All optional, set via environment variables:

| Variable           | Default                     | Description                                  |
|--------------------|-----------------------------|----------------------------------------------|
| `PORT`             | `8080`                      | Port the web UI listens on                   |
| `KOKORO_BASE_URL`  | `http://alpha-old:8880/v1`  | OpenAI-compatible Kokoro base URL            |
| `KOKORO_API_KEY`   | `not-needed`                | Bearer token sent upstream                   |
| `KOKORO_MODEL`     | `kokoro`                    | Model name passed upstream                   |
| `KOKORO_MAX_CHARS` | `50000`                     | Reject text longer than this                 |

Example:

```bash
KOKORO_BASE_URL=http://192.168.1.50:8880/v1 PORT=9000 go run .
```

## Features

- Beautiful single-page UI (no build step — HTML/CSS/JS is embedded in the binary)
- Full Kokoro voice catalogue grouped by language (American/British English,
  Japanese, Mandarin, French, Hindi, Italian, Portuguese, Spanish), defaulting to
  `af_heart`
- Selectable output format (MP3, WAV, Opus, FLAC, AAC) and speed (0.5×–2×)
- Inline player + one-click download
- Server-side validation of voice, format, speed, and length

## Project layout

```
macos/                   Native SwiftUI macOS app (Invtts.app)
main.go                  HTTP server (optional web UI)
voices.go                Kokoro voice catalogue + validation
web/index.html           Embedded single-page UI
scripts/kokoro-alpha-old.sh  Deploy/manage Kokoro Docker on alpha-old
Dockerfile               Multi-stage build for containerised invtts
docker-compose.yml       Local invtts container (points at alpha-old)
Makefile                 run, build, test, kokoro-up/down shortcuts
```

## API

`GET /api/health` — JSON `{ "status": "ok", "endpoint", "model" }` or 503 if Kokoro is unreachable.

`POST /api/tts` — JSON `{ "text", "voice", "format", "speed" }`, returns raw
audio bytes with the matching `Content-Type`.

`GET /api/voices` — JSON `{ "default", "groups": [{ "language", "voices": [...] }] }`.
