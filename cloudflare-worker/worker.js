export default {
  async email(message, env, ctx) {
    const rawEmail = await new Response(message.raw).text();

    // 1. Automatically verify Google Forwarding
    if (message.from.includes("forwarding-noreply@google.com")) {
        const match = rawEmail.match(/https:\/\/mail\.google\.com\/mail\/vf-[^\s"']+/);
        if (match && match[0]) {
            await fetch(match[0]); // Clicks the verification link
            console.log("✅ Google forwarding verified.");
        }
        return;
    }

    // 2. Parse FamApp Payment Email
    // Matches "You have successfully received ₹10.05" or similar
    const amountMatch = rawEmail.match(/received\s*(?:₹|Rs\.?|INR)?\s*([\d]+\.[\d]{2})/i);
    if (amountMatch && amountMatch[1]) {
        const fingerprint = amountMatch[1]; // e.g. "10.25"
        
        // Save to KV instantly (Expires in 15 mins)
        await env.PAYMENT_KV.put(`payment:${fingerprint}`, JSON.stringify({
            received_at: Date.now(),
            status: "paid"
        }), { expirationTtl: 900 });
        
        console.log(`✅ Payment ₹${fingerprint} saved to KV.`);
    } else {
        console.log("⚠️ Email did not match FamApp payment format.");
    }
  }
}
