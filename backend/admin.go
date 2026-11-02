package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ListAllUsers fetches every user from Cloudflare KV (prefix "user:")
func ListAllUsers() ([]KeyData, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/keys?prefix=user:", cfAccountID, cfNamespaceID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfAPIToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Result []struct {
			Name string `json:"name"`
		} `json:"result"`
	}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)

	var users []KeyData
	for _, key := range result.Result {
		email := strings.TrimPrefix(key.Name, "user:")
		var data KeyData
		if err := KVGet("user:"+email, &data); err == nil {
			users = append(users, data)
		}
	}
	return users, nil
}

// P0 7.4: Log every admin action to KV (90 days retention)
func LogAdminAction(ip string, action string, details string) {
	logEntry := map[string]string{
		"ts":      time.Now().Format(time.RFC3339),
		"ip":      ip,
		"action":  action,
		"details": details,
	}
	KVPut("audit:"+time.Now().Format("20060102150405")+":"+action, logEntry, 7776000)
}

// HandleAdminLogin verifies password using bcrypt (P0 2.10)
func HandleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "Invalid request"}`))
		return
	}

	// IMPORTANT: You must set ADMIN_PASSWORD_HASH in Render env vars
	hash := os.Getenv("ADMIN_PASSWORD_HASH")
	if hash == "" {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Admin password hash not configured"}`))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Invalid password"}`))
		return
	}

	sessionToken := generateSecureRandomString(32)
	// P0 2.9: Admin session 8h TTL
	KVPut("admin_session:"+sessionToken, "true", 28800)

	// P0 2.9: SameSite=Strict for Admin
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   28800,
	})

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func HandleAdminLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("admin_session")
	if err == nil {
		KVDelete("admin_session:" + cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "admin_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// HandleAdminUsers lists all users securely (P0 7.12)
func HandleAdminUsers(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("admin_session")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Unauthorized"}`))
		return
	}

	var val string
	if err := KVGet("admin_session:"+cookie.Value, &val); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Session expired"}`))
		return
	}

	users, err := ListAllUsers()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Failed to fetch users"}`))
		return
	}

	// P0 7.12: Never send encrypted tokens to the frontend
	for i := range users {
		users[i].EncryptedToken = "HIDDEN"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// HandleAdminUpdateExpiry allows admins to add/remove days from a user
func HandleAdminUpdateExpiry(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("admin_session")
	if err != nil || KVGet("admin_session:"+cookie.Value, nil) != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var req struct {
		Email string `json:"email"`
		Days  int    `json:"days"` // Can be positive or negative
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var userKey KeyData
	if err := KVGet("user:"+req.Email, &userKey); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if userKey.ExpiresAt < time.Now().Unix() {
		userKey.ExpiresAt = time.Now().Unix()
	}
	userKey.ExpiresAt += int64(req.Days * 86400)
	KVPut("user:"+req.Email, userKey, 0)

	// Sync the expiry to their API key record
	var keyStr string
	if err := KVGet("email_key:"+req.Email, &keyStr); err == nil {
		KVPut("key:"+keyStr, userKey, 0)
	}

	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	LogAdminAction(ip, "update_expiry", fmt.Sprintf("Updated %s by %d days", req.Email, req.Days))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}
