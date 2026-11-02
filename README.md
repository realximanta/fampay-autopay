<div align="center">

# 💳 fampay-autopay

**Automated UPI payment verification for FamApp using a Go backend + Cloudflare email worker.**

[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Cloudflare Workers](https://img.shields.io/badge/Cloudflare-Workers-F38020?logo=cloudflare&logoColor=white)](https://workers.cloudflare.com/)
[![Cloudflare KV](https://img.shields.io/badge/Storage-Cloudflare%20KV-F38020?logo=cloudflare&logoColor=white)](https://developers.cloudflare.com/kv/)
[![OAuth 2.0](https://img.shields.io/badge/OAuth%202.0-Google-4285F4?logo=google&logoColor=white)](https://developers.google.com/identity/protocols/oauth2)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

[![Live Demo Guide](https://img.shields.io/badge/Live%20Demo-Integration%20Guide-3b82f6?style=for-the-badge&logo=readthedocs&logoColor=white)](./auto-payment-fixed-author-developer-ximanta.md)
[![Client Docs](https://img.shields.io/badge/Client%20Docs-Integration%20Rules-brightgreen?style=for-the-badge&logo=markdown&logoColor=white)](./auto-payment-fixed-author-developer-ximanta.md)

</div>

---

## Overview

`fampay-autopay` is the current production architecture for handling FamApp payment verification without a gateway, webhook, or manual reconciliation.

The system now consists of:

- `backend/` — a Go web app for OAuth sign-in, dashboard, QR generation, verification, admin tools, and user management
- `cloudflare-worker/` — a Cloudflare Worker that listens to incoming Gmail/FamApp emails, validates SPF/DKIM/DMARC, extracts the payment amount + UTR, and saves signed payment records to KV
- `Cloudflare KV` — storage for payment data, API keys, sessions, and user metadata

This is the latest architecture and replaces the older split `verify-api` + `qr-api` layout.

---

## How it works

```
┌────────────────────┐
│ User / Bot / App  │
└─────────┬──────────┘
          │
          │ 1. Login / API key / QR flow
          ▼
┌────────────────────┐      2. Generate QR + fingerprint      ┌────────────────────┐
│ backend/           │ ───────────────────────────────────────▶ │ Go HTTP backend    │
│ (Go server)        │ ◀───────────────────────────────────────   │ /api/qr            │
└─────────┬──────────┘                                         └─────────┬──────────┘
          │                                                          │
          │ 3. User pays via UPI                                     │
          ▼                                                          ▼
┌────────────────────┐      4. Gmail email arrives            ┌────────────────────┐
│ FamApp email       │ ───────────────────────────────────────▶ │ Cloudflare Worker   │
│ notification        │                                         │ validates SPF/DKIM/ │
└────────────────────┘                                         │ DMARC + parses      │
                                                              │ amount/UTR + signs  │
                                                              │ payload + stores in │
                                                              │ KV                 │
                                                              └─────────┬──────────┘
                                                                        │
                                                                        │ 5. Verify with HMAC + KV
                                                                        ▼
                                                              ┌────────────────────┐
                                                              │ backend /api/verify│
                                                              │ checks conditions  │
                                                              │ and marks payment  │
                                                              └────────────────────┘
```

The key idea is simple:

- The app creates a QR with a tiny randomized paise offset so the exact amount becomes a unique fingerprint.
- The worker listens for FamApp success emails and stores a signed record keyed by the amount/fingerprint.
- The backend validates the stored payment, prevents replay, checks API-key validity, and updates expiry or recharge state.

---

## Current architecture

### Backend (`backend/`)

The Go backend handles the product experience and API layer:

- OAuth login with Google
- browser dashboard and admin UI
- QR generation endpoints
- payment verification
- recharge verification
- history and revoke flows
- API-key and expiry management
- release metadata endpoints
- health endpoint

Key routes:

- `GET /` — landing page
- `GET /dashboard` — authenticated dashboard
- `GET /admin` — admin console
- `GET /docs` — docs page
- `GET /health` — health check
- `GET /api/latest-release` — release metadata
- `GET /api/login` — OAuth login
- `GET /api/auth/callback` — Google callback
- `GET /api/me` — current authenticated user
- `GET /api/history` — payment history
- `GET /api/revoke` — revoke and rotate API key
- `GET /api/qr` / `POST /api/qr` style generation flow — generate QR payload
- `GET /api/verify` — verify payment by API key + amount
- `GET /api/recharge/verify` — recharge verification
- `POST /api/admin/*` — admin endpoints

### Worker (`cloudflare-worker/`)

The worker processes inbound mail and extracts only valid FamApp payment notifications:

- checks `Authentication-Results` for `spf=pass`, `dkim=pass`, and `dmarc=pass`
- ignores Google forwarding emails
- matches successful payment text like `Received ₹X.XX`
- extracts `UTR` and `amount`
- creates an HMAC signature using `KV_HMAC_SECRET`
- stores the record in Cloudflare KV for backend verification

This makes the payment data ingestion reliable and tamper-resistant.

---

## Features

- Google OAuth 2.0 sign-in
- per-user API key generation and rotation
- QR generation with fingerprinted amount
- FamApp sender validation and email verification
- replay protection and consumed-key checks
- precise verification using HMAC-backed payload integrity
- session and user tracking in Cloudflare KV
- admin dashboard for user and expiry management
- Go-based backend with embedded UI templates
- Cloudflare Worker-based email ingestion

---

## Quick start

### 1. Install backend dependencies

```bash
cd backend
go mod download
```

### 2. Run the backend locally

```bash
cd backend
PORT=8080 go run .
```

The backend serves the landing page, dashboard, admin pages, health checks, and API routes.

### 3. Deploy the Cloudflare Worker

```bash
cd cloudflare-worker
npm install
wrangler deploy
```

Make sure the worker is configured with the required KV bindings and secret:

```toml
[[kv_namespaces]]
binding = "PAYMENT_KV"
id = "..."
```

And the env secret:

```bash
wrangler secret put KV_HMAC_SECRET
```

---

## Required configuration

### Backend environment variables

Typical values used by the Go app:

```bash
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
GOOGLE_REDIRECT_URI=
CLOUDFLARE_ACCOUNT_ID=
CLOUDFLARE_KV_NAMESPACE=
CLOUDFLARE_API_TOKEN=
BASE_URL=
ADMIN_PASSWORD=
SUPPORT_LINK=
PORT=8080
```

### Worker environment variables

```bash
KV_HMAC_SECRET=your-long-random-secret
PAYMENT_KV=your-kv-binding
```

---

## Project structure

```text
fampay-autopay/
├── README.md
├── LICENSE
├── auto-payment-fixed-author-developer-ximanta.md
├── backend/
│   ├── main.go
│   ├── handlers.go
│   ├── kv.go
│   ├── email.go
│   ├── gmail.go
│   ├── qr.go
│   ├── release.go
│   ├── worker.go
│   ├── admin.go
│   ├── go.mod
│   └── templates/
│       ├── index.html
│       ├── dashboard.html
│       ├── admin.html
│       ├── docs.html
│       ├── privacy.html
│       ├── terms.html
│       └── ai_prompt.md
├── cloudflare-worker/
│   ├── worker.js
│   ├── package.json
│   └── wrangler.toml
├── verify-api/        # legacy
├── qr-api/            # legacy
└── Vercel-API/        # legacy folder retained for historical reference only
```

> The repo still contains older legacy folders for reference, but the current active implementation is the `backend/` + `cloudflare-worker/` architecture.

---

## Client integration guide

For the integration and payment rules, see the client-facing guide:

- [LIVE DEMO GUIDE](./auto-payment-fixed-author-developer-ximanta.md)

This guide covers:

- OAuth/API key setup
- QR generation flow
- verification rules
- response shape
- required client-side behaviour
- payment edge cases and rules

---

## License

MIT — see [LICENSE](LICENSE).

---

## Author

**realximanta** — [Developer Telegram](https://b4nzw.t.me)
