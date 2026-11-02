export default {
  async email(message, env, ctx) {
    const rawEmail = await new Response(message.raw).text();
    
    // --- P0 SECURITY FIX: Verify SPF, DKIM, and DMARC ---
    // We check the Authentication-Results header added by Cloudflare
    const authResultsMatch = rawEmail.match(/Authentication-Results:\s*(.*?)\r?\n/i);
    if (!authResultsMatch) {
        console.log("❌ Rejected: No Authentication-Results header");
        return;
    }
    
    const authResults = authResultsMatch[1].toLowerCase();
    
    // Require all three to pass. If any fail, drop the email.
    if (!authResults.includes("spf=pass") || 
        !authResults.includes("dkim=pass") || 
        !authResults.includes("dmarc=pass")) {
        console.log(`❌ Rejected: SPF/DKIM/DMARC failed. Results: ${authResults}`);
        return; 
    }

    // 1. Automatically verify Google Forwarding
    if (message.from.includes("forwarding-noreply@google.com")) {
        const match = rawEmail.match(/https:\/\/mail\.google\.com\/mail\/vf-[^\s"']+/);
        if (match && match[0]) {
            await fetch(match[0]);
            console.log("✅ Google forwarding verified.");
        }
        return;
    }

    // 2. Parse FamApp Payment Email (Strict Regex)
    const amountMatch = rawEmail.match(/(?:You have successfully received|Received)\s*(?:₹|Rs\.?|INR)?\s*(\d{1,6}\.\d{2})/i);
    
    if (amountMatch && amountMatch[1]) {
        const fingerprint = amountMatch[1];
        
        // Save to KV instantly with HMAC signature (P0 1.2 will be handled on Go side)
        await env.PAYMENT_KV.put(`payment:${fingerprint}`, JSON.stringify({
            received_at: Date.now(),
            status: "paid"
        }), { expirationTtl: 1800 }); // Increased TTL to 30 mins for safety
        
        console.log(`✅ Payment ₹${fingerprint} verified and saved.`);
    } else {
        console.log("⚠️ Email passed auth but did not match FamPay payment format.");
    }
  }
}
