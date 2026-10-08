// api/auth.js — initiates Google OAuth
const { google } = require('googleapis');
const config = require('../lib/config');

module.exports = async (req, res) => {
  const oauth2Client = new google.auth.OAuth2(
    config.GOOGLE_CLIENT_ID,
    config.GOOGLE_CLIENT_SECRET,
    config.GOOGLE_REDIRECT_URI
  );

  const scopes = ['https://www.googleapis.com/auth/gmail.readonly'];
  const action = req.query.action === 'revoke' ? 'revoke' : 'login';
  const state = Buffer.from(JSON.stringify({ action })).toString('base64');

  const url = oauth2Client.generateAuthUrl({
    access_type: 'offline',
    scope: scopes,
    prompt: 'consent',
    state,
  });

  res.redirect(url);
};
