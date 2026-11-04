export default {
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

    // 2. Strict Sender Validation
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

    // 3. Parse Amount and UTR / Transaction ID
    // FIXED: Handles 1.0 or 1.00, and matches HTML tags/spaces between text
    const amountMatch = rawEmail.match(/(?:You have successfully received|Received)[\s\S]*?(?:₹|Rs\.?|INR)?\s*([\d]+(?:\.\d{1,2})?)/i);
    // FIXED: Looks for both UTR and Transaction ID
    const utrMatch = rawEmail.match(/(?:UTR|Transaction ID)\s*:\s*([A-Za-z0-9]+)/i);
    
    if (amountMatch && amountMatch[1] && utrMatch && utrMatch[1]) {
        let amount = amountMatch[1];
        const utr = utrMatch[1];
        
        // Ensure amount is formatted with two decimal places (e.g., 1.0 -> 1.00)
        if (!amount.includes('.')) {
            amount = amount + '.00';
        } else {
            const parts = amount.split('.');
            if (parts[1].length === 1) amount = amount + '0';
        }

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
        
        console.log(`✅ Verified FamPay ₹${amount} (UTR: ${utr}) saved to KV.`);
    } else {
        console.log("⚠️ FamPay email passed auth but format didn't match.");
        console.log("Amount Match:", amountMatch ? amountMatch[1] : "None");
        console.log("UTR Match:", utrMatch ? utrMatch[1] : "None");
    }
  }
}
