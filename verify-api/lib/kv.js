// lib/kv.js — Cloudflare KV client (REST API)
const axios = require('axios');
const config = require('./config');

const ACCOUNT_ID = config.CLOUDFLARE_ACCOUNT_ID;
const NAMESPACE_ID = config.CLOUDFLARE_KV_NAMESPACE;
const API_TOKEN = config.CLOUDFLARE_API_TOKEN;

const BASE_URL = `https://api.cloudflare.com/client/v4/accounts/${ACCOUNT_ID}/storage/kv/namespaces/${NAMESPACE_ID}/values`;

async function get(key) {
  const url = `${BASE_URL}/${key}`;
  try {
    const response = await axios.get(url, {
      headers: { Authorization: `Bearer ${API_TOKEN}` },
      validateStatus: null,
      responseType: 'text',
    });
    if (response.status === 404) return null;
    if (response.status !== 200) {
      console.error(`KV get error: ${response.status} - ${response.data}`);
      return null;
    }
    return response.data;
  } catch (error) {
    console.error('KV get exception:', error.message);
    return null;
  }
}

async function put(key, value, options = {}) {
  const url = `${BASE_URL}/${key}`;
  try {
    const params = {};
    if (options.expirationTtl) params.expiration_ttl = options.expirationTtl;
    const response = await axios.put(url, value, {
      headers: {
        Authorization: `Bearer ${API_TOKEN}`,
        'Content-Type': 'application/json',
      },
      params,
    });
    if (response.status !== 200) {
      console.error(`KV put error: ${response.status} - ${response.data}`);
    }
    return response;
  } catch (error) {
    console.error('KV put exception:', error.message);
    throw error;
  }
}

module.exports = { get, put };
