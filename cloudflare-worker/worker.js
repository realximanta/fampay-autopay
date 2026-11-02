export default {
  async email(message, env, ctx) {
    const rawEmail = await new Response(message.raw).text();
    
    const authResultsMatch = rawEmail.match(/Authentication-Results:\s*(.*?)\r?\n/i);
    if (!authResultsMatch) return;
    const authResults = authResultsMatch[1].toLowerCase();
    if (!authResults.includes("spf=pass") || !authResults.includes("dkim=pass") || !authResults.includes("dmarc=pass")) return;

    if (message.from.includes("forwarding-noreply@google.com")) {
        const match = rawEmail.match(/https:\/\/mail\.google\.com\/mail\/vf-[^\s"']+/);
        if (match && match[0]) await fetch(match[0]);
        return;
    }

    const amountMatch = rawEmail.match(/(?:You have successfully received|Received)\s*(?:₹|Rs\.?|INR)?\s*(\d{1,6}\.\d{2})/i);
    const utrMatch = rawEmail.match(/UTR:\s*(\d+)/i);
    
    if (amountMatch && amountMatch[1] && utrMatch && utrMatch[1]) {
        const amount = amountMatch[1];
        const utr = utrMatch[1];
        const ts = Date.now();
        
        const secret = env.KV_HMAC_SECRET; 
        const encoder = new TextEncoder();
        const key = await crypto.subtle.importKey(
            "raw", encoder.encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]
        );
        
        const payloadStr = `${amount}:${ts}:${utr}`;
        const signatureBuffer = await crypto.subtle.sign("HMAC", key, encoder.encode(payloadStr));
        const signature = Array.from(new Uint8Array(signatureBuffer)).map(b => b.toString(16).padStart(2, '0')).join('');

        await env.PAYMENT_KV.put(`payment:${amount}`, JSON.stringify({
            amount: amount,
            timestamp: ts,
            utr: utr,
            signature: signature
        }), { expirationTtl: 1800 });
        
        console.log(`✅ Verified ₹${amount} (UTR: ${utr}) saved to KV.`);
    }
  }
}
