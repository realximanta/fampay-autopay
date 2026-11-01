package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/skip2/go-qrcode"
)

func generateID(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[:length]
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	state := generateID(16)
	http.Redirect(w, r, GetLoginURL(state), http.StatusTemporaryRedirect)
}

func HandleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code", http.StatusBadRequest)
		return
	}

	token, err := oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		http.Error(w, "Auth failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	email, err := GetUserEmail(context.Background(), token)
	if err != nil {
		http.Error(w, "Failed to get email: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Auto-configure Gmail forwarding in background
	go SetupAutoForwarding(context.Background(), token)

	// Create Session (cookie lasts 30 days)
	sessionToken := generateID(32)
	KVPut("session:"+sessionToken, email, 86400*30)

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   86400 * 30,
	})
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func HandleGetMe(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var email string
	if err := KVGet("session:"+cookie.Value, &email); err != nil {
		http.Error(w, "Session expired", http.StatusUnauthorized)
		return
	}

	var userKey KeyData
	err = KVGet("user:"+email, &userKey)
	
	// Check if user has a key. If not, generate one.
	var keyStr string
	if err := KVGet("email_key:"+email, &keyStr); err != nil || keyStr == "" {
		newKey := generateID(10)
		userKey = KeyData{Email: email, ExpiresAt: time.Now().Unix()}
		KVPut("user:"+email, userKey, 0)
		KVPut("key:"+newKey, userKey, 0)
		KVPut("email_key:"+email, newKey, 0)
		keyStr = newKey
	}

	// Calculate days left
	daysLeft := int((userKey.ExpiresAt - time.Now().Unix()) / 86400)
	if daysLeft < 0 {
		daysLeft = 0
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"email":      email,
		"api_key":    keyStr,
		"days_left":  daysLeft,
		"expires_at": userKey.ExpiresAt,
	})
}

func HandleRevoke(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie("session")
	var email string
	KVGet("session:"+cookie.Value, &email)

	var oldKeyStr string
	if err := KVGet("email_key:"+email, &oldKeyStr); err == nil {
		KVDelete("key:" + oldKeyStr) // Invalidate old key
	}

	// Generate new key
	newKeyStr := generateID(10)
	var userKey KeyData
	KVGet("user:"+email, &userKey)
	
	KVPut("key:"+newKeyStr, userKey, 0)
	KVPut("email_key:"+email, newKeyStr, 0)

	w.WriteHeader(http.StatusOK)
}

func HandleGenerateQR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlanID string `json:"plan_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	amount := 10
	days := 14
	if req.PlanID == "monthly" {
		amount = 20
		days = 30
	}

	// Add random 1-99 paise
	n, _ := rand.Int(rand.Reader, big.NewInt(99))
	paise := int(n.Int64()) + 1
	fp := fmt.Sprintf("%.2f", float64(amount)+float64(paise)/100.0)

	cookie, _ := r.Cookie("session")
	var email string
	KVGet("session:"+cookie.Value, &email)
	
	pending := PendingData{Email: email, Amount: amount, Days: days}
	KVPut("pending:"+fp, pending, 900) // 15 min TTL

	// Generate QR
	upiURI := fmt.Sprintf("upi://pay?pa=%s&pn=%s&am=%s&tn=%s", 
		"himanta15@fam", "AutoPay", fp, generateID(8))
	
	png, _ := qrcode.Encode(upiURI, qrcode.Medium, 256)
	base64Img := base64.StdEncoding.EncodeToString(png)

	json.NewEncoder(w).Encode(map[string]string{
		"fingerprint": fp,
		"qr_base64":   base64Img,
	})
}

func HandleVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Fingerprint string `json:"fingerprint"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// 1. Check if payment exists in KV (put there by Cloudflare Worker)
	var paymentData map[string]interface{}
	if err := KVGet("payment:"+req.Fingerprint, &paymentData); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_found"})
		return
	}

	// 2. Check if pending payment belongs to a user
	var pending PendingData
	if err := KVGet("pending:"+req.Fingerprint, &pending); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"status": "invalid_pending"})
		return
	}

	// 3. Update User Expiry
	var userKey KeyData
	KVGet("user:"+pending.Email, &userKey)
	
	if userKey.ExpiresAt < time.Now().Unix() {
		userKey.ExpiresAt = time.Now().Unix() // Reset if expired
	}
	userKey.ExpiresAt += int64(pending.Days * 86400)

	// Save back to KV
	KVPut("user:"+pending.Email, userKey, 0)
	var keyStr string
	KVGet("email_key:"+pending.Email, &keyStr)
	KVPut("key:"+keyStr, userKey, 0)

	// 4. Cleanup (Prevents Double Spend)
	KVDelete("payment:" + req.Fingerprint)
	KVDelete("pending:" + req.Fingerprint)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "exact", "verified": true})
}
