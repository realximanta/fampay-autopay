// api/admin/blacklist.js
const { get, put } = require('../../lib/kv');

async function checkAuth(token) {
  if (!token) return false;
  const session = await get(`admin_session:${token}`);
  return !!session;
}

module.exports = async (req, res) => {
  const token = req.headers['x-admin-token'];
  if (!(await checkAuth(token))) {
    return res.status(401).json({ error: 'Unauthorized' });
  }

  const { email, action } = req.query;
  if (!email) return res.status(400).json({ error: 'Missing email' });

  if (action === 'unblacklist') {
    await put(`blacklist:${email}`, null);
    return res.json({ success: true, action: 'unblacklisted' });
  }

  await put(`blacklist:${email}`, JSON.stringify({
    addedAt: new Date().toISOString(),
  }));

  res.json({ success: true, action: 'blacklisted' });
};
