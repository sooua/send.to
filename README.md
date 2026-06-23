<div align="center">

# send.to

**Easy and fast file sharing from the command line — and a clean web UI when you need it.**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://golang.org/)
[![Node](https://img.shields.io/badge/Node-20%2B-339933?logo=node.js)](https://nodejs.org/)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker)](./Dockerfile)
[![CI](https://img.shields.io/badge/CI-tested-brightgreen)](./.github/workflows/test.yml)

[English](./README.md) · [简体中文](./README.zh-CN.md)

</div>

---

## Why send.to

You have a file. Someone else needs it. You want a URL you can paste into chat, drop into a CI log, or `curl` down from a production box five minutes later — then have it disappear. `send.to` does exactly that, and nothing else.

```bash
curl --upload-file ./build.tar.gz https://send.to/build.tar.gz
# → https://send.to/aB3cD4eF/build.tar.gz

curl https://send.to/aB3cD4eF/build.tar.gz -o build.tar.gz
```

One static Go binary. One 52 MB Docker image. No database. No account. Your files.

---

## Table of contents

- [Features](#features)
- [Quick start](#quick-start)
- [Usage](#usage)
- [HTTP API](#http-api)
- [Configuration](#configuration)
- [Deployment](#deployment)
- [Architecture](#architecture)
- [Benchmarks](#benchmarks)
- [FAQ](#faq)
- [Contributing](#contributing)
- [License](#license)

---

## Features

| | |
|---|---|
| **Single static binary** | `CGO_ENABLED=0`, runs on `scratch` as non-root UID 10001 |
| **Pluggable storage** | local filesystem, S3 (Minio / DO Spaces), Google Drive, Storj |
| **Server-side encryption** | OpenPGP AES-256 via `X-Encrypt-Password` header |
| **Client-friendly** | Works with `curl`, `wget`, HTTPie, PowerShell, any HTTP client |
| **Auto-expiry** | Per-file `Max-Days` / `Max-Downloads` headers + scheduled purge |
| **Virus scanning** | Optional ClamAV prescan and VirusTotal submission |
| **TLS** | Bring-your-own cert, or automatic via Let's Encrypt |
| **Auth** | HTTP Basic, htpasswd, IP allow/deny lists |
| **Hardened** | Strict CSP, COOP/CORP, HSTS, slow-loris timeouts, constant-time auth compare, per-IP rate limiting |
| **Graceful shutdown** | SIGINT/SIGTERM → `Shutdown(ctx)` → in-flight uploads complete |
| **Modern web UI** | Astro 5 + React 19 + Tailwind 4, drag-and-drop, ETA, abortable upload, i18n (English / 中文 / 日本語) |

---

## Quick start

Pick whichever fits your setup. All three produce the same running service on <http://localhost:18080>.

### Docker Compose (recommended)

Requires only **Docker**.

```bash
git clone https://github.com/sooua/send.to && cd send.to
./scripts/docker.sh up
```

That's it. The wrapper script:

1. Copies `.env.example` → `.env` if missing (edit to customise).
2. Runs `docker compose up -d --build`.
3. Probes `/health.html` for up to 30 s and prints the URL once ready.

| Command                       | What it does                                      |
| ----------------------------- | ------------------------------------------------- |
| `./scripts/docker.sh up`      | Build & start in background                       |
| `./scripts/docker.sh down`    | Stop & remove containers (volume preserved)       |
| `./scripts/docker.sh purge`   | Stop & **delete the data volume** (asks first)    |
| `./scripts/docker.sh logs`    | Follow container logs                             |

The image is ≈ 52 MB, non-root, read-only root filesystem, no shell, with a built-in `HEALTHCHECK`.

### Native (no Docker)

Requires **Go 1.25+** and **Node 20+**.

<details>
<summary><b>Linux / macOS / WSL</b></summary>

```bash
./scripts/deploy.sh              # foreground, Ctrl+C for graceful shutdown
./scripts/deploy.sh --daemon     # background; logs in build/sendto.log
./scripts/deploy.sh --stop       # stop a daemonised instance
PORT=9000 ./scripts/deploy.sh    # override port
```

</details>

<details>
<summary><b>Windows (PowerShell)</b></summary>

```powershell
.\scripts\deploy.ps1                 # foreground
.\scripts\deploy.ps1 -Daemon         # background
.\scripts\deploy.ps1 -Stop           # stop
$env:PORT="9000"; .\scripts\deploy.ps1
```

</details>

Everything lives under `./build/` and `./data/` — uninstall is `rm -rf build data web/dist web/node_modules`.

### From source

```bash
make build                        # Go binary + web bundle
./send.to --provider local --basedir ./data --web-path ./web/dist
```

Or run `make dev` to get the Go server and the Astro dev server hot-reloading in parallel.

---

## Usage

### Upload

```bash
# Plain upload
curl --upload-file ./notes.md https://send.to/notes.md

# Encrypted upload (server stores ciphertext)
curl -H "X-Encrypt-Password: s3cret" \
     --upload-file ./notes.md https://send.to/notes.md

# Limit to 5 downloads, expire in 7 days
curl -H "Max-Downloads: 5" -H "Max-Days: 7" \
     --upload-file ./notes.md https://send.to/notes.md

# Multipart: many files in one request
curl -F file1=@a.txt -F file2=@b.txt https://send.to/
```

### Download

```bash
curl https://send.to/<token>/notes.md -o notes.md

# Decrypt on the fly
curl -H "X-Decrypt-Password: s3cret" \
     https://send.to/<token>/notes.md -o notes.md

# Resumable (Range requests)
curl -C - -o big.iso https://send.to/<token>/big.iso
```

### Archive download

```bash
# Combine multiple stored files into one stream
curl https://send.to/(tokenA/a.txt,tokenB/b.txt).zip -o bundle.zip
curl https://send.to/(tokenA/a.txt,tokenB/b.txt).tar.gz -o bundle.tgz
```

### Delete

The upload response includes an `X-Url-Delete` header — hit it with `DELETE`.

```bash
curl -X DELETE https://send.to/<token>/notes.md/<deletion-token>
```

### Shell helper

Drop this in your `~/.bashrc` / `~/.zshrc`:

```bash
send() { curl --progress-bar --upload-file "$1" "https://send.to/$(basename "$1")"; }
# then:  send ./report.pdf
```

---

## HTTP API

| Method | Path                               | Purpose                         |
| ------ | ---------------------------------- | ------------------------------- |
| `PUT`  | `/{filename}`                      | Upload a single file            |
| `POST` | `/`                                | Multipart upload (one or many)  |
| `GET`  | `/{token}/{filename}`              | Download or preview             |
| `HEAD` | `/{token}/{filename}`              | Metadata only                   |
| `GET`  | `/({files}).{zip,tar,tar.gz}`      | Combine files into an archive   |
| `DELETE` | `/{token}/{filename}/{delToken}` | Delete a file                   |
| `PUT`  | `/{filename}/scan`                 | ClamAV scan                     |
| `PUT`  | `/{filename}/virustotal`           | VirusTotal submission           |
| `GET`  | `/health.html`                     | Health check                    |

### Headers

| Request header         | Purpose                                             |
| ---------------------- | --------------------------------------------------- |
| `Max-Days`             | Auto-expire after N days                            |
| `Max-Downloads`        | Cap downloads at N                                  |
| `X-Encrypt-Password`   | Server encrypts payload (OpenPGP AES-256)           |
| `X-Decrypt-Password`   | Provide password on download                        |

| Response header        | Meaning                                             |
| ---------------------- | --------------------------------------------------- |
| `X-Url-Delete`         | Authenticated URL to `DELETE` this upload           |
| `X-Remaining-Days`     | Days until auto-expiry                              |
| `X-Remaining-Downloads`| Downloads remaining under the cap                   |

A live reference is rendered in the web UI at `/api-docs`.

---

## Configuration

Every CLI flag has an environment-variable equivalent (`--listener` ↔ `LISTENER`). Run `./send.to --help` for the full matrix. The most relevant knobs:

| Flag / Env                        | Default     | Notes                                         |
| --------------------------------- | ----------- | --------------------------------------------- |
| `--listener` / `LISTENER`         | `:18080`     | Plain HTTP bind address                       |
| `--tls-listener` / `TLS_LISTENER` | empty       | Enable native HTTPS                           |
| `--provider` / `PROVIDER`         | —           | `local` \| `s3` \| `gdrive` \| `storj`        |
| `--basedir` / `BASEDIR`           | —           | Storage root for the `local` provider         |
| `--max-upload-size`               | `0`         | KB per upload; `0` = unlimited                |
| `--rate-limit`                    | `0`         | Requests / minute / IP (PUT/POST/GET)         |
| `--purge-days`                    | `0`         | Delete uploads older than N days              |
| `--shutdown-timeout`              | `30s`       | Grace period for in-flight requests on quit   |
| `--http-auth-user` / `_pass`      | empty       | HTTP Basic Auth credentials                   |
| `--cors-domains`                  | empty       | Comma-separated Allow-Origin list             |
| `--clamav-host`                   | empty       | e.g. `tcp://clamav:3310`                      |
| `--virustotal-key`                | empty       | Enables `/{file}/virustotal`                  |
| `--lets-encrypt-hosts`            | empty       | Comma-separated hostnames for ACME            |

The [`docker-compose.yml`](./docker-compose.yml) + [`.env.example`](./.env.example) combo documents everything you need for container deployments.

---

## Deployment

### TLS / HTTPS

Two production patterns:

**1. Reverse proxy (most common)** — keep `send.to` on plain HTTP and let Caddy / Nginx / Traefik / Cloudflare terminate TLS:

```caddy
files.example.com {
    reverse_proxy 127.0.0.1:18080
}
```

Make sure the proxy forwards `X-Forwarded-Proto: https` so the HSTS / CSP headers kick in.

**2. Native TLS**

```bash
./send.to --tls-listener :8443 \
  --tls-cert-file fullchain.pem --tls-private-key privkey.pem
```

Or automatic Let's Encrypt:

```bash
./send.to --lets-encrypt-hosts files.example.com
```

### Production checklist

- [ ] Set `MAX_UPLOAD_SIZE` to a sane value — protects disk / S3 bill.
- [ ] Set `RATE_LIMIT` — protects against a single abusive IP.
- [ ] Always front with HTTPS (browsers refuse `clipboard.writeText` on insecure origins).
- [ ] Enable Basic Auth on instances that must not accept anonymous uploads.
- [ ] Mount the storage volume on a partition with a quota.
- [ ] Monitor the `HEALTHCHECK` status (`docker ps`) or poll `/health.html`.

### Updating

```bash
git pull
./scripts/docker.sh up            # rebuilds & rolls the container
# or:
./scripts/deploy.sh --stop && ./scripts/deploy.sh --daemon
```

Graceful shutdown is bounded by `SHUTDOWN_TIMEOUT`, so in-flight uploads up to that deadline complete before the old instance exits.

---

## Architecture

```
┌──────────────┐       HTTPS        ┌────────────────┐
│   Browser /  │ ─────────────────▶ │  Reverse Proxy │
│   curl /     │                    │   (optional)   │
│   HTTPie     │ ◀───────────────── │                │
└──────────────┘                    └────────┬───────┘
                                             │ HTTP
                                             ▼
                                    ┌────────────────┐
                                    │   send.to      │
                                    │   Go binary    │
                                    │                │
                                    │ • mux router   │
                                    │ • rate limit   │
                                    │ • CSP / HSTS   │
                                    │ • OpenPGP enc  │
                                    │ • ClamAV /     │
                                    │   VirusTotal   │
                                    └────────┬───────┘
                                             │
                  ┌──────────────┬───────────┴───────────┬──────────────┐
                  ▼              ▼                       ▼              ▼
             local FS         S3 / Minio           Google Drive      Storj
```

### Source layout

```
.
├── cmd/                CLI flag parsing (urfave/cli)
├── server/             HTTP handlers, auth, security, storage backends
│   └── storage/        local | s3 | gdrive | storj
├── internal/clamd/     Embedded ClamAV daemon client
├── web/                Astro 5 + React 19 frontend
├── scripts/            deploy.sh · deploy.ps1 · docker.sh  (one-click ops)
├── test/               End-to-end smoke test + Windows signal helper
├── Dockerfile          3-stage build: web bundle → Go binary → scratch
├── docker-compose.yml  Production-ready compose (healthcheck, read-only FS)
├── Makefile            build · test · lint · vuln · smoke · docker
└── .github/workflows/  CI: Go test+race+coverage, lint, govulncheck, web build
```

---

## Benchmarks

Approximate numbers from a single-core laptop run (Windows 11, WSL2 off, local filesystem backend, empty pre-warmed cache):

| Metric                        | Value              |
| ----------------------------- | ------------------ |
| Cold-start time               | < 50 ms            |
| Image size                    | 51.8 MB            |
| RSS at idle                   | ~ 20 MB            |
| Upload throughput (local FS)  | line-rate (~ disk) |
| Concurrent uploads (10×)      | all succeed, zero errors, distinct tokens |
| Graceful shutdown overhead    | bounded by `SHUTDOWN_TIMEOUT` (default 30 s) |

A self-contained smoke test exercising 22 assertions (health, security headers, round-trip, encryption, limits, concurrency, graceful shutdown) is in [`test/smoke/`](./test/smoke/). Run it locally with `make smoke`.

---

## FAQ

<details>
<summary><b>Is there a size limit?</b></summary>

No hard-coded limit. Set `--max-upload-size` (KB) to bound uploads. With no limit the only cap is disk space on the storage backend.
</details>

<details>
<summary><b>How long are files kept?</b></summary>

Clients choose per upload via the `Max-Days` and `Max-Downloads` headers. Operators set a server-wide floor with `--purge-days`. Reach the cap → file is deleted on next access or next purge run.
</details>

<details>
<summary><b>Are files encrypted at rest?</b></summary>

Only if the uploader sends `X-Encrypt-Password` (OpenPGP AES-256). Otherwise files are stored as-is. For S3 / Storj backends, use their server-side encryption features in addition.
</details>

<details>
<summary><b>Can I run it behind a reverse proxy on a sub-path?</b></summary>

Yes. Set `--proxy-path /send` (or `PROXY_PATH=/send`) and point the proxy at the container. All URLs in responses will be rewritten accordingly.
</details>

<details>
<summary><b>How do I back up uploads?</b></summary>

For `local` provider: everything is in `--basedir`. Snapshot that directory or use filesystem-level tools (`rsync`, ZFS snapshots, etc.). For S3 / Storj, use the backend's replication / snapshot features.
</details>

<details>
<summary><b>Does it work offline / on an air-gapped network?</b></summary>

Yes. The binary ships with no runtime deps, the image has none, and the web UI is a fully static Astro build. Only use of the network is outbound to the configured storage backend (and optional ClamAV / VirusTotal endpoints).
</details>

---

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for dev setup, build / test commands, and PR conventions.

For **security issues**, please follow [SECURITY.md](./SECURITY.md) — don't open public issues for vulnerabilities.

Third-party license attributions are consolidated in [THIRD_PARTY_LICENSES.md](./THIRD_PARTY_LICENSES.md).

---

## License

[MIT](./LICENSE)
