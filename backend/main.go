package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
)

// (Keep the dashboardHTML const from the previous step here. I've omitted it for brevity in this message, but DO NOT delete it from your file!)
const dashboardHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Auto-Pay Dashboard</title>
	<!-- ... YOUR EXISTING HTML ... -->
</head>
<body>
	<!-- ... YOUR EXISTING BODY ... -->
</body>
</html>
`

func main() {
	// OAuth & Dashboard Routes
	http.HandleFunc("/api/login", HandleLogin)
	http.HandleFunc("/api/logout", HandleLogout)
	http.HandleFunc("/api/auth/callback", HandleCallback)
	http.HandleFunc("/api/me", HandleGetMe)
	http.HandleFunc("/api/revoke", HandleRevoke)
	
	// Payment & QR Routes
	http.HandleFunc("/api/qr", HandleGenerateQR) // Dashboard uses this for recharge
	http.HandleFunc("/qr", HandleQR)             // NEW: Public API endpoint for bots
	
	// Verification Route
	http.HandleFunc("/api/verify", HandleVerify)
	
	// Serve Dashboard
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
