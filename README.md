<div align="center">

# 💳 fampay-verify

**Automated UPI payment verification for FamApp — no gateway, no webhooks, no manual checking.**

[![Made with Node.js](https://img.shields.io/badge/Node.js-18+-339933?logo=node.js&logoColor=white)](https://nodejs.org/)
[![Python](https://img.shields.io/badge/Python-3.10+-3776AB?logo=python&logoColor=white)](https://python.org/)
[![Hosted on Vercel](https://img.shields.io/badge/Hosted%20on-Vercel-000000?logo=vercel&logoColor=white)](https://vercel.com/)
[![Cloudflare KV](https://img.shields.io/badge/Database-Cloudflare%20KV-F38020?logo=cloudflare&logoColor=white)](https://developers.cloudflare.com/kv/)
[![OAuth 2.0](https://img.shields.io/badge/OAuth%202.0-Google-4285F4?logo=google&logoColor=white)](https://developers.google.com/identity/protocols/oauth2)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

</div>

---

## Overview

`fampay-verify` is a pair of serverless HTTP APIs that let any bot, website, or backend
accept UPI payments to a personal FamApp VPA and verify them automatically by reading
FamApp payment notification emails from a Gmail inbox.

**Two APIs. Two calls. That's the entire integration.**

- **QR Generator API** (Python / Flask) — generates a scannable UPI QR with a unique amount fingerprint.
- **Verification API** (Node.js / Express-style serverless) — matches FamApp emails by that fingerprint.

---

## How it works

```
┌─────────────┐   1. Get QR    ┌──────────────┐
│  Your Bot   │───────────────▶│  QR API      │
│             │◀── PNG + amt ──│              │
└──────┬──────┘                └──────────────┘
       │
       │  2. User scans & pays via any UPI app
       ▼
┌─────────────┐   3. Verify    ┌──────────────┐
│  Your Bot   │───────────────▶│  Verify API  │
│             │◀── {verified}──│  (reads      │
└─────────────┘                │   Gmail)     │
                               └──────────────┘
```

FamApp removed the "Purpose:" field from their notification emails. This project
solves that by adding a random 1–10 paise to the requested amount and encoding the
total in the QR. The decimal amount becomes the transaction fingerprint.

---

## Features

- Google OAuth 2.0 sign-in (read-only Gmail scope)
- API key generation per Gmail account (idempotent)
- Amount-fingerprint verification (no reliance on the `Purpose:` field)
- FamApp sender-domain validation
- Replay protection (consume markers)
- Rate limiting (30 req/min per API key)
- Admin console with revoke + blacklist
- Serverless — free to host on Vercel + Cloudflare KV

---

## Quick start

### 1. Get your API key

Visit the deployed landing page, click **Sign In with Google**, grant read-only
Gmail access, copy the 20-character API key.

### 2. Generate a payment QR

```bash
curl -D headers.txt \
  "https://your-qr-api.vercel.app/qr?upi=yourname@fam&amount=10&bot_name=MyBot&remark=ABC123" \
  -o qr.png

# Read the fingerprint from the headers
grep -i x-final-amount headers.txt
# → X-Final-Amount: 10.05
```

### 3. Verify the payment

```bash
curl "https://your-verify-api.vercel.app/api/verify?key=YOUR_KEY&amount=10.05"
```

```json
{
  "verified": true,
  "received": 10.05,
  "status": "exact"
}
```

---

## Deployment

### Prerequisites

- Vercel account
- Cloudflare account (for KV storage)
- Google Cloud project (for OAuth credentials)

### Setup Google OAuth

1. [Google Cloud Console](https://console.cloud.google.com/) → new project
2. Enable **Gmail API**
3. OAuth consent screen → External → add test users
4. Create OAuth Client ID → **Web application**
5. Add redirect URI: `https://your-verify-api.vercel.app/api/auth/callback`
6. Copy Client ID + Secret

### Setup Cloudflare KV

1. Cloudflare dashboard → Workers & Pages → KV
2. Create namespace → copy its ID
3. Create API token with **Workers KV Storage: Edit** permission
4. Copy Account ID + Namespace ID + API Token

### Deploy QR API

```bash
cd qr-api
vercel --prod
```

### Deploy Verify API

```bash
cd verify-api
vercel --prod
```

### Configure environment variables on Vercel

Set these on the **verify-api** project:

```
GOOGLE_CLIENT_ID
GOOGLE_CLIENT_SECRET
GOOGLE_REDIRECT_URI
CLOUDFLARE_ACCOUNT_ID
CLOUDFLARE_KV_NAMESPACE
CLOUDFLARE_API_TOKEN
BASE_URL
ADMIN_PASSWORD
SUPPORT_LINK
```

---

## Project structure

```
fampay-verify/
├── verify-api/          ← Node.js verification service
│   ├── api/             ← serverless endpoints
│   ├── lib/             ← config, KV client, Gmail logic
│   ├── public/          ← landing page + admin console
│   ├── package.json
│   └── vercel.json
└── qr-api/              ← Python QR generator service
    ├── api/index.py
    ├── requirements.txt
    └── vercel.json
```

---

## License

MIT — see [LICENSE](LICENSE).

---

## Author

**realximanta** — [kimi9bot.t.me](https://kimi9bot.t.me)
