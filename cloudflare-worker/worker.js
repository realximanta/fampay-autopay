export default {
  // This stops the 500 error when you visit the URL in a browser
  async fetch(request, env, ctx) {
    return new Response("Auto-Pay Email Worker is running perfectly!", { 
      status: 200, 
      headers: { "content-type": "text/plain" } 
    });
  },

  async email(message, env, ctx) {
    console.log(`📩 Received email from: ${message.from}`);
    const rawEmail = await new Response(message.raw).text();

    // 1. Automatically verify Google Forwarding
    if (message.from.includes("forwarding-noreply@google.com")) {
        const match = rawEmail.match(/https:\/\/mail\.google\.com\/mail\/vf-[^\s"']+/);
        if (match && match[0]) {
            await fetch(match[0]);
            console.log("✅ Google forwarding verified.");
        }
        return;
    }

    // 2. Strict Sender Validation (FamPay Domain)
    const fromMatch = rawEmail.match(/^From:\s*(.*)$/m);
    if (!fromMatch) {
        console.log("❌ Rejected: No From header found.");
        return;
    }
    
    const fromHeader = fromMatch[1].toLowerCase();
    const isFamPay = fromHeader.includes("famapp.in") || 
                     fromHeader.includes("fampay.in") || 
                     fromHeader.includes("famx.fampay.in");

    if (!isFamPay) {
        console.log(`❌ Rejected: Sender is not FamPay. From: ${fromHeader}`);
        return;
    }

    // 3. Parse Amount and UTR
    const amountMatch = rawEmail.match(/(?:You have successfully received|Received)\s*(?:₹|Rs\.?|INR)?\s*(\d{1,6}\.\d{2})/i);
    const utrMatch = rawEmail.match(/UTR:\s*(\d+)/i);
    
    if (amountMatch && amountMatch[1] && utrMatch && utrMatch[1]) {
        const amount = amountMatch[1];
        const utr = utrMatch[1];
        const ts = Date.now();
        
        const secret = env.KV_HMAC_SECRET; 
        if (!secret) {
            console.log("❌ Rejected: KV_HMAC_SECRET not set.");
            return;
        }

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
        }), { expirationTtl: 86400 });
        
        console.log(`✅ Verified FamPay ₹${amount} saved to KV.`);
    } else {
        console.log("⚠️ FamPay email passed auth but format didn't match.");
    }
  }
}
