// api/admin/user.js
const { get } = require('../../lib/kv');

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

  const indexRaw = await get('user_index');
  const userIndex = indexRaw ? JSON.parse(indexRaw) : [];

  const enriched = [];
  for (const entry of userIndex) {
    const dataRaw = await get(`api_key:${entry.apiKey}`);
    if (!dataRaw) continue;
    const data = JSON.parse(dataRaw);
    const blacklistRaw = await get(`blacklist:${entry.emailAddress}`);
    enriched.push({
      apiKey: entry.apiKey,
      emailAddress: entry.emailAddress,
      createdAt: data.createdAt,
      lastLogin: data.lastLogin,
      revoked: data.revoked === true,
      blacklisted: !!blacklistRaw,
    });
  }

  enriched.sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt));

  res.json({ users: enriched, total: enriched.length });
};
