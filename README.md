[![Sponsor on GitHub](https://img.shields.io/badge/Sponsor%20on%20GitHub-EA4AAA?style=for-the-badge&logo=github&logoColor=white)](https://github.com/sponsors/vineetchoudhary)
[![Buy me a coffee](https://img.shields.io/badge/Buy%20me%20a%20coffee-FFDD00?style=for-the-badge&logo=buy-me-a-coffee&logoColor=black)](https://buymeacoffee.com/vineetchoudhary)
[![Build status](https://img.shields.io/github/actions/workflow/status/getappbox/install-helper/docker-image.yml?branch=main&style=for-the-badge&logo=githubactions&logoColor=white&label=build)](https://github.com/getappbox/install-helper/actions/workflows/docker-image.yml)
[![Docker image](https://img.shields.io/badge/ghcr.io-install--helper-2496ED?style=for-the-badge&logo=docker&logoColor=white)](https://github.com/getappbox/install-helper/pkgs/container/install-helper)

# AppBox Install Helper

The AppBox backend

| Hostname | Role | Routes |
|---|---|---|
| `api.getappbox.com` | AppBox client API | `/api/v1/*` |
| `install.getappbox.com` | Install / proxy layer | `/cors`, `/install/**`, `/appinfo/**` |

**Contents** — [Quick start](#quick-start) · [Configuration](#configuration) ·
[API reference](#api-reference) · [Deployment](#deployment) · [Operations](#operations) · [Support](#support)

---

## Quick start

Every method needs a `.env` file first:

```bash
cp .env.example .env
```

Fill in the values (see [Configuration](#configuration)).

### Option 1 — Docker Compose

```bash
docker compose build
```

```bash
docker compose up -d app
```

```bash
curl http://localhost:2551/
```

### Option 2 — Docker run

```bash
docker pull ghcr.io/getappbox/install-helper:latest
```

```bash
docker run -d --name install-helper --env-file .env -p 8080:8080 --restart unless-stopped ghcr.io/getappbox/install-helper:latest
```

Pin a specific version in production rather than tracking `latest`:

```bash
docker run -d --name install-helper --env-file .env -p 8080:8080 --restart unless-stopped ghcr.io/getappbox/install-helper:1.0.2
```

### Option 3 — Build the image directly

```bash
docker build -t install-helper:latest .
```

```bash
docker run -d --name install-helper --env-file .env -p 8080:8080 install-helper:latest
```

The image is a two-stage build (Swift 6.1.3 builder → `ubuntu:jammy` runtime, statically linked stdlib), runs as the non-root `vapor` user, sets `MALLOC_ARENA_MAX=2`, and ships a `HEALTHCHECK` that probes TCP `8080`:

```bash
docker inspect --format '{{.State.Health.Status}}' install-helper
```

### systemd (native install)

Install the unit file at `/etc/systemd/system/installhelper.service` (`installhelper.service`
in this repo). It loads secrets from the `.env` and sets `MALLOC_ARENA_MAX=2`.

```bash
sudo systemctl daemon-reload && sudo systemctl enable --now installhelper
```

### Updating

**Docker Compose** (builds locally, so rebuild rather than pull):

```bash
git pull && docker compose build && docker compose up -d app
```

**Prebuilt GHCR image:**

```bash
docker pull ghcr.io/getappbox/install-helper:latest && docker compose up -d app
```

**Native** — do not run as root:

```bash
sudo su vapor
```

```bash
cd install-helper && git fetch && git pull
```

```bash
swift build -c release
```

```bash
sudo service installhelper restart
```

---

## Configuration

All configuration is environment variables, loaded from `.env` in development and passed
through the environment by Compose/systemd in production. `.env.example` is the annotated
reference; the essentials:

| Variable | Purpose | Default |
|---|---|---|
| `PORT` | HTTP listen port | `8080` (fallback) |
| `APPBOX_CLIENT_TOKEN` | Shared secret for `X-AppBox-Client-Token` | — (required) |
| `DROPBOX_APP_KEY` | Public OAuth client_id served by `/api/v1/config` | — |
| `DROPBOX_TOKEN_CACHE_TTL_SECONDS` | How long a verified Dropbox token stays cached | `600` |
| `MAILGUN_API_KEY` / `MAILGUN_DOMAIN` / `MAILGUN_FROM` | Build-email delivery | — |
| `YOURLS_API_URL` / `YOURLS_SIGNATURE_SECRET` | Short-link backend | — |
| `SHORTLINK_TARGET_BASE` | Installer page the short link points at | — |
| `CORS_PROXY_ALLOWED_HOSTS` | Host suffixes `/cors?url=` may proxy to | `dropbox.com,dropboxusercontent.com,getappbox.com` |
| `PROXY_MAX_BODY_BYTES` | Max bytes buffered from one proxied response | `4194304` (4 MiB) |
| `GITHUB_LATEST_RELEASE_URL` / `HOMEBREW_CASK_URL` | Update-check upstreams | — |
| `UPDATE_CACHE_TTL_SECONDS` | Update-check cache lifetime | `3600` |
| `NOTIFY_ALLOWED_HOSTS` | Host suffixes `/api/v1/notify` may POST to | Slack + Teams hosts |
| `TRUSTED_PROXY_COUNT` | Reverse proxies appending to `X-Forwarded-For` | `1` |

A missing `/api/v1` secret is logged at boot but is **not** fatal — the install/appinfo/cors
routes keep serving, and only the affected `/api/v1` endpoint errors (`500 misconfigured`)
per request.

---

## API reference

### Conventions

**Authentication.** Every `/api/v1` route requires the static client token header. Other endpoints additionally require the caller's Dropbox access token as `Authorization: Bearer <token>`, server verifies it against Dropbox and caches the verdict by token digest for `DROPBOX_TOKEN_CACHE_TTL_SECONDS`. Only real, logged-in AppBox users can use the short links, send mail, or fire notifications.

**Errors.** All `/api/v1` failures return the same envelope:

```json
{ "error": { "code": "snake_case_code", "message": "Human readable detail." } }
```

| Code | Status | Meaning |
|---|---|---|
| `invalid_client_token` | 401 | Missing/incorrect `X-AppBox-Client-Token` |
| `missing_dropbox_token` / `invalid_dropbox_token` | 401 | Bearer token absent or rejected by Dropbox |
| `invalid_body` | 400 | Body did not decode into the expected shape |
| `invalid_recipients` | 400 | Not 1–100 valid email addresses |
| `invalid_service` | 400 | `service` was not `slack` or `teams` |
| `invalid_url` | 400 | Malformed URL in the request body |
| `webhook_not_allowed` | 403 | Webhook host not on `NOTIFY_ALLOWED_HOSTS` |
| `rate_limited` | 429 | Bucket exhausted; see `Retry-After` |
| `misconfigured` | 500 | A required env var is unset |
| `verification_unavailable` | 503 | Dropbox verification outage (fails closed, uncached) |
| `shortener_unavailable` / `mail_provider_error` / `notify_failed` / `update_check_unavailable` | 502 | Upstream provider failed |

**Rate limits.** Fixed window, per client IP, in-memory (single instance). Exceeding a bucket returns `429` with a `Retry-After` header.

| Bucket | Limit |
|---|---|
| `/api/v1/config` | 120 / min |
| `/api/v1/latest-version` | 120 / min |
| `/api/v1/shorten` | 60 / min |
| `/api/v1/mail/send` | 20 / min |
| `/api/v1/notify` | 60 / min |
| Install / appinfo / cors routes | 300 / min |

---

## Operations

### Memory bounds

1. **`/cors` host allowlist** — bounds the HTTP client's per-host connection pools
2. **`PROXY_MAX_BODY_BYTES`** — caps proxied upstream bodies (default 4 MiB).
3. **Self-sweeping token cache** — Vapor's memory cache never sweeps expired-but-unread keys,
   so Dropbox token verdicts use a bounded cache that does.
4. **`MALLOC_ARENA_MAX=2`** — set in both the Docker image and `installhelper.service`; glibc arena fragmentation otherwise ratchets RSS on multi-core Linux. If you run the binary outside those two paths, export `MALLOC_ARENA_MAX=2` yourself.

### S3 publication links

`POST /api/v1/shorten` additionally accepts `provider: "s3"` with `url` set to the complete install-page URL (`https://web.getappbox.com/?provider=s3&url=<encoded public HTTPS appinfo.json URL>`). The outer page must match `SHORTLINK_TARGET_BASE`; the inner metadata may use an AWS, Spaces or custom public domain. Unknown providers, duplicate parameters and credential-bearing/non-HTTPS sources are rejected before shortening. Requests still require the client token and the publisher's Dropbox bearer token. Omitting `provider` retains the existing Dropbox request and target behavior. The service does not fetch S3 metadata, manifests or IPAs; the install page and SDK download them anonymously from storage. Existing Dropbox proxy routes and rate limits are unchanged. Deploy matching readers before enabling S3 publishing.

S3 responses additionally include `provider: "s3"`. Updated publishers require that confirmation before accepting a short URL, so an older service that ignores the request's provider cannot accidentally return a Dropbox target for an S3 link. Dropbox responses retain their existing shape.

---

## Support

If AppBox has been useful to you, consider supporting its continued development.

<a href="https://github.com/sponsors/vineetchoudhary">
  <img src="https://img.shields.io/badge/Sponsor%20on%20GitHub-EA4AAA?style=for-the-badge&logo=github&logoColor=white" alt="Sponsor on GitHub" height="50">
</a>
&nbsp;
<a href="https://buymeacoffee.com/vineetchoudhary">
  <img src="https://img.shields.io/badge/Buy%20me%20a%20coffee-FFDD00?style=for-the-badge&logo=buy-me-a-coffee&logoColor=black" alt="Buy Me a Coffee" height="50">
</a>

Thank you for supporting open source! 🙏
