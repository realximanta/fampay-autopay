# Auto-Pay Verification API — Client Integration Guide

Two endpoints. Two calls. That's the entire integration.

Built and maintained by **Ximanta** — Better Call Tuku.

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

---

## 2. GENERATE PAYMENT QR

Endpoint:
GET https://qr-api-vercel.vercel.app/qr

Query parameters:
- upi       (required) — your UPI ID, e.g. yourname@fam
- amount    (required) — base amount in rupees, integer, e.g. 10
- bot_name  (required) — display name shown to payer, e.g. MyBot
- remark    (required) — any unique alphanumeric string, e.g. A7K2N9QX4M

Example:
GET https://qr-api-vercel.vercel.app/qr?upi=yourname@fam&amount=10&bot_name=MyBot&remark=A7K2N9QX4M

Returns:
- Content-Type: image/png
- Body: PNG image bytes (scannable UPI QR)

Returns these response headers:
- X-Final-Amount  — the exact rupee amount encoded in the QR (e.g. 10.05)
- X-Base-Amount   — the amount you requested (e.g. 10)
- X-Paise         — random 1–10 paise added by the API (e.g. 5)

Always read X-Final-Amount and store it against the deposit. That exact value
is the transaction fingerprint used later by the verify endpoint.

### Why the extra paise?

FamApp removed the "Purpose:" field from their payment notification emails.
Earlier versions of this API relied on that field to identify which payment
belonged to which transaction. Without it, matching broke.

This version adds a random 1–10 paise to the requested amount and encodes the
total in the QR. That decimal amount becomes a unique fingerprint for the
transaction. When the FamApp email arrives, the verify endpoint matches by
that exact decimal amount.

The paise is not a fee. It is not credited to the user. Only the base rupee
amount is credited.

---

## 3. VERIFY THE PAYMENT

Endpoint:
GET https://auto-payment.ximanta.xyz/api/verify

Query parameters:
- key     (required) — your API key from section 1
- amount  (required) — the X-Final-Amount value from the QR API (e.g. 10.05)
- remark  (optional) — accepted for backward compatibility, ignored by the verifier

Example:
GET https://auto-payment.ximanta.xyz/api/verify?key=YOUR_API_KEY&amount=10.05

### Response shapes

Exact match:
{
  "verified": true,
  "received": 10.05,
  "status": "exact",
  "remaining": 27
}

Not found:
{
  "verified": false,
  "received": 0,
  "status": "not_found",
  "remaining": 26
}

Partial payment (user paid less):
{
  "verified": false,
  "received": 499.13,
  "expected": 500.17,
  "status": "partial",
  "remaining": 25
}

Overpaid (user paid more):
{
  "verified": false,
  "received": 500.50,
  "expected": 500.17,
  "status": "overpaid",
  "remaining": 24
}

Already verified (same amount queried twice):
{
  "verified": true,
  "received": 10.05,
  "status": "exact",
  "note": "already_verified"
}

Rate limited:
{
  "verified": false,
  "received": 0,
  "status": "rate_limited",
  "error": "Too many requests",
  "remaining": 0,
  "retry_after_seconds": 60
}

Invalid request:
{
  "verified": false,
  "received": 0,
  "status": "invalid_request",
  "error": "Invalid amount format"
}

### Status values

| Status          | verified | Meaning                                                       |
|-----------------|----------|---------------------------------------------------------------|
| exact           | true     | Received amount matches expected within ±0.01                 |
| partial         | false    | Received less than expected — client decides what to do       |
| overpaid        | false    | Received more than expected — client decides what to do       |
| not_found       | false    | No matching FamApp email in the last 10 minutes               |
| error           | false    | Upstream failure (Gmail or network)                           |
| invalid_key     | false    | API key is wrong                                              |
| revoked         | false    | Key was revoked                                               |
| blacklisted     | false    | Account is blocked                                            |
| expired         | false    | Key has expired                                               |
| rate_limited    | false    | Too many requests this minute — wait 60 seconds                |
| invalid_request | false    | Malformed amount — client must fix the request                 |

**Important:** Only `status: "exact"` returns `verified: true`. Partial and
overpaid return `verified: false` but include the `received` amount so the
client can still act on the payment. See the Rules section for how.

---

## 4. HOW THE TWO ENDPOINTS FIT TOGETHER

Step 1 — Call the QR API with your base amount.
Step 2 — Read the X-Final-Amount response header. Store it as the deposit's
         `fingerprint_amount` in your DB.
Step 3 — Show the PNG to the user.
Step 4 — User scans the QR with any UPI app and pays the exact amount shown.
Step 5 — Call the Verify API with that same X-Final-Amount value.
Step 6 — Apply the Rules section to decide whether to credit.

The X-Final-Amount is the transaction fingerprint. It is unique to this
transaction. No remark matching is needed.

---

## 5. PYTHON EXAMPLE

```python
import os
import time
import secrets
import string
import requests

QR_API     = "https://qr-api-vercel.vercel.app/qr"
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
        "key": API_KEY,
        "amount": amount,
    }, timeout=15)
    return r.json()

def verify_with_retry(amount, attempts=5, delay=5):
    last = {"verified": False, "status": "not_found"}
    for i in range(attempts):
        res = verify(amount)
        status = res.get("status")

        # Rule 5: stop on terminal errors
        if status in ("invalid_key", "revoked", "blacklisted",
                      "expired", "invalid_request", "rate_limited"):
            return res

        # Rule 6: exact match with our fingerprint → done
        if status == "exact" and res.get("verified"):
            return res

        # Rule 7 / 8: partial or overpaid → stop and let caller decide
        if status in ("partial", "overpaid"):
            return res

        last = res
        if i < attempts - 1:
            time.sleep(delay)

    return last


def handle_payment(user_id, base_amount, db):
    # Rule 1: only one pending deposit per user
    if db.has_pending_deposit(user_id):
        return {"error": "Finish or cancel your pending deposit first"}

    qr = create_qr(base_amount)
    deposit_id = db.create_deposit(
        user_id=user_id,
        base_amount=qr["base_amount"],
        fingerprint_amount=qr["fingerprint_amount"],
        status="pending",
    )

    # send qr["image"] to user

    result = verify_with_retry(qr["fingerprint_amount"])
    status = result.get("status")

    # Rule 6 + Rule 21: exact AND matches our fingerprint
    if status == "exact" and abs(result["received"] - qr["fingerprint_amount"]) < 0.01:
        # Rule 5: idempotent credit
        if db.mark_paid_once(deposit_id):
            db.credit_user(user_id, qr["base_amount"])
        return {"ok": True, "credited": qr["base_amount"]}

    # Rule 7: partial — credit what was received
    if status == "partial":
        # Rule 21: only if the received amount is close to OUR fingerprint
        if abs(result["received"] - qr["fingerprint_amount"]) < 1.0:
            if db.mark_paid_once(deposit_id):
                db.credit_user(user_id, result["received"])
            return {
                "ok": True,
                "credited": result["received"],
                "shortfall": result["expected"] - result["received"],
            }
        return {"ok": False, "status": "partial_mismatch"}

    # Rule 8: overpaid — credit only base
    if status == "overpaid":
        if abs(result["received"] - qr["fingerprint_amount"]) < 1.0:
            if db.mark_paid_once(deposit_id):
                db.credit_user(user_id, qr["base_amount"])
            return {
                "ok": True,
                "credited": qr["base_amount"],
                "excess": result["received"] - result["expected"],
            }
        return {"ok": False, "status": "overpaid_mismatch"}

    if status == "not_found":
        db.mark_pending(deposit_id)
        return {"ok": False, "status": "not_found"}

    return {"ok": False, "status": status, "raw": result}
```

---

## 6. JAVASCRIPT EXAMPLE

```javascript
import crypto from "node:crypto";

const QR_API     = "https://qr-api-vercel.vercel.app/qr";
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
    key: API_KEY,
    amount: String(amount),
  });
  const res = await fetch(`${VERIFY_API}?${params}`);
  return res.json();
}

async function verifyWithRetry(amount, attempts = 5, delayMs = 5000) {
  let last = { verified: false, status: "not_found" };
  for (let i = 0; i < attempts; i++) {
    const r = await verify(amount);
    const status = r.status;

    if (["invalid_key", "revoked", "blacklisted",
         "expired", "invalid_request", "rate_limited"].includes(status)) return r;
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

## 7. COMPLETE ENDPOINT LIST

QR Generator:
GET https://qr-api-vercel.vercel.app/qr
Query: upi, amount, bot_name, remark
Returns: PNG + headers X-Final-Amount, X-Base-Amount, X-Paise

Payment Verification:
GET https://auto-payment.ximanta.xyz/api/verify
Query: key, amount, [remark]
Returns: JSON { verified, received, status, expected?, remaining?, note? }

Sign In:
GET https://auto-payment.ximanta.xyz/api/auth

Revoke Key:
GET https://auto-payment.ximanta.xyz/api/auth?action=revoke

That is the complete surface area of the API. Nothing else exists.

---

## 8. RULES

These rules are mandatory. Follow them exactly. They exist to prevent fraud,
credit mistakes, and lost revenue. Read them before writing any code.

### Rule 1 — Only one pending deposit per user

A user cannot request a new QR while a previous deposit is still pending or
being checked. Enforce this in your DB.

    SELECT COUNT(*) FROM deposits
    WHERE user_id = ? AND status IN ('pending', 'checking');

If the count is greater than 0, reject the new QR request with a message like
"You have a pending deposit. Complete or cancel it first."

This rule also prevents two QRs from the same user getting the same
fingerprint amount, which would cause cross-matching.

### Rule 2 — Store base and fingerprint amounts separately

The deposit row must have two distinct columns:

- base_amount         — the amount the user asked for (e.g. 500)
- fingerprint_amount  — the exact amount encoded in the QR (e.g. 500.17)

Never derive one from the other. Never overwrite one with the other.

    CREATE TABLE deposits (
      id                  INTEGER PRIMARY KEY,
      user_id             INTEGER NOT NULL,
      base_amount         REAL NOT NULL,
      fingerprint_amount  REAL NOT NULL,
      status              TEXT NOT NULL,
      received_amount     REAL DEFAULT 0,
      created_at          INTEGER NOT NULL
    );

### Rule 3 — Credit only the base amount

When verification succeeds, credit the user's base_amount, not the
fingerprint amount. The extra 1–10 paise is a transaction fingerprint, not
a bonus.

Wrong:   creditUser(userId, deposit.fingerprintAmount);  // credits 500.17
Right:   creditUser(userId, deposit.baseAmount);         // credits 500

### Rule 4 — Use the API key server-side only

The API key must never appear in:

- Frontend JavaScript
- HTML source
- Mobile app binaries
- Public GitHub repositories
- Client-side logs

The QR and Verify calls must be made from your backend. The frontend calls
your backend, and your backend calls this API.

If the key leaks, anyone can consume your rate limit and verify payments
against your Gmail. Revoke and regenerate immediately if exposed.

### Rule 5 — Idempotent credit

Before crediting a user, the DB must atomically flip the deposit status
from checking to approved. If the UPDATE affects 0 rows, do NOT credit.
This prevents double-credit from replay attacks, retries, or bugs.

    UPDATE deposits
    SET status = 'approved'
    WHERE id = ? AND status IN ('pending', 'checking');
    -- Only credit if this UPDATE returned changes = 1

### Rule 6 — Credit ONLY when status is "exact" AND received matches your fingerprint

Two conditions must both be true before crediting:

1. response.status === "exact"
2. Math.abs(response.received - deposit.fingerprint_amount) < 0.01

If either fails, do not credit. Route partial and overpaid to Rule 7 / Rule 8.

Example:

    if (response.status === "exact"
        && Math.abs(response.received - deposit.fingerprint_amount) < 0.01) {
      const ok = await db.markPaidOnce(deposit.id);
      if (ok) await creditUser(deposit.userId, deposit.base_amount);
    }

### Rule 7 — Handle partial payments gracefully

If the user pays less than the fingerprint amount, the API returns:

    {
      "verified": false,
      "received": 499.13,
      "expected": 500.17,
      "status": "partial"
    }

Two acceptable behaviors. Pick ONE and stick to it.

Option A — Credit what they paid:

    if (status === "partial") {
      const credited = await db.markPaidOnce(deposit.id);
      if (credited) await creditUser(deposit.userId, result.received);
      await notifyUser(
        `You paid ₹${result.received} instead of ₹${result.expected}. ` +
        `₹${result.received} has been credited.`
      );
    }

Option B — Route to manual review:

    if (status === "partial") {
      await db.markForReview(deposit.id, result.received);
      await notifyUser("Partial payment received. Please contact support.");
    }

Never credit the full base amount when the user paid less.

### Rule 8 — Handle overpaid payments

If the user pays more than the fingerprint amount, the API returns:

    {
      "verified": false,
      "received": 500.50,
      "expected": 500.17,
      "status": "overpaid"
    }

Standard behavior — credit the base amount, ignore the excess:

    if (status === "overpaid") {
      const ok = await db.markPaidOnce(deposit.id);
      if (ok) await creditUser(deposit.userId, deposit.base_amount);
      const excess = result.received - result.expected;
      if (excess > 0) {
        await notifyUser(
          `₹${deposit.base_amount} credited. ` +
          `You paid ₹${excess.toFixed(2)} extra — this has not been added.`
        );
      }
    }

Never credit the excess. Never credit result.received when it exceeds the
base amount.

### Rule 9 — Never retry on terminal errors

These statuses mean the request is hopeless. Do not retry:

- invalid_key       — the API key is wrong; fix your config
- revoked           — the key was revoked; re-authenticate
- blacklisted       — the account is blocked
- expired           — the key is past its expiry
- invalid_request   — your request is malformed; fix it
- rate_limited      — too many requests; wait retry_after_seconds

Retrying these wastes your quota and can trigger further rate limits.

### Rule 10 — Respect the rate limit

The API allows 30 requests per minute per API key. The response includes a
remaining field showing how many you have left this minute.

If remaining drops below 5, slow down. If you receive rate_limited, wait the
number of seconds given in retry_after_seconds before retrying.

Do not run parallel verify loops. One user = one polling loop. Aggregate
polling across users must be sequential, not parallel.

### Rule 11 — Persist deposit state in your DB

The API is stateless. It does not remember what it told you 5 seconds ago.

You must store, for every deposit:

- The fingerprint_amount returned by the QR API
- The base_amount the user requested
- The current status
- The received amount once verified

If your server restarts and you have not persisted this, the payment is
unrecoverable from your side.

### Rule 12 — Never expose the API key to the user

The API key is a credential for your Gmail inbox. Do not:

- Show it to end users
- Include it in any URL shown to the user
- Log it to files accessible to users
- Send it in chat messages
- Include it in error reports

Only your backend talks to auto-payment.ximanta.xyz. The user's device only
talks to your backend.

### Rule 13 — Never invent new endpoints

The only endpoints that exist are listed in section 7. Do not call
/api/deposit/create, /api/payments, /api/status, or any other path — they do
not exist and will return 404.

If you need an endpoint that wraps this API for your frontend, build it
yourself in your own backend. Names like /api/deposit/create are your
endpoints, not ours.

### Rule 14 — Never trust the user's word

Do not credit any user because they said "I paid." Credit only after the
verify endpoint returns verified: true with status: "exact", or after you
have applied the partial/overpaid handling in Rule 7 and Rule 8.

"This is my payment screenshot" is not verification. A screenshot can be
faked in 30 seconds. The API response is the only source of truth.

### Rule 15 — Never store the API key in the frontend

If you are building a website or app:

- The frontend calls your backend
- Your backend calls the QR API and Verify API
- The frontend never sees the API key
- The frontend never calls qr-api-vercel.vercel.app or
  auto-payment.ximanta.xyz directly

This is not optional. Client-side API keys are the number one cause of abuse
on this service.

### Rule 16 — Always use HTTPS

Both APIs are served over HTTPS. Never proxy them over plain HTTP. Never
disable certificate validation. Never call them from a plain HTTP page
(mixed content will be blocked by browsers).

### Rule 17 — Log everything server-side

For every verify call, log:

- Deposit ID
- Fingerprint amount
- Status returned
- Received amount (if any)
- Timestamp
- IP of the caller (if applicable)

When a user disputes a credit or a missed payment, these logs are your only
defense. Log for at least 30 days.

### Rule 18 — Credit at the exact moment of verification

Do not batch credits. Do not schedule credits. Do not credit "later in the
day." When the verify endpoint returns success, credit the user immediately
in the same request handler. Delays cause:

- User complaints
- Race conditions with other deposits
- Missing credits if your server crashes before flushing

### Rule 19 — Notify the user clearly

After verification, tell the user exactly what happened:

- Exact:      "₹500 credited to your balance. Thank you."
- Partial:    "You paid ₹499.13 instead of ₹500.17. ₹499.13 credited."
- Overpaid:   "₹500 credited. You paid ₹0.33 extra — not added."
- Not found:  "Payment not detected yet. Wait 30 seconds and try again."
- Failed:     "Payment not detected. Please contact support with your UTR."

Vague messages like "Payment processing…" lead to support tickets.

### Rule 20 — Keep it simple

Do not over-engineer. The two endpoints are the entire API. Wrap them in
your backend, persist the state, credit idempotently, and notify the user.
Anything beyond that is unnecessary and creates bugs.

### Rule 21 — Match the received amount against YOUR deposit's fingerprint

On every verify response, compare result.received against your deposit's
fingerprint_amount — the exact value that came from the QR API when you
created this specific deposit.

Even if the API says status: "exact", only credit if:

    Math.abs(response.received - deposit.fingerprint_amount) < 0.01

Never credit based on response.received alone. The Gmail inbox may contain
payments from other users of the same API key. The only safe rule is:

    Credit ONLY when response.status === "exact"
    AND
    Math.abs(response.received - deposit.fingerprint_amount) < 0.01

Example — User B's fingerprint is 10.04. User A's payment of 10.10 lands in
the same inbox. User B calls verify with amount=10.04. The API returns:

    {
      "verified": false,
      "received": 10.10,
      "expected": 10.04,
      "status": "overpaid"
    }

Client code checks:

    Math.abs(10.10 - 10.04) < 0.01  →  false  →  do NOT credit.

Even a naive client that only checks received === fingerprint (without
looking at status) is safe. That's the point of Rule 21 — it makes the rule
trivial for AI-generated code to obey.

### Rule 22 — Guard the value comparison with tolerance, not equality

Floating-point comparison (10.05 === 10.05) can fail due to binary
representation. Always compare with Math.abs(a - b) < 0.01.

### Rule 23 — Cancel stale deposits

If a user never pays and abandons a deposit, its status remains pending
forever. This blocks Rule 1 (no new QR). Implement a cancel button or an
automatic 15-minute timeout that marks the deposit as cancelled.

    UPDATE deposits
    SET status = 'cancelled'
    WHERE id = ? AND status = 'pending'
      AND created_at < (NOW() - INTERVAL '15 minutes');

Without this, users get stuck and complain.

### Rule 24 — Never let the client credit from raw "received" alone

Rule 6 and Rule 21 must both be enforced in code, not just documented. If
your AI-generated client uses only one of them, the collision loophole
reopens. Verify both checks exist before deploying.

---

## 9. AUTHOR - give a credit to the author

Built and maintained by **Ximanta** — Better Call Tuku.
For support: https://kimi9bot.t.me
