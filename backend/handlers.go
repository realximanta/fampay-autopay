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

func generateSecureRandomString(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)[:length]
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	state := generateSecureRandomString(32)
	verifier, challenge := GeneratePKCE()
	KVPut("pkce:"+state, verifier, 600)

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

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != state {
		http.Error(w, "Invalid OAuth state", http.StatusForbidden)
		return
	}

	var verifier string
	if err := KVGet("pkce:"+state, &verifier); err != nil {
		http.Error(w, "Invalid or expired state", http.StatusForbidden)
		return
	}
	KVDelete("pkce:" + state)

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

	encToken, err := EncryptToken(token.RefreshToken)
	if err != nil {
		http.Error(w, "Failed to secure token", http.StatusInternalServerError)
		return
	}

	go SetupAutoForwarding(context.Background(), token)

	sessionToken := generateSecureRandomString(32)
	KVPut("session:"+sessionToken, email, 86400*30)

	var userKey KeyData
	if err := KVGet("user:"+email, &userKey); err != nil {
		// NEW USER SIGN UP
		newKey := generateSecureRandomString(20)
		userKey = KeyData{Email: email, ExpiresAt: time.Now().Unix()}
		KVPut("user:"+email, userKey, 0)
		KVPut("key:"+newKey, userKey, 0)
		KVPut("email_key:"+email, newKey, 0)
		
		// Send Welcome Email & In-App Notification
		go SendWelcomeEmail(email)
		go LogNotification(email, "Welcome to Auto-Pay! 🚀", "Your account is ready. Integrate our API to automate your FamApp payments.")
		
	} else {
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

func HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}
	var email string
	if err := KVGet("session:"+cookie.Value, &email); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Session expired"})
		return
	}

	history, _ := GetUserHistory(email)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// --- NEW: NOTIFICATION ENDPOINTS ---

func HandleGetNotifications(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}
	var email string
	if err := KVGet("session:"+cookie.Value, &email); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Session expired"})
		return
	}

	notifications, _ := GetNotifications(email)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notifications)
}

func HandleMarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var email string
	if err := KVGet("session:"+cookie.Value, &email); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	MarkNotificationsRead(email)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func HandleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var email string
	if err := KVGet("session:"+cookie.Value, &email); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	SaveFCMToken(email, req.Token)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

// ----------------------------------

func HandleRevoke(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie("session")
	var email string
	KVGet("session:"+cookie.Value, &email)

	var oldKeyStr string
	if err := KVGet("email_key:"+email, &oldKeyStr); err == nil {
		KVDelete("key:" + oldKeyStr)
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
	days := 7
	
	if req.PlanID == "plus" {
		amount = 25
		days = 21
	} else if req.PlanID == "pro" {
		amount = 45
		days = 41
	}

	n, _ := rand.Int(rand.Reader, big.NewInt(99))
	paise := int(n.Int64()) + 1
	fp := fmt.Sprintf("%.2f", float64(amount)+float64(paise)/100.0)

	cookie, _ := r.Cookie("session")
	var email string
	KVGet("session:"+cookie.Value, &email)
	
	pending := PendingData{Email: email, Amount: amount, Days: days}
	KVPut("pending:"+fp, pending, 900)

	LogHistory(email, "QR Generated", fp, "generated")

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

func HandleRechargeVerify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	cookie, err := r.Cookie("session")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"status": "unauthorized"})
		return
	}
	
	var email string
	if err := KVGet("session:"+cookie.Value, &email); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"status": "unauthorized"})
		return
	}

	var req struct {
		Fingerprint string `json:"fingerprint"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	var paymentData map[string]interface{}
	if err := KVGet("payment:"+req.Fingerprint, &paymentData); err != nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_found"})
		return
	}

	var pending PendingData
	if err := KVGet("pending:"+req.Fingerprint, &pending); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"status": "invalid_pending"})
		return
	}

	var userKey KeyData
	KVGet("user:"+pending.Email, &userKey)
	
	if userKey.ExpiresAt < time.Now().Unix() {
		userKey.ExpiresAt = time.Now().Unix()
	}
	userKey.ExpiresAt += int64(pending.Days * 86400)

	KVPut("user:"+pending.Email, userKey, 0)
	var keyStr string
	KVGet("email_key:"+pending.Email, &keyStr)
	KVPut("key:"+keyStr, userKey, 0)

	KVDelete("payment:" + req.Fingerprint)
	KVDelete("pending:" + req.Fingerprint)

	LogHistory(email, "Subscription Recharged", req.Fingerprint, "exact")
	
	// Send Thank You Email & In-App Notification
	go SendThankYouEmail(pending.Email, float64(pending.Amount), pending.Days)
	go LogNotification(pending.Email, "Subscription Recharged", fmt.Sprintf("Successfully added %d days to your API key.", pending.Days))

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "exact", "verified": true})
}

func HandleVerify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	apiKey := r.URL.Query().Get("key")
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		apiKey = strings.TrimPrefix(authHeader, "Bearer ")
	}

	amountStr := r.URL.Query().Get("amount")

	if match, _ := regexp.MatchString(`^\d{1,6}(\.\d{1,2})?$`, amountStr); !match {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "invalid_amount", "error": "Invalid format"})
		return
	}

	if match, _ := regexp.MatchString(`^[a-zA-Z0-9]{10,64}$`, apiKey); !match {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "invalid_key", "error": "Invalid key format"})
		return
	}

	var userKey KeyData
	if err := KVGet("key:"+apiKey, &userKey); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "invalid_key", "error": "Key not found"})
		return
	}

	if userKey.ExpiresAt < time.Now().Unix() {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "expired", "error": "Key expired"})
		return
	}

	consumeKey := fmt.Sprintf("consumed:%s:%s", apiKey, amountStr)
	if KVGet(consumeKey, nil) == nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": true, "status": "already_verified", "received": amountStr})
		return
	}

	var payment PaymentPayload
	if err := KVGet("payment:"+amountStr, &payment); err != nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "not_found"})
		return
	}

	payloadStr := fmt.Sprintf("%s:%d:%s", payment.Amount, payment.Timestamp, payment.UTR)
	if !VerifyKVSignature(payloadStr, payment.Signature) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "error", "error": "Integrity check failed"})
		return
	}

	if IsUTRUsed(payment.UTR) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{"verified": false, "status": "error", "error": "UTR already used"})
		return
	}

	KVPut(consumeKey, payment, 86400)
	MarkUTRUsed(payment.UTR, amountStr, userKey.Email)
	KVDelete("payment:" + amountStr)

	LogHistory(userKey.Email, "API Verify", amountStr, "exact")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"verified": true, 
		"received": amountStr,
		"status":   "exact",
	})
}
