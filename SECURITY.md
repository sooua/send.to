# Security Policy

## Supported Versions

Only the most recent release on `main` receives security fixes. Older
tagged releases are not patched — please upgrade.

| Version       | Status            |
| ------------- | ----------------- |
| `main` (HEAD) | Actively patched  |
| Older tags    | Best-effort only  |

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security reports.**

Email the maintainers privately at **xivion7@gmail.com** with:

- A description of the issue and its impact
- Reproduction steps or a proof-of-concept
- The affected version / commit SHA
- Any relevant configuration (storage backend, deployment shape)

You should expect:

- An acknowledgement within **5 working days**
- A triage decision (accept / decline / need-more-info) within **14 days**
- A fix or mitigation timeline once triaged
- Public disclosure coordinated with you, typically after a fix is shipped

If the report is accepted, you will be credited in the release notes
unless you prefer to remain anonymous.

## Scope

In scope:

- The Go server (`./server`, `./cmd`, root `main.go`)
- The Astro/React web client (`./web`)
- The Dockerfile and default deployment configuration
- Authentication (basic auth, htpasswd, IP filtering)
- File upload, download, encryption, and deletion flows
- Built-in rate limiting and security headers

Out of scope:

- Vulnerabilities in third-party storage providers (S3, Storj, GDrive)
  — report to the upstream vendor
- DoS via raw resource exhaustion when no `--max-upload-size`,
  `--rate-limit`, or `IPFilter` is configured (these are operator
  responsibilities)
- Issues only reproducible against an outdated commit
- Self-XSS or social-engineering attacks against the operator

## Hardening Recommendations for Operators

If you self-host send.to, please:

1. Run behind HTTPS (`--tls-listener` or terminate at a reverse proxy
   that sets `X-Forwarded-Proto: https`).
2. Configure `--max-upload-size` and `--rate-limit` to bound resource
   usage.
3. Enable basic auth (`--basic-auth-user`/`--basic-auth-pass` or
   `--http-auth-htpasswd`) on PUT/POST routes for non-public instances.
4. Set `--ip-whitelist`/`--ip-blacklist` to restrict access where
   appropriate.
5. Keep the binary updated — pull the latest tagged image regularly.
6. Run as a non-root user (the official Docker image already does, UID
   `10001`).
7. Mount the storage backend on a separate volume with an enforced
   quota.
