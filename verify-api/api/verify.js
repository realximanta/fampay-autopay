// api/verify.js — the main verification endpoint
const { get, put } = require('../lib/kv');
const { verifyPayment } = require('../lib/gmail');
const config = require('../lib/config');

async function checkRateLimit(apiKey) {
  const minuteBucket = Math.floor(Date.now() / 60000);
  const rlKey = `ratelimit:${apiKey}:${minuteBucket}`;
  const current = parseInt((await get(rlKey)) || '0', 10);
  if (current >= config.RATE_LIMIT_PER_MINUTE) {
    return { allowed: false, remaining: 0 };
  }
  await put(rlKey, String(current + 1), { expirationTtl: 120 });
  return { allowed: true, remaining: config.RATE_LIMIT_PER_MINUTE - current - 1 };
}

module.exports = async (req, res) => {
  const { key, amount } = req.query;

  if (!key || !amount) {
    return res.status(400).json({
      verified: false, received: 0, status: 'invalid_request',
      error: 'Missing key or amount',
    });
  }

  const amountStr = String(amount);
  if (!/^\d+(\.\d{1,2})?$/.test(amountStr)) {
    return res.status(400).json({
      verified: false, received: 0, status: 'invalid_request',
      error: 'Invalid amount format',
    });
  }

  const amountNum = Number(amountStr);
  if (!Number.isFinite(amountNum) || amountNum <= 0 || amountNum > 1_000_000) {
    return res.status(400).json({
      verified: false, received: 0, status: 'invalid_request',
      error: 'Amount out of range',
    });
  }

  const rl = await checkRateLimit(key);
  if (!rl.allowed) {
    return res.status(429).json({
      verified: false, received: 0, status: 'rate_limited',
      error: 'Too many requests',
      remaining: 0,
      retry_after_seconds: 60,
    });
  }

  const consumeKey = `consumed:${key}:${amountNum.toFixed(2)}`;
  const consumed = await get(consumeKey);
  if (consumed) {
    try {
      const data = JSON.parse(consumed);
      return res.json({
        verified: true,
        received: data.received,
        status: 'exact',
        note: 'already_verified',
      });
    } catch {}
  }

  try {
    const raw = await get(`api_key:${key}`);
    if (!raw) {
      return res.status(401).json({
        verified: false, received: 0, status: 'invalid_key',
        error: 'Invalid API key',
      });
    }

    let userData;
    try { userData = JSON.parse(raw); } catch {
      return res.status(500).json({
        verified: false, received: 0, status: 'error',
        error: 'Invalid data store',
      });
    }

    if (userData.revoked === true) {
      return res.status(403).json({
        verified: false, received: 0, status: 'revoked',
        error: 'API key revoked',
      });
    }

    if (userData.expiresAt && new Date() > new Date(userData.expiresAt)) {
      return res.status(403).json({
        verified: false, received: 0, status: 'expired',
        error: 'API key expired',
      });
    }

    if (userData.emailAddress) {
      const blacklistRaw = await get(`blacklist:${userData.emailAddress}`);
      if (blacklistRaw) {
        return res.status(403).json({
          verified: false, received: 0, status: 'blacklisted',
          error: 'Account blacklisted',
        });
      }
    }

    if (!userData.refreshToken) {
      return res.status(500).json({
        verified: false, received: 0, status: 'error',
        error: 'No refresh token for this key',
      });
    }

    const result = await verifyPayment(userData.refreshToken, amountNum);

    if (result.verified && result.status === 'exact') {
      await put(
        consumeKey,
        JSON.stringify({ received: result.received, at: Date.now() }),
        { expirationTtl: config.CONSUME_TTL_SECONDS }
      );
    }

    res.json({ ...result, remaining: rl.remaining });
  } catch (error) {
    console.error('Verification error:', error);
    res.status(500).json({
      verified: false, received: 0, status: 'error',
      error: 'Internal server error',
    });
  }
};
