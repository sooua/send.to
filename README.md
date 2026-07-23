<div align="center">

# send.to

**Easy and fast file sharing from the command line — and a clean web UI when you need it.**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://golang.org/)
[![Node](https://img.shields.io/badge/Node-22%2B-339933?logo=node.js)](https://nodejs.org/)
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
- [The `send` CLI](#the-send-cli)
- [HTTP API](#http-api)
- [Configuration](#configuration)
- [Deployment](#deployment)
- [Architecture](#architecture)
- [Benchmarks](#benchmarks)
- [FAQ](#faq)
- [Examples](./examples.md)
- [Contributing](#contributing)
- [License](#license)

---

## Features

| | |
|---|---|
| **Single static binary** | `CGO_ENABLED=0`, runs on `scratch` as non-root UID 10001 |
| **Pluggable storage** | local filesystem, S3 (Minio / DO Spaces), Google Drive, Storj |
| **End-to-end encryption** | AES-256-GCM on the client; the key rides in the URL fragment and never reaches the server |
| **Server-side encryption** | OpenPGP AES-256 via `X-Encrypt-Password` header |
| **Client-friendly** | Works with `curl`, `wget`, HTTPie, PowerShell, any HTTP client |
| **Resumable uploads** | Chunked sessions: a 5 GB transfer that dies at 90% continues where it stopped |
| **Collections** | One link for several files, with a landing page and a download-all archive |
| **Accountless history** | An owner token lists and deletes your uploads from any machine |
| **Auto-expiry** | Per-file `Max-Days` / `Max-Downloads` headers + scheduled purge |
| **Virus scanning** | Optional ClamAV prescan and VirusTotal submission |
| **TLS** | Bring-your-own cert, or automatic via Let's Encrypt |
| **Auth** | HTTP Basic, htpasswd, IP allow/deny lists |
| **Hardened** | Strict CSP, COOP/CORP, HSTS, slow-loris timeouts, constant-time auth compare, per-IP rate limiting |
| **Graceful shutdown** | SIGINT/SIGTERM → `Shutdown(ctx)` → in-flight uploads complete |
| **JSON API** | `Accept: application/json` returns url, delete url, size, expiry |
| **Observability** | `/health` (JSON) and `/metrics` (Prometheus text format) |
| **Modern web UI** | Astro 5 + React 19 + Tailwind 4: multi-file queue, paste-to-upload, per-file ETA, retry, expiry / download-limit / password controls, QR codes, and a local upload history that keeps your delete links |

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

Requires **Go 1.25+** and **Node 22+**.

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

More recipes — fish/PowerShell aliases, encrypted database backups, CI snippets — are in [examples.md](./examples.md).

### Shell helper

Drop this in your `~/.bashrc` / `~/.zshrc`:

```bash
send() { curl --progress-bar --upload-file "$1" "https://send.to/$(basename "$1")"; }
# then:  send ./report.pdf
```

---

## The `send` CLI

A shell alias gets you a URL. The client gets you a URL you can find again.

```bash
send config add home https://send.to --default     # once
send put report.pdf                                # every day after
```

```
send put ./build.tar.gz --days 7 --max-downloads 3
send get https://send.to/aB3cD4eF/build.tar.gz     # resumes if interrupted
send info https://send.to/aB3cD4eF/build.tar.gz    # limits, without spending one
send ls                                            # what this machine uploaded
send rm https://send.to/aB3cD4eF/build.tar.gz      # uses the stored delete link
```

| Command | What it does |
| ------- | ------------ |
| `send put <file>...` | Upload; `-` reads stdin (`--name` required) |
| `send get <url>` | Download, resuming a partial file automatically |
| `send info <url>` | Size and remaining limits — `HEAD`, so it costs no download |
| `send ls` | Local upload history: links, expiry, delete links |
| `send rm <url>` | Delete, using the deletion link recorded at upload time |
| `send config` | Named server profiles, `--default`, basic auth |

Flags work wherever you put them: `send put a.txt --days 7` does what it looks
like, rather than silently uploading with no expiry.

The history lives in `send config path` (`~/.config/sendto` on Linux). Losing
a link no longer means losing the ability to delete the file — the one thing a
`curl` alias can never do for you.

`SENDTO_URL`, `SENDTO_USER` and `SENDTO_PASS` override the config file, so CI
needs no state on disk.

### End-to-end encryption

`--e2e` encrypts on your machine before anything is sent. The key is generated
locally and travels in the link's **fragment** — the part after `#`, which
browsers, proxies and access logs never transmit. The server receives
ciphertext and has no way to read it.

```bash
send put contract.pdf --e2e
# https://send.to/aB3cD4eF/contract.pdf#k=9qbxPpQRFJ0xmdYqZ4qh73BppXc3UzK26wyvyXPQxFs
#                                       └── the key. Lose it and the file is gone.

send get 'https://send.to/aB3cD4eF/contract.pdf#k=9qbx…'   # decrypts locally
```

The web UI offers the same thing as a checkbox, and a recipient opening the
link in a browser decrypts it there — the key never leaves the tab.

AES-256-GCM in 64 KiB chunks, with the chunk counter and a final-chunk marker
folded into each nonce, so reordering or truncating the stream is an
authentication failure rather than a silently shorter file. The Go and browser
implementations are checked against each other by
[`test/e2e-interop/run.sh`](./test/e2e-interop/run.sh).

This is not the same as `--encrypt` / `X-Encrypt-Password`, which is
server-side: convenient, works with plain `curl`, but the server sees the
plaintext on the way through. Use `--e2e` when the server itself is not
trusted.

**There is no recovery.** Nobody — not the operator, not you — can decrypt a
file whose fragment was lost.

---

## HTTP API

| Method | Path                               | Purpose                         |
| ------ | ---------------------------------- | ------------------------------- |
| `PUT`  | `/{filename}`                      | Upload a single file            |
| `POST` | `/`                                | Multipart upload (one or many)  |
| `POST` | `/upload/{filename}`               | Start a resumable upload        |
| `PATCH`| `/upload/{id}/{filename}`          | Send one chunk of a resumable upload |
| `HEAD` | `/upload/{id}/{filename}`          | How many bytes the server holds |
| `DELETE`| `/upload/{id}/{filename}`         | Abandon a resumable upload      |
| `GET`  | `/{token}/{filename}`              | Download or preview             |
| `HEAD` | `/{token}/{filename}`              | Metadata only                   |
| `POST` | `/collection`                      | Group existing uploads behind one link |
| `GET`  | `/c/{token}`                       | A collection: page, JSON, or one URL per line |
| `GET`  | `/c/{token}.{zip,tar,tar.gz}`      | The whole collection as one archive |
| `GET`  | `/({files}).{zip,tar,tar.gz}`      | Combine files into an archive   |
| `DELETE` | `/{token}/{filename}/{delToken}` | Delete a file                   |
| `PUT`  | `/{filename}/scan`                 | ClamAV scan (auth + rate limited) |
| `PUT`  | `/{filename}/virustotal`           | VirusTotal submission (auth + rate limited) |
| `GET`  | `/owner/files`                     | List this owner token's uploads |
| `GET`  | `/health` · `/health.html`         | Health check (JSON on `Accept: application/json`) |
| `GET`  | `/metrics`                         | Prometheus counters             |
| `GET`  | `/qr?url=`                         | QR code PNG for a share link on this host |

### Headers

| Request header         | Purpose                                             |
| ---------------------- | --------------------------------------------------- |
| `Max-Days`             | Auto-expire after N days                            |
| `Max-Downloads`        | Cap downloads at N                                  |
| `X-Owner-Token`        | Record the upload in this owner's server-side list  |
| `X-Encrypt-Password`   | Server encrypts payload (OpenPGP AES-256)           |
| `X-Decrypt-Password`   | Provide password on download                        |

| Response header        | Meaning                                             |
| ---------------------- | --------------------------------------------------- |
| `X-Url-Delete`         | Authenticated URL to `DELETE` this upload           |
| `X-Remaining-Days`     | Days until auto-expiry                              |
| `X-Remaining-Downloads`| Downloads remaining under the cap                   |

### Resumable uploads

A `PUT` is all or nothing: lose the link at 90% of a 5 GB artefact and all of it
is gone. A resumable upload is a session plus chunks, and a chunk is the only
thing a failure can cost.

```bash
size=$(stat -c%s build.tar.gz)

# Open a session; the URL comes back in Location.
session=$(curl -sS -D- -o /dev/null -X POST     -H "Upload-Length: $size" -H "Max-Days: 7"     https://send.to/upload/build.tar.gz | awk '/^[Ll]ocation:/{print $2}' | tr -d '')

# Send a chunk. 204 means "keep going", and Upload-Offset says from where.
curl -sS -X PATCH -H "Content-Range: bytes 0-8388607/$size"     --data-binary @chunk-0 "$session"

# Ask where to resume after anything goes wrong.
curl -sS -I "$session" | grep -i upload-offset

# The chunk that completes the file answers exactly like a plain PUT:
# the share URL, with the delete link in X-Url-Delete.
curl -sS -X PATCH -H "Content-Range: bytes 8388608-$((size - 1))/$size"     --data-binary @chunk-1 "$session"
```

Rules worth knowing:

- A chunk is **all or nothing**. A body that arrives short is discarded and the
  offset stays where it was, so the offset you resume from is always one you
  chose.
- Sending the wrong offset answers `409 Conflict` with `Upload-Offset` set to
  the right one, rather than corrupting the file.
- Bytes are spooled under `TEMP_PATH` and only reach the storage backend when
  the upload completes — an interrupted upload is never a visible half file.
- Sessions expire after 24 hours and take their spool file with them.
- `X-Encrypt-Password` is rejected here: the server would have to keep it in
  clear between chunks. Encrypt on the client instead (`send put --e2e`).

`send put` uses this automatically for files of 16 MiB and up, records the
session, and continues it on the next run — including for `--e2e`, where the
ciphertext is regenerated from the offset the server stopped at:

```
$ send put build.tar.gz
^C
$ send put build.tar.gz
resuming at 1.4 GB of 5.0 GB
```

Against a server without these endpoints the client falls back to a plain `PUT`.

### Collections

Uploading five log files gives you five links, and the fifth one is the one that
gets lost in the chat. A collection is one link that lists them all:

```bash
send put ./*.log --collection --collection-name "nightly logs"
# https://send.to/c/aB3cD4eF
```

Opening it in a browser shows the file list with a "download all" button;
`curl` gets one share URL per line, and `Accept: application/json` gets the
whole record. Everything at once is the same link with an extension:

```bash
curl -sL https://send.to/c/aB3cD4eF.zip -o logs.zip     # .tar and .tar.gz too
curl -s https://send.to/c/aB3cD4eF | xargs -n1 curl -O  # or fetch them one by one
```

A collection can also be assembled from uploads that already exist, which is
what the CLI does under the hood:

```bash
curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"nightly logs","files":["https://send.to/aB3/app.log","https://send.to/cD4/worker.log"]}' \
    https://send.to/collection
```

It owns no bytes of its own:

- Deleting the collection leaves the files alone — somebody may hold one of
  their links directly. Each file keeps its own delete link and its own limits.
- A member that expires, runs out of downloads or is deleted simply drops off
  the list, and a collection with nothing left answers 404.
- Downloading the archive charges each member one download, exactly as fetching
  them individually would.
- At most 100 files per collection.
- `--collection` and `--e2e` do not combine: each file has its own key, and one
  link cannot carry several of them without handing the server the means to
  read the files.

### Server-side upload history

The `send` CLI keeps a local record of what it uploaded, which is no help at all
when the machine that uploaded it was a CI runner that no longer exists — the
delete links went with it.

Send an owner token and the server keeps the list instead:

```bash
curl -H "X-Owner-Token: $SENDTO_OWNER_TOKEN" --upload-file build.log https://send.to/build.log

curl -H "X-Owner-Token: $SENDTO_OWNER_TOKEN" https://send.to/owner/files
# one share URL per line, or the full records with Accept: application/json
```

There is still no account:

- The server stores **sha256 of the token** and never the token itself. It is an
  index name, not a credential it could hand to anyone else.
- Holding the token is the whole of the authorisation, so treat it as a
  password: it lists share links and delete links for every upload made with it.
- Entries disappear when their upload does — deleted, expired, or out of
  downloads — and the list prunes itself when it is read.
- Uploads made without the header stay anonymous, exactly as before.
- At most 200 entries per token are kept; the oldest fall off.

`send` derives one token per server from a master secret in its config
directory (`owner.key`, mode 0600), so a server you upload to learns nothing it
could replay against a different one:

```bash
send put build.log            # the token rides along
send ls --remote              # what this owner has on the server, from any machine
send rm https://send.to/aB3cD4eF/build.log   # uses the delete link the server kept
```

Set `SENDTO_OWNER_TOKEN` to share one identity across machines — several CI
runners publishing into the same list, for instance.

### JSON responses

Send `Accept: application/json` on an upload to get structured output instead of
a bare URL — no more scraping the `X-Url-Delete` header:

```bash
curl -H "Accept: application/json" -H "Max-Days: 7"      --upload-file notes.md https://send.to/notes.md
```

```json
{
  "url": "https://send.to/aB3cD4eF/notes.md",
  "delete_url": "https://send.to/aB3cD4eF/notes.md/9xK…",
  "filename": "notes.md",
  "size": 4096,
  "content_type": "text/x-markdown",
  "encrypted": false,
  "expires_at": "2026-07-30T08:06:49Z"
}
```

A multipart `POST` returns `{"files": [ … ]}` with one object per file.

### Download accounting

`Max-Downloads` counts *completed* transfers only:

- **Range requests never count.** `curl -C -`, video scrubbing and byte-range
  probes can resume a file freely without eating the budget.
- A transfer that fails or is aborted mid-body does not count.
- When the budget or `Max-Days` runs out the object is **deleted from storage**
  immediately, rather than waiting for `--purge-days`.

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
| `--max-storage-size`              | `0`         | KB an instance will hold in total; `0` = unlimited |
| `--max-temp-size`                 | `0`         | KB of spool space for uploads in progress; `0` = unlimited |
| `--rate-limit`                    | `0`         | Requests / minute / IP, shared across all routes |
| `--temp-path` / `TEMP_PATH`       | system temp | Spool dir for uploads; must be disk-backed    |
| `--purge-days`                    | `0`         | Delete uploads older than N days              |
| `--shutdown-timeout`              | `30s`       | Grace period for in-flight requests on quit   |
| `--http-auth-user` / `_pass`      | empty       | HTTP Basic Auth credentials                   |
| `--cors-domains`                  | empty       | Comma-separated Allow-Origin list             |
| `--clamav-host`                   | empty       | e.g. `tcp://clamav:3310`                      |
| `--virustotal-key`                | empty       | Enables `/{file}/virustotal`                  |
| `--lets-encrypt-hosts`            | empty       | Comma-separated hostnames for ACME            |

### Limits worth setting on a public instance

`MAX_UPLOAD_SIZE` bounds one upload, which is no bound at all: the same
permitted file uploaded often enough fills the disk. Two totals close that.

| | |
|---|---|
| `MAX_STORAGE_SIZE` | Total bytes (KB) the storage backend may hold |
| `MAX_TEMP_SIZE` | Total bytes (KB) of spool space for uploads in progress |

Both answer **507 Insufficient Storage** rather than failing halfway. A
resumable upload is checked twice — once against its declared total when the
session is opened, so a doomed transfer is refused before a byte moves, and
again on each chunk, because several sessions can be opened before any of them
fills anything.

`MAX_TEMP_SIZE` is the one that matters against abuse: an unfinished session is
kept for 24 hours by design, so without it anyone can open sessions, send almost
all of each, and leave the bytes sitting on `TEMP_PATH` — which in the shipped
compose file is the data volume.

Two properties to know before relying on them:

- **`MAX_STORAGE_SIZE` is a counter, not a measurement.** It is seeded from the
  backend at startup and then tracked in memory, because measuring means listing
  the bucket. It drifts if files are removed behind the server's back, and a
  restart re-seeds it. `local` and `s3` can be counted; `gdrive` and `storj`
  cannot, and an instance configured with a limit on those backends **refuses to
  start** rather than pretend a limit is in force.
- **Both are per process.** Several replicas behind a load balancer each enforce
  their own total. Resumable uploads are single-instance for the same reason:
  the session spool lives on local disk, so a session opened on one replica
  cannot be continued on another. Use one instance, or a sticky-session
  balancer with a shared spool volume.

`/metrics` exposes `sendto_storage_used_bytes`, `sendto_storage_limit_bytes`,
`sendto_temp_used_bytes` and `sendto_temp_limit_bytes`, which is what to alert
on — a 507 is the point at which it is already too late.

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
- [ ] Monitor the `HEALTHCHECK` status (`docker ps`) or poll `/health`.
- [ ] Scrape `/metrics` (uploads, downloads, bytes, 429s, expired purges).
- [ ] Keep `TEMP_PATH` on a disk-backed path — uploads without a
      `Content-Length`, and every upload when the ClamAV prescan is on, are
      spooled there first. The compose file points it at the data volume
      because the container's `/tmp` is a small tmpfs.

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
