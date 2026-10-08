// lib/gmail.js — Gmail verification logic
const { google } = require('googleapis');
const config = require('./config');

function isFamAppSender(fromHeader) {
  if (!fromHeader) return false;
  const lower = fromHeader.toLowerCase();
  return config.FAMAPP_SENDER_DOMAINS.some(d => lower.includes(d));
}

async function verifyPayment(refreshToken, expectedAmount) {
  const oauth2Client = new google.auth.OAuth2(
    config.GOOGLE_CLIENT_ID,
    config.GOOGLE_CLIENT_SECRET,
    config.GOOGLE_REDIRECT_URI
  );
  oauth2Client.setCredentials({ refresh_token: refreshToken });

  const gmail = google.gmail({ version: 'v1', auth: oauth2Client });

  try {
    const queries = [
      `from:no-reply@famapp.in "successfully received" newer_than:10m`,
      `from:famapp.in "successfully received" newer_than:10m`,
      `from:famx.fampay.in "successfully received" newer_than:10m`,
    ];

    let messages = [];
    for (const q of queries) {
      const res = await gmail.users.messages.list({
        userId: 'me',
        q,
        maxResults: 20,
      });
      messages = res.data.messages || [];
      if (messages.length > 0) break;
    }

    if (messages.length === 0) {
      return { verified: false, received: 0, status: 'not_found' };
    }

    const now = Date.now();
    const EXACT_TOLERANCE = 0.01;
    const PARTIAL_WINDOW = 5.0;
    let closest = null;

    for (const msg of messages) {
      const msgData = await gmail.users.messages.get({
        userId: 'me',
        id: msg.id,
        format: 'full',
      });

      const headers = msgData.data.payload?.headers || [];
      const fromHeader = headers.find(h => h.name === 'From')?.value || '';
      if (!isFamAppSender(fromHeader)) continue;

      const { body, internalDate } = parseEmail(msgData.data);
      const emailTime = parseInt(internalDate);
      if (now - emailTime > 10 * 60 * 1000) continue;

      const amountMatch = body.match(
        /You have successfully received\s*(?:₹|Rs\.?|INR)?\s*([\d]+\.\d{2})/i
      );
      if (!amountMatch) continue;

      const received = parseFloat(amountMatch[1]);
      if (!Number.isFinite(received) || received <= 0) continue;

      const diff = Math.abs(received - expectedAmount);

      if (diff < EXACT_TOLERANCE) {
        return { verified: true, received, status: 'exact' };
      }

      if (diff < PARTIAL_WINDOW) {
        if (!closest || diff < closest.diff) {
          closest = { received, diff };
        }
      }
    }

    if (closest) {
      const status = closest.received < expectedAmount ? 'partial' : 'overpaid';
      return {
        verified: false,
        received: closest.received,
        expected: expectedAmount,
        status,
      };
    }

    return { verified: false, received: 0, status: 'not_found' };
  } catch (error) {
    console.error('Gmail API error:', error);
    return { verified: false, received: 0, status: 'error' };
  }
}

async function getEmailFromAccessToken(accessToken) {
  const oauth2Client = new google.auth.OAuth2(
    config.GOOGLE_CLIENT_ID,
    config.GOOGLE_CLIENT_SECRET,
    config.GOOGLE_REDIRECT_URI
  );
  oauth2Client.setCredentials({ access_token: accessToken });
  const gmail = google.gmail({ version: 'v1', auth: oauth2Client });
  const profile = await gmail.users.getProfile({ userId: 'me' });
  return profile.data.emailAddress;
}

async function getEmailFromRefreshToken(refreshToken) {
  const oauth2Client = new google.auth.OAuth2(
    config.GOOGLE_CLIENT_ID,
    config.GOOGLE_CLIENT_SECRET,
    config.GOOGLE_REDIRECT_URI
  );
  oauth2Client.setCredentials({ refresh_token: refreshToken });
  const gmail = google.gmail({ version: 'v1', auth: oauth2Client });
  const profile = await gmail.users.getProfile({ userId: 'me' });
  return profile.data.emailAddress;
}

function parseEmail(msg) {
  const headers = msg.payload?.headers || [];
  const subject = headers.find(h => h.name === 'Subject')?.value || '';
  let body = '';

  function walk(part) {
    if (!part) return;
    if (part.mimeType === 'text/plain' && part.body?.data) {
      body += Buffer.from(part.body.data, 'base64').toString('utf-8') + '\n';
    }
    if (part.parts) for (const p of part.parts) walk(p);
  }

  if (msg.payload?.parts) walk(msg.payload);
  else if (msg.payload?.body?.data) {
    body = Buffer.from(msg.payload.body.data, 'base64').toString('utf-8');
  }

  return { subject, body, internalDate: msg.internalDate };
}

module.exports = {
  verifyPayment,
  getEmailFromAccessToken,
  getEmailFromRefreshToken,
};
