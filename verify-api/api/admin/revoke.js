// api/admin/revoke.js
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

  const { key } = req.query;
  if (!key) return res.status(400).json({ error: 'Missing key' });

  const dataRaw = await get(`api_key:${key}`);
  if (!dataRaw) return res.status(404).json({ error: 'Key not found' });

  const data = JSON.parse(dataRaw);
  data.revoked = true;
  data.revokedAt = new Date().toISOString();
  await put(`api_key:${key}`, JSON.stringify(data));

  if (data.emailAddress) {
    await put(`email:${data.emailAddress}`, null);
  }

  const indexRaw = await get('user_index');
  if (indexRaw) {
    const userIndex = JSON.parse(indexRaw);
    const entry = userIndex.find(u => u.apiKey === key);
    if (entry) entry.revoked = true;
    await put('user_index', JSON.stringify(userIndex));
  }

  res.json({ success: true, message: 'Key revoked' });
};
