// api/admin/login.js
const { v4: uuidv4 } = require('uuid');
const { put } = require('../../lib/kv');
const config = require('../../lib/config');

module.exports = async (req, res) => {
  if (req.method !== 'POST') {
    return res.status(405).json({ error: 'Method not allowed' });
  }

  const { password } = req.body || {};
  if (!password || password !== config.ADMIN_PASSWORD) {
    return res.status(401).json({ error: 'Invalid password' });
  }

  const token = uuidv4().replace(/-/g, '');
  await put(`admin_session:${token}`, JSON.stringify({
    createdAt: new Date().toISOString(),
  }));

  res.json({ token });
};
