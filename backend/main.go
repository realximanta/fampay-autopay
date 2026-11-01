package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
)

const dashboardHTML = `
<!DOCTYPE html>
<html>
<head>
	<title>Auto-Pay Dashboard</title>
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background: #0b1120; color: #e2e8f0; text-align: center; padding: 40px 20px; margin: 0; }
		.card { background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.1); border-radius: 16px; padding: 30px; max-width: 400px; margin: 0 auto; }
		h1 { margin-top: 0; font-size: 24px; color: #60a5fa; }
		code { background: #1e293b; padding: 8px 12px; border-radius: 6px; font-size: 18px; color: #facc15; letter-spacing: 1px; display: inline-block; margin: 10px 0; }
		button { background: #3b82f6; color: white; border: none; padding: 10px 20px; border-radius: 8px; font-size: 14px; font-weight: 600; cursor: pointer; margin: 5px; transition: 0.2s; }
		button:hover { background: #2563eb; }
		button.danger { background: transparent; border: 1px solid #f87171; color: #f87171; }
		button.danger:hover { background: rgba(248,113,113,0.1); }
		button.success { background: #22c55e; }
		button.success:hover { background: #16a34a; }
		.qr-box { display: none; margin-top: 20px; padding-top: 20px; border-top: 1px solid #1e293b; }
		img { border-radius: 12px; margin-top: 10px; border: 4px solid white; }
		.badge { background: #22c55e; color: white; padding: 4px 8px; border-radius: 12px; font-size: 12px; font-weight: bold; }
		.expired { background: #ef4444; }
	</style>
</head>
<body>
	<div class="card">
		<h1>⚡ Auto-Pay Dashboard</h1>
		<p>Signed in as: <b id="email">Loading...</b></p>
		
		<p>Status: <span id="statusBadge" class="badge">Loading</span></p>
		<p>Days Left: <b id="days">0</b></p>
		
		<div style="margin: 20px 0;">
			<p style="font-size: 12px; color: #94a3b8;">Your API Key</p>
			<code id="apikey">Loading...</code>
		</div>
		
		<button class="danger" onclick="revoke()">Revoke Key</button>
		
		<hr style="border-color: #1e293b; margin: 30px 0;">
		
		<p style="font-weight: bold; margin-bottom: 10px;">Recharge Subscription</p>
		<button class="success" onclick="recharge('biweekly')">₹10 (14 Days)</button>
		<button class="success" onclick="recharge('monthly')">₹20 (30 Days)</button>

		<div id="qrArea" class="qr-box">
			<p style="font-weight: bold;">Scan to Pay Exactly: <span id="payAmount" style="color: #facc15;"></span></p>
			<img id="qrImg" width="200" />
			<p style="font-size: 12px; color: #94a3b8;">Waiting for payment confirmation...</p>
		</div>
	</div>

	<script>
		async function load() {
			const res = await fetch('/api/me');
			if (res.status === 401) return window.location.href = '/api/login';
			const data = await res.json();
			
			document.getElementById('email').innerText = data.email;
			document.getElementById('apikey').innerText = data.api_key;
			document.getElementById('days').innerText = data.days_left;
			
			const badge = document.getElementById('statusBadge');
			if (data.days_left > 0) {
				badge.innerText = "ACTIVE";
				badge.className = "badge";
			} else {
				badge.innerText = "EXPIRED";
				badge.className = "badge expired";
			}
		}

		async function revoke() {
			if(!confirm("Are you sure? Your old API key will stop working immediately.")) return;
			await fetch('/api/revoke', {method: 'POST'});
			load();
		}

		async function recharge(plan) {
			const res = await fetch('/api/qr', {
				method: 'POST',
				body: JSON.stringify({plan_id: plan})
			});
			const data = await res.json();
			
			document.getElementById('qrArea').style.display = 'block';
			document.getElementById('payAmount').innerText = '₹' + data.fingerprint;
			document.getElementById('qrImg').src = 'data:image/png;base64,' + data.qr_base64;
			
			// Poll every 3 seconds to check if the Cloudflare Worker got the email
			const interval = setInterval(async () => {
				const v = await fetch('/api/verify', {
					method: 'POST',
					body: JSON.stringify({fingerprint: data.fingerprint})
				});
				if (v.ok) {
					clearInterval(interval);
					alert('Payment Successful! Subscription Updated.');
					document.getElementById('qrArea').style.display = 'none';
					load();
				}
			}, 3000);
		}
		load();
	</script>
</body>
</html>
`

func main() {
	http.HandleFunc("/api/login", HandleLogin)
	http.HandleFunc("/api/auth/callback", HandleCallback)
	http.HandleFunc("/api/me", HandleGetMe)
	http.HandleFunc("/api/revoke", HandleRevoke)
	http.HandleFunc("/api/qr", HandleGenerateQR)
	http.HandleFunc("/api/verify", HandleVerify)
	
	http.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		t, _ := template.New("dash").Parse(dashboardHTML)
		t.Execute(w, nil)
	})

	// Health check for UptimeRobot
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Legal pages for Google OAuth Verification
	http.HandleFunc("/privacy", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<h1>Privacy Policy</h1><p>We only read Gmail to verify FamApp payments. We do not store personal emails.</p>"))
	})
	http.HandleFunc("/terms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<h1>Terms of Service</h1><p>API keys are non-refundable. Do not abuse the service.</p>"))
	})

	// Redirect root to dashboard
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("🚀 Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
