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
	docsTmpl      *template.Template
	privacyTmpl   *template.Template
	termsTmpl     *template.Template
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
	docsTmpl, err = template.ParseFS(templateFS, "templates/docs.html")
	if err != nil {
		log.Fatal("Failed to parse docs template: ", err)
	}
	privacyTmpl, err = template.ParseFS(templateFS, "templates/privacy.html")
	if err != nil {
		log.Fatal("Failed to parse privacy template: ", err)
	}
	termsTmpl, err = template.ParseFS(templateFS, "templates/terms.html")
	if err != nil {
		log.Fatal("Failed to parse terms template: ", err)
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
	http.HandleFunc("/api/history", HandleGetHistory) // NEW ROUTE
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
	http.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		docsTmpl.Execute(w, nil)
	})
	http.HandleFunc("/privacy", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		privacyTmpl.Execute(w, nil)
	})
	http.HandleFunc("/terms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		termsTmpl.Execute(w, nil)
	})

	http.HandleFunc("/api/download-prompt", func(w http.ResponseWriter, r *http.Request) {
		data, err := templateFS.ReadFile("templates/ai_prompt.md")
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=\"auto-pay-prompt.md\"")
		w.Header().Set("Content-Type", "text/markdown")
		w.Write(data)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("🚀 Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
