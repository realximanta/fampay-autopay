# SYSTEM PROMPT: Auto-Pay Verification API Integration

You are an expert backend developer. Your task is to write the complete, production-ready integration code for my application (Python or Node.js) to use the Auto-Pay Verification API.

Read this entire document carefully. It contains the exact endpoints, the mandatory security rules to prevent payment fraud, and example implementations.

---

## 1. GET YOUR API KEY

1. Open `https://auto-payment.ximanta.xyz`
2. Click "Sign In with Google"
3. Use the Gmail account that receives FamApp payment notifications
4. Grant read-only access
5. Copy the 20-character API key shown

Revoke and get a new one anytime:
`https://auto-payment.ximanta.xyz/api/auth?action=revoke`

Store your key in an environment variable. Never expose it client-side. 
**Note on Expiry:** API keys expire. You must build a way for users to "recharge" their API key on the dashboard. If the API returns `expired`, your app must prompt the user to recharge.

---

## 2. GENERATE PAYMENT QR

Endpoint:
`GET https://auto-payment.ximanta.xyz/qr`

Query parameters:
- `upi`       (required) — your UPI ID, e.g. `yourname@fam`
- `amount`    (required) — base amount in rupees, integer, e.g. `10`
- `bot_name`  (required) — display name shown to payer, e.g. `MyBot`
- `remark`    (required) — any unique alphanumeric string, e.g. `A7K2N9QX4M`

Example Request:
```http
GET https://auto-payment.ximanta.xyz/qr?upi=yourname@fam&amount=10&bot_name=MyBot&remark=A7K2N9QX4M
```

Returns:
- Content-Type: `image/png`
- Body: PNG image bytes (scannable UPI QR)

**Returns these response headers (CRITICAL):**
- `X-Final-Amount`  — the exact rupee amount encoded in the QR (e.g. 10.05)
- `X-Base-Amount`   — the amount you requested (e.g. 10)
- `X-Paise`         — random 1–99 paise added by the API (e.g. 5)

Always read `X-Final-Amount` and store it against the deposit. That exact value is the transaction fingerprint used later by the verify endpoint.

### Why the extra paise?
FamApp removed the "Purpose:" field from their payment notification emails. Earlier versions of this API relied on that field to identify which payment belonged to which transaction. Without it, matching broke.

This version adds a random 1–99 paise to the requested amount and encodes the total in the QR. That decimal amount becomes a unique fingerprint for the transaction. When the FamApp email arrives, the verify endpoint matches by that exact decimal amount.

The paise is not a fee. It is not credited to the user. Only the base rupee amount is credited.

---

## 3. VERIFY THE PAYMENT

Endpoint:
`GET https://auto-payment.ximanta.xyz/api/verify`

Query parameters:
- `key`     (required) — your API key from section 1
- `amount`  (required) — the `X-Final-Amount` value from the QR API (e.g. 10.05)

**Authentication:** Accepts `?key=YOUR_KEY` in the query, but for security, you should also send it via the `Authorization: Bearer YOUR_KEY` header.

Example:
```http
GET https://auto-payment.ximanta.xyz/api/verify?key=YOUR_API_KEY&amount=10.05
Authorization: Bearer YOUR_API_KEY
```

### Response shapes

Exact match:
```json
{
  "verified": true,
  "received": 10.05,
  "status": "exact"
}
```

Not found:
```json
{
  "verified": false,
  "received": 0,
  "status": "not_found"
}
```

Partial payment (user paid less):
```json
{
  "verified": false,
  "received": 499.13,
  "expected": 500.17,
  "status": "partial"
}
```

Overpaid (user paid more):
```json
{
  "verified": false,
  "received": 500.50,
  "expected": 500.17,
  "status": "overpaid"
}
```

Already verified (same amount queried twice):
```json
{
  "verified": true,
  "received": 10.05,
  "status": "exact",
  "note": "already_verified"
}
```

### Status values
| Status          | verified | Meaning                                                       |
|-----------------|----------|---------------------------------------------------------------|
| `exact`         | true     | Received amount matches expected within ±0.01                 |
| `partial`       | false    | Received less than expected — client decides what to do       |
| `overpaid`      | false    | Received more than expected — client decides what to do       |
| `not_found`     | false    | No matching FamApp email in the last 10 minutes               |
| `error`         | false    | Upstream failure (Gmail or network)                           |
| `invalid_key`   | false    | API key is wrong                                              |
| `revoked`       | false    | Key was revoked                                               |
| `expired`       | false    | Key has expired (User must recharge)                          |
| `invalid_amount`| false    | Malformed amount — client must fix the request                |

**Important:** Only `status: "exact"` returns `verified: true`. Partial and overpaid return `verified: false` but include the `received` amount so the client can still act on the payment.

---

## 4. HOW THE TWO ENDPOINTS FIT TOGETHER

Step 1 — Call the QR API with your base amount.
Step 2 — Read the `X-Final-Amount` response header. Store it as the deposit's `fingerprint_amount` in your DB.
Step 3 — Show the PNG to the user.
Step 4 — User scans the QR with any UPI app and pays the exact amount shown.
Step 5 — Call the Verify API with that same `X-Final-Amount` value.
Step 6 — Apply the Rules section to decide whether to credit.

---

## 5. RULES (MANDATORY)

These rules are mandatory. Follow them exactly. They exist to prevent fraud, credit mistakes, and lost revenue.

### Rule 1 — Only one pending deposit per user
A user cannot request a new QR while a previous deposit is still pending. Enforce this in your DB.
```sql
SELECT COUNT(*) FROM deposits WHERE user_id = ? AND status IN ('pending', 'checking');
```
If the count > 0, reject the new QR request with: "You have a pending deposit. Complete or cancel it first."

### Rule 2 — Store base and fingerprint amounts separately
The deposit row must have two distinct columns:
- `base_amount` (e.g. 500)
- `fingerprint_amount` (e.g. 500.17)
Never derive one from the other.
```sql
CREATE TABLE deposits (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  base_amount REAL NOT NULL,
  fingerprint_amount REAL NOT NULL,
  status TEXT NOT NULL,
  received_amount REAL DEFAULT 0,
  created_at INTEGER NOT NULL
);
```

### Rule 3 — Credit only the base amount
When verification succeeds, credit the user's `base_amount`, not the `fingerprint_amount`.
Wrong: `creditUser(userId, deposit.fingerprintAmount);`
Right: `creditUser(userId, deposit.baseAmount);`

### Rule 4 — Use the API key server-side only
The API key must never appear in frontend code. The frontend calls your backend, and your backend calls the API.

### Rule 5 — Idempotent credit
Before crediting a user, atomically flip the deposit status. If the UPDATE affects 0 rows, do NOT credit.
```sql
UPDATE deposits SET status = 'approved' WHERE id = ? AND status IN ('pending', 'checking');
```

### Rule 6 — Credit ONLY when status is "exact" AND received matches your fingerprint
Two conditions must both be true:
1. `response.status === "exact"`
2. `Math.abs(response.received - deposit.fingerprint_amount) < 0.01`
If either fails, do not credit.

### Rule 7 — Handle partial payments gracefully
If the user pays less, the API returns `status: "partial"`. Either credit exactly what they paid, or route to manual review. Never credit the full base amount.

### Rule 8 — Handle overpaid payments
If the user pays more, the API returns `status: "overpaid"`. Credit only the `base_amount`, ignore the excess.

### Rule 9 — Never retry on terminal errors
Do not retry if status is `invalid_key`, `revoked`, `expired`, or `invalid_amount`. These mean the request is hopeless without user intervention.

### Rule 10 — Respect the rate limit
Polling should be max 5 attempts, 5 seconds apart. Do not run parallel verify loops.

### Rule 11 — Persist deposit state
The API is stateless. You must store the fingerprint, base amount, and status in your own database.

### Rule 12 — Never expose the API key to the user
Never show the API key in URLs, logs, or chat messages.

### Rule 13 — Never trust the user's word
Credit only after the verify endpoint returns `verified: true` with `status: "exact"`. A payment screenshot can be faked.

### Rule 14 — Never store the API key in the frontend
If building an app or website, the frontend never talks to `auto-payment.ximanta.xyz` directly.

### Rule 15 — Cancel stale deposits
If a user abandons a deposit, mark it as `cancelled` after 15 minutes so they aren't blocked by Rule 1.

---

## 6. PYTHON EXAMPLE

```python
import os
import time
import secrets
import string
import requests

QR_API     = "https://auto-payment.ximanta.xyz/qr"
VERIFY_API = "https://auto-payment.ximanta.xyz/api/verify"

API_KEY  = os.environ["AUTOPAY_KEY"]
UPI_ID   = os.environ["UPI_ID"]
BOT_NAME = os.environ["BOT_NAME"]

def generate_remark(length=10):
    alphabet = string.ascii_uppercase + string.digits
    return "".join(secrets.choice(alphabet) for _ in range(length))

def create_qr(base_amount):
    r = requests.get(QR_API, params={
        "upi": UPI_ID,
        "amount": base_amount,
        "bot_name": BOT_NAME,
        "remark": generate_remark(),
    }, timeout=15)
    r.raise_for_status()
    return {
        "image": r.content,
        "fingerprint_amount": float(r.headers["X-Final-Amount"]),
        "base_amount": base_amount,
    }

def verify(amount):
    r = requests.get(VERIFY_API, params={
        "amount": amount,
    }, headers={
        "Authorization": f"Bearer {API_KEY}"
    }, timeout=15)
    return r.json()

def verify_with_retry(amount, attempts=5, delay=5):
    last = {"verified": False, "status": "not_found"}
    for i in range(attempts):
        res = verify(amount)
        status = res.get("status")

        if status in ("invalid_key", "revoked", "expired", "invalid_amount"):
            return res

        if status == "exact" and res.get("verified"):
            return res

        if status in ("partial", "overpaid"):
            return res

        last = res
        if i < attempts - 1:
            time.sleep(delay)

    return last

def handle_payment(user_id, base_amount, db):
    if db.has_pending_deposit(user_id):
        return {"error": "Finish or cancel your pending deposit first"}

    qr = create_qr(base_amount)
    deposit_id = db.create_deposit(
        user_id=user_id,
        base_amount=qr["base_amount"],
        fingerprint_amount=qr["fingerprint_amount"],
        status="pending",
    )

    result = verify_with_retry(qr["fingerprint_amount"])
    status = result.get("status")

    if status == "exact" and abs(result["received"] - qr["fingerprint_amount"]) < 0.01:
        if db.mark_paid_once(deposit_id):
            db.credit_user(user_id, qr["base_amount"])
        return {"ok": True, "credited": qr["base_amount"]}

    if status == "partial" and abs(result["received"] - qr["fingerprint_amount"]) < 1.0:
        if db.mark_paid_once(deposit_id):
            db.credit_user(user_id, result["received"])
        return {"ok": True, "credited": result["received"]}

    if status == "overpaid" and abs(result["received"] - qr["fingerprint_amount"]) < 1.0:
        if db.mark_paid_once(deposit_id):
            db.credit_user(user_id, qr["base_amount"])
        return {"ok": True, "credited": qr["base_amount"]}

    if status == "not_found":
        db.mark_pending(deposit_id)
        return {"ok": False, "status": "not_found"}

    return {"ok": False, "status": status}
```

---

## 7. JAVASCRIPT EXAMPLE

```javascript
import crypto from "node:crypto";

const QR_API     = "https://auto-payment.ximanta.xyz/qr";
const VERIFY_API = "https://auto-payment.ximanta.xyz/api/verify";

const API_KEY  = process.env.AUTOPAY_KEY;
const UPI_ID   = process.env.UPI_ID;
const BOT_NAME = process.env.BOT_NAME;

function generateRemark(length = 10) {
  const a = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
  const b = crypto.randomBytes(length);
  return Array.from(b, x => a[x % a.length]).join("");
}

async function createQr(baseAmount) {
  const params = new URLSearchParams({
    upi: UPI_ID,
    amount: String(baseAmount),
    bot_name: BOT_NAME,
    remark: generateRemark(),
  });
  const res = await fetch(`${QR_API}?${params}`);
  return {
    image: Buffer.from(await res.arrayBuffer()),
    fingerprintAmount: parseFloat(res.headers.get("X-Final-Amount")),
    baseAmount,
  };
}

async function verify(amount) {
  const params = new URLSearchParams({
    amount: String(amount),
  });
  const res = await fetch(`${VERIFY_API}?${params}`, {
    headers: { "Authorization": `Bearer ${API_KEY}` }
  });
  return res.json();
}

async function verifyWithRetry(amount, attempts = 5, delayMs = 5000) {
  let last = { verified: false, status: "not_found" };
  for (let i = 0; i < attempts; i++) {
    const r = await verify(amount);
    const status = r.status;

    if (["invalid_key", "revoked", "expired", "invalid_amount"].includes(status)) return r;
    if (status === "exact" && r.verified) return r;
    if (status === "partial" || status === "overpaid") return r;

    last = r;
    if (i < attempts - 1) await new Promise(x => setTimeout(x, delayMs));
  }
  return last;
}

async function handlePayment(userId, baseAmount, db) {
  // Rule 1: only one pending deposit per user
  if (await db.hasPendingDeposit(userId)) {
    return { error: "Finish or cancel your pending deposit first" };
  }

  const qr = await createQr(baseAmount);
  const depositId = await db.createDeposit({
    userId,
    baseAmount: qr.baseAmount,
    fingerprintAmount: qr.fingerprintAmount,
    status: "pending",
  });

  // send qr.image to user

  const result = await verifyWithRetry(qr.fingerprintAmount);
  const status = result.status;

  // Rule 6 + Rule 21
  if (status === "exact" && Math.abs(result.received - qr.fingerprintAmount) < 0.01) {
    const ok = await db.markPaidOnce(depositId);
    if (ok) await db.creditUser(userId, qr.baseAmount);
    return { ok: true, credited: qr.baseAmount };
  }

  // Rule 7
  if (status === "partial" && Math.abs(result.received - qr.fingerprintAmount) < 1.0) {
    const ok = await db.markPaidOnce(depositId);
    if (ok) await db.creditUser(userId, result.received);
    return {
      ok: true,
      credited: result.received,
      shortfall: result.expected - result.received,
    };
  }

  // Rule 8
  if (status === "overpaid" && Math.abs(result.received - qr.fingerprintAmount) < 1.0) {
    const ok = await db.markPaidOnce(depositId);
    if (ok) await db.creditUser(userId, qr.baseAmount);
    return {
      ok: true,
      credited: qr.baseAmount,
      excess: result.received - result.expected,
    };
  }

  if (status === "not_found") {
    await db.markPending(depositId);
    return { ok: false, status: "not_found" };
  }

  return { ok: false, status, raw: result };
}
```

---

## 8. COMPLETE ENDPOINT LIST

QR Generator:
`GET https://auto-payment.ximanta.xyz/qr`
Query: `upi`, `amount`, `bot_name`, `remark`
Returns: PNG + headers `X-Final-Amount`, `X-Base-Amount`, `X-Paise`

Payment Verification:
`GET https://auto-payment.ximanta.xyz/api/verify`
Query: `key`, `amount`
Headers: `Authorization: Bearer YOUR_KEY`
Returns: JSON `{ verified, received, status, expected?, note? }`

Sign In:
`GET https://auto-payment.ximanta.xyz/api/login`

Revoke Key:
`GET https://auto-payment.ximanta.xyz/api/logout` (or through the dashboard UI)

That is the complete surface area of the API. Nothing else exists.

---

## 9. AUTHOR

Built and maintained by **Ximanta** — Tuku.
For Telegram support: https://kimi9bot.t.me
Email support: official@ximanta.xyz
