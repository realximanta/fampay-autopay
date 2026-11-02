package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
)

//go:embed templates/*
var templateFS embed.FS

var (
	indexTmpl     *template.Template
	dashboardTmpl *template.Template
	adminTmpl     *template.Template
)

func main() {
	var err error
	indexTmpl, err = template.ParseFS(templateFS, "templates/index.html")
	if err != nil {
		log.Fatal("Failed to parse index template: ", err)
	}
	dashboardTmpl, err = template.ParseFS(templateFS, "templates/dashboard.html")
	if err != nil {
		log.Fatal("Failed to parse dashboard template: ", err)
	}
	adminTmpl, err = template.ParseFS(templateFS, "templates/admin.html")
	if err != nil {
		log.Fatal("Failed to parse admin template: ", err)
	}

	// Landing Page & Release API
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		indexTmpl.Execute(w, nil)
	})
	http.HandleFunc("/api/latest-release", HandleLatestRelease)

	// Public API & Dashboard
	http.HandleFunc("/api/login", HandleLogin)
	http.HandleFunc("/api/logout", HandleLogout)
	http.HandleFunc("/api/auth/callback", HandleCallback)
	http.HandleFunc("/api/me", HandleGetMe)
	http.HandleFunc("/api/revoke", HandleRevoke)
	http.HandleFunc("/api/qr", HandleGenerateQR)
	http.HandleFunc("/api/verify", HandleVerify)
	http.HandleFunc("/api/recharge/verify", HandleRechargeVerify)
	http.HandleFunc("/qr", HandleQR)
	
	// Admin API
	http.HandleFunc("/api/admin/login", HandleAdminLogin)
	http.HandleFunc("/api/admin/logout", HandleAdminLogout)
	http.HandleFunc("/api/admin/users", HandleAdminUsers)
	http.HandleFunc("/api/admin/expiry", HandleAdminUpdateExpiry)
	http.HandleFunc("/api/admin/revoke", HandleAdminRevoke)
	
	// HTML Pages
	http.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		dashboardTmpl.Execute(w, nil)
	})
	http.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		adminTmpl.Execute(w, nil)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Will be updated to serve privacy.html and terms.html in the next step
	http.HandleFunc("/privacy", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<h1>Privacy Policy</h1><p>We only read Gmail to verify FamApp payments. We do not store personal emails.</p>"))
	})
	http.HandleFunc("/terms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<h1>Terms of Service</h1><p>API keys are non-refundable. Do not abuse the service.</p>"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("🚀 Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
