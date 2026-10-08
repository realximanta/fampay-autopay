// api/auth/callback.js
const { google } = require('googleapis');
const { v4: uuidv4 } = require('uuid');
const { get, put } = require('../../lib/kv');
const config = require('../../lib/config');
const { getEmailFromAccessToken } = require('../../lib/gmail');

module.exports = async (req, res) => {
  const code = req.query.code;
  if (!code) return res.status(400).send('Missing code');

  let action = 'login';
  if (req.query.state) {
    try {
      const parsed = JSON.parse(Buffer.from(req.query.state, 'base64').toString());
      action = parsed.action || 'login';
    } catch {}
  }

  const oauth2Client = new google.auth.OAuth2(
    config.GOOGLE_CLIENT_ID,
    config.GOOGLE_CLIENT_SECRET,
    config.GOOGLE_REDIRECT_URI
  );

  try {
    const { tokens } = await oauth2Client.getToken(code);

    let emailAddress;
    try {
      emailAddress = await getEmailFromAccessToken(tokens.access_token);
    } catch (err) {
      console.error('getEmail error:', err);
      return res.status(500).send('Failed to identify Gmail account');
    }

    const blacklistRaw = await get(`blacklist:${emailAddress}`);
    if (blacklistRaw) {
      return res.status(403).send(`
        <html><body style="font-family:sans-serif;background:#0b1120;color:#e2e8f0;display:flex;align-items:center;justify-content:center;min-height:100vh;text-align:center;padding:2rem;">
          <div>
            <h1 style="color:#f87171;">Access Denied</h1>
            <p>Your Gmail account (<b>${emailAddress}</b>) has been blacklisted.</p>
            <p>Contact support: <a href="${config.SUPPORT_LINK}" style="color:#60a5fa;">${config.SUPPORT_LINK}</a></p>
          </div>
        </body></html>
      `);
    }

    const existingKey = await get(`email:${emailAddress}`);
    let apiKey = null;
    let wasRevoked = false;
    let isExisting = false;

    if (existingKey) {
      const existingDataRaw = await get(`api_key:${existingKey}`);
      if (existingDataRaw) {
        const existingData = JSON.parse(existingDataRaw);
        if (!existingData.revoked) {
          if (action === 'revoke') {
            existingData.revoked = true;
            existingData.revokedAt = new Date().toISOString();
            await put(`api_key:${existingKey}`, JSON.stringify(existingData));
            wasRevoked = true;
          } else {
            apiKey = existingKey;
            if (tokens.refresh_token) existingData.refreshToken = tokens.refresh_token;
            existingData.lastLogin = new Date().toISOString();
            existingData.emailAddress = emailAddress;
            await put(`api_key:${apiKey}`, JSON.stringify(existingData));
            isExisting = true;
          }
        }
      }
    }

    if (!apiKey) {
      apiKey = uuidv4().replace(/-/g, '').slice(0, 20);

      let refreshToken = tokens.refresh_token;
      if (!refreshToken && existingKey) {
        const oldRaw = await get(`api_key:${existingKey}`);
        if (oldRaw) {
          const oldData = JSON.parse(oldRaw);
          refreshToken = oldData.refreshToken;
        }
      }
      if (!refreshToken) {
        return res.status(400).send(
          'No refresh token. Please revoke app access in your Google Account, then sign in again.'
        );
      }

      const userData = {
        refreshToken,
        emailAddress,
        createdAt: new Date().toISOString(),
        lastLogin: new Date().toISOString(),
        revoked: false,
      };
      await put(`api_key:${apiKey}`, JSON.stringify(userData));
      await put(`email:${emailAddress}`, apiKey);

      const indexRaw = await get('user_index');
      let userIndex = indexRaw ? JSON.parse(indexRaw) : [];
      userIndex = userIndex.filter(u => u.emailAddress !== emailAddress);
      userIndex.push({
        apiKey,
        emailAddress,
        createdAt: userData.createdAt,
        revoked: false,
      });
      await put('user_index', JSON.stringify(userIndex));
    }

    const params = new URLSearchParams({ api_key: apiKey });
    if (wasRevoked) params.set('revoked', '1');
    else if (isExisting) params.set('existing', '1');
    else params.set('new', '1');

    res.redirect(`${config.BASE_URL}/?${params.toString()}`);
  } catch (error) {
    console.error('OAuth callback error:', error);
    res.status(500).send('Authentication failed: ' + error.message);
  }
};
