package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
)

// Helper for generating secure random strings (used for Sessions and PKCE)
func generateSecureRandomString(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)[:length]
}

// P0 2.1 & 2.2: Handle Login with State and PKCE
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	state := generateSecureRandomString(32)
	verifier, challenge := GeneratePKCE()

	// Store verifier in KV for 10 minutes using the state as the key
	KVPut("pkce:"+state, verifier, 600)

	// Set state in a secure cookie to verify on callback
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	http.Redirect(w, r, GetLoginURL(state, challenge), http.StatusTemporaryRedirect)
}

// P0 2.7: Logout deletes KV session
func HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		KVDelete("session:" + cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func HandleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	
	if code == "" || state == "" {
		http.Error(w, "Missing code or state", http.StatusBadRequest)
		return
	}

	// Verify the state matches the cookie (P0 2.1)
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != state {
		http.Error(w, "Invalid OAuth state", http.StatusForbidden)
		return
	}

	// Retrieve the PKCE verifier from KV (P0 2.2)
	var verifier string
	if err := KVGet("pkce:"+state, &verifier); err != nil {
		http.Error(w, "Invalid or expired state", http.StatusForbidden)
		return
	}
	KVDelete("pkce:" + state) // Clean up

	token, err := ExchangeCode(context.Background(), code, verifier)
	if err != nil {
		http.Error(w, "Auth failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	email, err := GetUserEmail(context.Background(), token)
	if err != nil {
		http.Error(w, "Failed to get email: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// P0 1.8: Encrypt the refresh token before saving to KV
	encToken, err := EncryptToken(token.RefreshToken)
	if err != nil {
		http.Error(w, "Failed to secure token", http.StatusInternalServerError)
		return
	}

	// Auto-configure Gmail forwarding in background
	go SetupAutoForwarding(context.Background(), token)

	// Create Session
	sessionToken := generateSecureRandomString(32)
	KVPut("session:"+sessionToken, email, 86400*30)

	// Check if user already exists and has a key
	var userKey KeyData
	if err := KVGet("user:"+email, &userKey); err != nil {
		// First time login: generate a key
		newKey := generateSecureRandomString(20)
		userKey = KeyData{Email: email, ExpiresAt: time.Now().Unix()}
		KVPut("user:"+email, userKey, 0)
		KVPut("key:"+newKey, userKey, 0)
		KVPut("email_key:"+email, newKey, 0)
	} else {
		// Update their encrypted token if it changed
		userKey.EncryptedToken = encToken
		KVPut("user:"+email, userKey, 0)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
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
	KVGet("user:"+email, &userKey)
	
	var keyStr string
	if err := KVGet("email_key:"+email, &keyStr); err != nil || keyStr == "" {
		keyStr = "ERROR_NO_KEY"
	}

	daysLeft := int((userKey.ExpiresAt - time.Now().Unix()) / 86400)
	if daysLeft < 0 {
		daysLeft = 0
	}

	w.Header().Set("Content-Type", "application/json")
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

	newKeyStr := generateSecureRandomString(20)
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

	n, _ := rand.Int(rand.Reader, big.NewInt(99))
	paise := int(n.Int64()) + 1
	fp := fmt.Sprintf("%.2f", float64(amount)+float64(paise)/100.0)

	cookie, _ := r.Cookie("session")
	var email string
	KVGet("session:"+cookie.Value, &email)
	
	pending := PendingData{Email: email, Amount: amount, Days: days}
	KVPut("pending:"+fp, pending, 900) // 15 min TTL

	upiURI := fmt.Sprintf("upi://pay?pa=%s&pn=%s&am=%s&tn=%s", 
		"himanta15@fam", "AutoPay", fp, generateSecureRandomString(8))
	
	png, _ := qrcode.Encode(upiURI, qrcode.Medium, 256)
	base64Img := base64.StdEncoding.EncodeToString(png)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"fingerprint": fp,
		"qr_base64":   base64Img,
	})
}

// P0 1.3, 3.2, 3.3, 4.4, 4.5: The Verifier Endpoint
func HandleVerify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 1. P0 1.3: Accept key from Header OR Query Param
	apiKey := r.URL.Query().Get("key")
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		apiKey = strings.TrimPrefix(authHeader, "Bearer ")
	}

	amountStr := r.URL.Query().Get("amount")

	// 2. P0 3.2 & 3.3: Strict Format Validation
	if match, _ := regexp.MatchString(`^\d{1,6}(\.\d{1,2})?$`, amountStr); !match {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "invalid_amount", "error": "Invalid format"})
		return
	}

	if match, _ := regexp.MatchString(`^[a-zA-Z0-9]{20,64}$`, apiKey); !match {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "invalid_key", "error": "Invalid key format"})
		return
	}

	// 3. Check if Key exists in KV
	var userKey KeyData
	if err := KVGet("key:"+apiKey, &userKey); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "invalid_key", "error": "Key not found"})
		return
	}

	// 4. Check if Key expired
	if userKey.ExpiresAt < time.Now().Unix() {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "expired", "error": "Key expired"})
		return
	}

	// 5. P0 4.4: Check 24h Consume Marker
	consumeKey := fmt.Sprintf("consumed:%s:%s", apiKey, amountStr)
	if KVGet(consumeKey, nil) == nil {
		// Already verified recently
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": true, "status": "already_verified", "received": amountStr})
		return
	}

	// 6. Check if Cloudflare Worker caught this payment
	var payment PaymentPayload
	if err := KVGet("payment:"+amountStr, &payment); err != nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "not_found"})
		return
	}

	// 7. P0 1.2: Verify HMAC Signature from Worker
	payloadStr := fmt.Sprintf("%s:%d:%s", payment.Amount, payment.Timestamp, payment.UTR)
	if !VerifyKVSignature(payloadStr, payment.Signature) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "error", "error": "Integrity check failed"})
		return
	}

	// 8. P0 4.5: UTR Deduplication
	if IsUTRUsed(payment.UTR) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "error", "error": "UTR already used"})
		return
	}

	// 9. Success! Write Consume Marker (24 hours = 86400s) and Mark UTR Used
	KVPut(consumeKey, payment, 86400)
	MarkUTRUsed(payment.UTR, amountStr, userKey.Email)

	// Delete the payment from KV so it can't be used by someone else
	KVDelete("payment:" + amountStr)

	// Return exact match (Backward compatible JSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"verified": true, 
		"received": amountStr,
		"status":   "exact",
	})
}
