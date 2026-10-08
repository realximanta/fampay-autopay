 // lib/config.js — reads from environment variables
// Never hardcode credentials in a public repo.

function required(name) {
  const value = process.env[name];
  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }
  return value;
}

module.exports = {
  // Google OAuth
  GOOGLE_CLIENT_ID: required('GOOGLE_CLIENT_ID'),
  GOOGLE_CLIENT_SECRET: required('GOOGLE_CLIENT_SECRET'),
  GOOGLE_REDIRECT_URI: required('GOOGLE_REDIRECT_URI'),

  // Cloudflare KV
  CLOUDFLARE_ACCOUNT_ID: required('CLOUDFLARE_ACCOUNT_ID'),
  CLOUDFLARE_KV_NAMESPACE: required('CLOUDFLARE_KV_NAMESPACE'),
  CLOUDFLARE_API_TOKEN: required('CLOUDFLARE_API_TOKEN'),

  // Base URL
  BASE_URL: required('BASE_URL'),

  // Admin panel
  ADMIN_PASSWORD: required('ADMIN_PASSWORD'),
  SUPPORT_LINK: process.env.SUPPORT_LINK || 'https://t.me/',

  // Rate limiting
  RATE_LIMIT_PER_MINUTE: parseInt(process.env.RATE_LIMIT_PER_MINUTE || '30', 10),

  // Consume marker TTL (seconds)
  CONSUME_TTL_SECONDS: parseInt(process.env.CONSUME_TTL_SECONDS || '900', 10),

  // FamApp sender domains for the Gmail search filter
  FAMAPP_SENDER_DOMAINS: [
    'famapp.in',
    'famx.fampay.in',
    'famapp.co.in',
  ],
};
