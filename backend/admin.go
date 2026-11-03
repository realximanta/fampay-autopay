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

type AdminUser struct {
	Email     string `json:"email"`
	APIKey    string `json:"api_key"`
	ExpiresAt int64  `json:"expires_at"`
	DaysLeft  int    `json:"days_left"`
}

// ListAllUsers fetches every user using cursor-based pagination
func ListAllUsers() ([]AdminUser, error) {
	var allUsers []AdminUser
	var cursor string

	for {
		url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/keys?prefix=user:", cfAccountID, cfNamespaceID)
		if cursor != "" {
			url += "&cursor=" + cursor
		}

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+cfAPIToken)
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		var result struct {
			Result []struct {
				Name string `json:"name"`
			} `json:"result"`
			Cursor string `json:"cursor"`
		}
		
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("KV list failed with status %d", resp.StatusCode)
		}

		json.Unmarshal(body, &result)

		for _, key := range result.Result {
			email := strings.TrimPrefix(key.Name, "user:")
			var data KeyData
			if err := KVGet("user:"+email, &data); err == nil {
				var apiKey string
				KVGet("email_key:"+email, &apiKey)
				
				daysLeft := int((data.ExpiresAt - time.Now().Unix()) / 86400)
				if daysLeft < 0 {
					daysLeft = 0
				}
				
				allUsers = append(allUsers, AdminUser{
					Email:     email,
					APIKey:    apiKey,
					ExpiresAt: data.ExpiresAt,
					DaysLeft:  daysLeft,
				})
			}
		}

		cursor = result.Cursor
		if cursor == "" {
			break // No more pages
		}
	}

	return allUsers, nil
}

func LogAdminAction(ip string, action string, details string) {
	logEntry := map[string]string{
		"ts":      time.Now().Format(time.RFC3339),
		"ip":      ip,
		"action":  action,
		"details": details,
	}
	KVPut("audit:"+time.Now().Format("20060102150405")+":"+action, logEntry, 7776000)
}

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
	KVPut("admin_session:"+sessionToken, "valid", 28800) 

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

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(users)
}

func HandleAdminUpdateExpiry(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("admin_session")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var val string
	if err := KVGet("admin_session:"+cookie.Value, &val); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var req struct {
		Email string `json:"email"`
		Days  int    `json:"days"`
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

// HandleAdminRevoke requires a POST request and valid admin session.
func HandleAdminRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie("admin_session")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	
	var val string
	if err := KVGet("admin_session:"+cookie.Value, &val); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	email := r.URL.Query().Get("email")
	if email == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var oldKeyStr string
	if err := KVGet("email_key:"+email, &oldKeyStr); err == nil {
		KVDelete("key:" + oldKeyStr)
	}

	newKeyStr := generateSecureRandomString(20)
	var userKey KeyData
	KVGet("user:"+email, &userKey)
	
	KVPut("key:"+newKeyStr, userKey, 0)
	KVPut("email_key:"+email, newKeyStr, 0)

	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	LogAdminAction(ip, "admin_revoke", "Revoked key for "+email)
	
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}
