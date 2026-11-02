package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
)

const adminHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Admin Console - Auto-Pay</title>
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@600&display=swap" rel="stylesheet">
	<style>
		body { font-family: 'Inter', sans-serif; background: #0b1120; color: #e2e8f0; margin: 0; padding: 20px; }
		.card { background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.08); border-radius: 16px; padding: 30px; max-width: 1000px; margin: 0 auto; }
		h1 { color: #60a5fa; margin-top: 0; }
		input { padding: 10px; border-radius: 8px; border: 1px solid #334155; background: #0f172a; color: white; width: 100%; margin-bottom: 10px; box-sizing: border-box; }
		button { background: #3b82f6; color: white; border: none; padding: 10px 20px; border-radius: 8px; font-weight: bold; cursor: pointer; }
		button:hover { background: #2563eb; }
		table { width: 100%; border-collapse: collapse; margin-top: 20px; }
		th, td { padding: 12px; text-align: left; border-bottom: 1px solid #1e293b; font-size: 14px; }
		th { background: rgba(255,255,255,0.05); color: #94a3b8; }
		.btn-sm { padding: 5px 10px; font-size: 12px; margin-right: 5px; }
		.btn-danger { background: #ef4444; }
		.btn-danger:hover { background: #dc2626; }
		.key { font-family: 'JetBrains Mono', monospace; color: #facc15; }
		.hidden { display: none !important; }
	</style>
</head>
<body>
	<div class="card" id="loginView">
		<h1>Admin Login</h1>
		<input type="password" id="adminPass" placeholder="Enter Admin Password" />
		<button onclick="login()">Login</button>
		<p id="loginError" style="color: #ef4444; display: none;">Invalid password</p>
	</div>

	<div class="card hidden" id="panelView">
		<div style="display: flex; justify-content: space-between; align-items: center;">
			<h1>User Management</h1>
			<div>
				<button onclick="loadUsers()">Refresh</button>
				<a href="/api/admin/logout"><button class="btn-danger">Logout</button></a>
			</div>
		</div>
		<table>
			<thead>
				<tr>
					<th>Email</th>
					<th>API Key</th>
					<th>Days Left</th>
					<th>Actions (Modify Days)</th>
				</tr>
			</thead>
			<tbody id="userTable"></tbody>
		</table>
	</div>

	<script>
		async function login() {
			const pass = document.getElementById('adminPass').value;
			const res = await fetch('/api/admin/login', {
				method: 'POST',
				body: JSON.stringify({password: pass})
			});
			if (res.ok) {
				document.getElementById('loginView').classList.add('hidden');
				document.getElementById('panelView').classList.remove('hidden');
				loadUsers();
			} else {
				document.getElementById('loginError').style.display = 'block';
			}
		}

		async function loadUsers() {
			const res = await fetch('/api/admin/users');
			if (!res.ok) return window.location.reload();
			const users = await res.json();
			
			const tbody = document.getElementById('userTable');
			tbody.innerHTML = '';
			
			users.forEach(u => {
				const tr = document.createElement('tr');
				tr.innerHTML = ` + "`" + `
					<td>${u.email}</td>
					<td class="key">${u.api_key || 'NO KEY'}</td>
					<td>${u.days_left}</td>
					<td>
						<button class="btn-sm" onclick="updateExp('${u.email}', 14)">+14d</button>
						<button class="btn-sm" onclick="updateExp('${u.email}', 30)">+30d</button>
						<button class="btn-sm btn-danger" onclick="updateExp('${u.email}', -1)">-1d</button>
					</td>
				` + "`" + `;
				tbody.appendChild(tr);
			});
		}

		async function updateExp(email, days) {
			if(!confirm("Update " + email + " expiry by " + days + " days?")) return;
			await fetch('/api/admin/expiry', {
				method: 'POST',
				body: JSON.stringify({email: email, days: days})
			});
			loadUsers();
		}
	</script>
</body>
</html>
`

const dashboardHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Auto-Pay Dashboard</title>
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@600&display=swap" rel="stylesheet">
	<style>
		:root { --bg: #0b1120; --card-bg: rgba(255, 255, 255, 0.03); --border: rgba(255, 255, 255, 0.08); --primary: #3b82f6; --success: #22c55e; --danger: #f87171; --text-main: #e2e8f0; --text-muted: #94a3b8; }
		* { box-sizing: border-box; margin: 0; padding: 0; }
		body { font-family: 'Inter', sans-serif; background: radial-gradient(ellipse at 50% 0%, #1a2639 0%, var(--bg) 70%); color: var(--text-main); min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 20px; }
		.card { background: var(--card-bg); backdrop-filter: blur(16px); border: 1px solid var(--border); border-radius: 24px; padding: 40px 30px; width: 100%; max-width: 440px; box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5); text-align: center; }
		h1 { font-size: 24px; font-weight: 700; margin-bottom: 24px; background: linear-gradient(135deg, #60a5fa, #a78bfa); -webkit-background-clip: text; -webkit-text-fill-color: transparent; }
		.user-info { display: flex; align-items: center; justify-content: center; gap: 10px; margin-bottom: 20px; font-size: 14px; color: var(--text-muted); }
		.badge { background: rgba(34, 197, 94, 0.15); color: var(--success); padding: 4px 12px; border-radius: 100px; font-size: 12px; font-weight: 600; border: 1px solid rgba(34, 197, 94, 0.2); }
		.badge.expired { background: rgba(248, 113, 113, 0.15); color: var(--danger); border-color: rgba(248, 113, 113, 0.2); }
		.key-box { background: rgba(0, 0, 0, 0.3); border: 1px solid var(--border); border-radius: 16px; padding: 20px; margin-bottom: 24px; }
		.key-label { font-size: 11px; text-transform: uppercase; letter-spacing: 1px; color: var(--text-muted); margin-bottom: 8px; }
		.api-key { font-family: 'JetBrains Mono', monospace; font-size: 20px; color: #facc15; letter-spacing: 2px; }
		button { background: var(--primary); color: white; border: none; padding: 12px 24px; border-radius: 12px; font-size: 14px; font-weight: 600; cursor: pointer; transition: all 0.2s ease; width: 100%; font-family: 'Inter', sans-serif; }
		button:hover { background: #2563eb; transform: translateY(-1px); }
		button.danger { background: transparent; border: 1px solid rgba(248, 113, 113, 0.3); color: var(--danger); margin-bottom: 24px; }
		button.danger:hover { background: rgba(248, 113, 113, 0.1); }
		button.success { background: var(--success); }
		button.success:hover { background: #16a34a; }
		.divider { height: 1px; background: var(--border); margin: 30px 0; }
		.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-bottom: 24px; }
		.qr-container { display: none; background: #ffffff; padding: 16px; border-radius: 16px; margin-top: 20px; box-shadow: 0 10px 25px -5px rgba(0,0,0,0.5); }
		.qr-container img { width: 100%; display: block; }
		.qr-text { font-size: 14px; margin-bottom: 12px; color: var(--text-main); font-weight: 600; }
		.qr-amount { color: #facc15; }
		.footer-links { display: flex; justify-content: center; gap: 20px; margin-top: 30px; font-size: 13px; }
		.footer-links a { color: var(--text-muted); text-decoration: none; }
		.footer-links a:hover { color: var(--text-main); }
	</style>
</head>
<body>
	<div class="card">
		<h1>⚡ Auto-Pay Dashboard</h1>
		<div class="user-info">
			<span id="email">Loading...</span>
			<span id="statusBadge" class="badge">LOADING</span>
		</div>
		<p style="margin-bottom: 24px; font-size: 15px;">Days Left: <b id="days" style="color: #fff;">0</b></p>
		<div class="key-box">
			<div class="key-label">Your API Key</div>
			<div id="apikey" class="api-key">Loading...</div>
		</div>
		<button class="danger" onclick="revoke()">Revoke & Generate New Key</button>
		<div class="divider"></div>
		<p style="font-weight: 600; margin-bottom: 16px; font-size: 15px;">Recharge Subscription</p>
		<div class="grid">
			<button class="success" onclick="recharge('biweekly')">₹10 <br><span style="font-size:11px;font-weight:400;">14 Days</span></button>
			<button class="success" onclick="recharge('monthly')">₹20 <br><span style="font-size:11px;font-weight:400;">30 Days</span></button>
		</div>
		<div id="qrArea">
			<div class="qr-text">Scan to Pay Exactly: <span id="payAmount" class="qr-amount"></span></div>
			<div class="qr-container" id="qrContainer">
				<img id="qrImg" />
			</div>
			<p id="qrStatus" style="font-size: 12px; color: #60a5fa; margin-top: 12px; display: none;">⏳ Waiting for payment confirmation...</p>
		</div>
		<div class="footer-links">
			<a href="/privacy" target="_blank">Privacy Policy</a>
			<a href="/terms" target="_blank">Terms of Service</a>
			<a href="/api/logout">Logout</a>
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
				badge.innerText = "ACTIVE"; badge.className = "badge";
			} else {
				badge.innerText = "EXPIRED"; badge.className = "badge expired";
			}
		}
		async function revoke() {
			if(!confirm("Are you sure? Your old API key will stop working immediately.")) return;
			await fetch('/api/revoke', {method: 'POST'});
			load();
		}
		let pollInterval = null;
		async function recharge(plan) {
			if(pollInterval) clearInterval(pollInterval);
			const res = await fetch('/api/qr', { method: 'POST', body: JSON.stringify({plan_id: plan}) });
			const data = await res.json();
			document.getElementById('payAmount').innerText = '₹' + data.fingerprint;
			document.getElementById('qrImg').src = 'data:image/png;base64,' + data.qr_base64;
			document.getElementById('qrContainer').style.display = 'block';
			document.getElementById('qrStatus').style.display = 'block';
			pollInterval = setInterval(async () => {
				const v = await fetch('/api/verify', { method: 'POST', body: JSON.stringify({fingerprint: data.fingerprint}) });
				const vData = await v.json();
				if (vData.status === 'exact') {
					clearInterval(pollInterval);
					alert('Payment Successful! Subscription Updated.');
					document.getElementById('qrContainer').style.display = 'none';
					document.getElementById('qrStatus').style.display = 'none';
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
	// Public API & Dashboard
	http.HandleFunc("/api/login", HandleLogin)
	http.HandleFunc("/api/logout", HandleLogout)
	http.HandleFunc("/api/auth/callback", HandleCallback)
	http.HandleFunc("/api/me", HandleGetMe)
	http.HandleFunc("/api/revoke", HandleRevoke)
	http.HandleFunc("/api/qr", HandleGenerateQR)
	http.HandleFunc("/api/verify", HandleVerify)
	http.HandleFunc("/qr", HandleQR)
	
	// Admin API
	http.HandleFunc("/api/admin/login", HandleAdminLogin)
	http.HandleFunc("/api/admin/logout", HandleAdminLogout)
	http.HandleFunc("/api/admin/users", HandleAdminUsers)
	http.HandleFunc("/api/admin/expiry", HandleAdminUpdateExpiry)
	
	// HTML Pages
	http.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		t, _ := template.New("dash").Parse(dashboardHTML)
		t.Execute(w, nil)
	})
	http.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		t, _ := template.New("admin").Parse(adminHTML)
		t.Execute(w, nil)
	})

	// Health check for UptimeRobot
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Legal pages
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
